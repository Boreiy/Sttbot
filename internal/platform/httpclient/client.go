package httpclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	randv2 "math/rand/v2"
	"net"
	stdhttp "net/http"
	"net/url"
	"os"
	"strconv"
	"syscall"
	"time"
)

// Client wraps http.Client with logging and retries.
type Client struct {
	hc               *stdhttp.Client
	log              *slog.Logger
	retries          int
	baseBackoff      time.Duration
	maxBackoff       time.Duration
	headers          map[string]string
	urlRedactor      func(*url.URL) string
	retryMethods     map[string]struct{}
	maxRetryDuration time.Duration
	retryNonIdem     bool
	maxReplayBody    int64
	retryPolicy      func(*stdhttp.Response, error) (time.Duration, bool)
}

// Option configures Client.
type Option func(*Client)

// WithTimeout sets request timeout.
func WithTimeout(t time.Duration) Option {
	return func(c *Client) { c.hc.Timeout = t }
}

// WithLogger sets logger used by client.
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) {
		if l != nil {
			c.log = l
		}
	}
}

// WithRetries enables retries with exponential backoff and jitter.
func WithRetries(n int, backoff time.Duration) Option {
	return func(c *Client) {
		c.retries = n
		if backoff > 0 {
			c.baseBackoff = backoff
		}
	}
}

// WithBaseBackoff overrides base backoff duration.
func WithBaseBackoff(d time.Duration) Option {
	return func(c *Client) {
		if d > 0 {
			c.baseBackoff = d
		}
	}
}

// WithMaxBackoff limits exponential backoff growth.
func WithMaxBackoff(d time.Duration) Option {
	return func(c *Client) { c.maxBackoff = d }
}

// WithHeaders adds default headers to each request.
func WithHeaders(h map[string]string) Option {
	return func(c *Client) {
		for k, v := range h {
			if c.headers == nil {
				c.headers = make(map[string]string)
			}
			c.headers[k] = v
		}
	}
}

// WithURLRedactor sets URL redactor for logs.
func WithURLRedactor(f func(*url.URL) string) Option {
	return func(c *Client) { c.urlRedactor = f }
}

// WithoutHeaders removes default headers.
func WithoutHeaders(keys ...string) Option {
	return func(c *Client) {
		for _, k := range keys {
			delete(c.headers, k)
		}
	}
}

// New creates configured Client.
func New(opts ...Option) *Client {
	tr := stdhttp.DefaultTransport.(*stdhttp.Transport).Clone()
	tr.MaxIdleConns = 100
	tr.MaxConnsPerHost = 100
	tr.MaxIdleConnsPerHost = 100
	tr.IdleConnTimeout = 90 * time.Second
	tr.TLSHandshakeTimeout = 10 * time.Second
	tr.ResponseHeaderTimeout = 10 * time.Second
	tr.ExpectContinueTimeout = 1 * time.Second

	c := &Client{
		hc: &stdhttp.Client{
			Timeout:   15 * time.Second,
			Transport: tr,
		},
		log:           slog.Default(),
		retries:       0,
		baseBackoff:   200 * time.Millisecond,
		maxReplayBody: 1 << 20,
		retryPolicy:   retryInfo,
		retryMethods: map[string]struct{}{
			stdhttp.MethodGet:     {},
			stdhttp.MethodHead:    {},
			stdhttp.MethodOptions: {},
			stdhttp.MethodTrace:   {},
			stdhttp.MethodPut:     {},
			stdhttp.MethodDelete:  {},
		},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// WithTransport sets custom transport.
func WithTransport(rt stdhttp.RoundTripper) Option {
	return func(c *Client) {
		if rt != nil {
			c.hc.Transport = rt
		}
	}
}

// WithRetryMethods adds methods allowed for retries.
func WithRetryMethods(methods ...string) Option {
	return func(c *Client) {
		if c.retryMethods == nil {
			c.retryMethods = make(map[string]struct{})
		}
		for _, m := range methods {
			c.retryMethods[m] = struct{}{}
		}
	}
}

// WithMaxRetryDuration limits total time spent on retries.
func WithMaxRetryDuration(d time.Duration) Option {
	return func(c *Client) { c.maxRetryDuration = d }
}

// WithRetryNonIdempotent allows retries for non-idempotent methods like POST.
func WithRetryNonIdempotent(v bool) Option {
	return func(c *Client) { c.retryNonIdem = v }
}

// WithMaxReplayBodySize limits size of buffered body for retries (0 disables limit).
func WithMaxReplayBodySize(n int64) Option {
	return func(c *Client) { c.maxReplayBody = n }
}

// ErrReplayBodyTooLarge indicates request body exceeds replay limit.
var ErrReplayBodyTooLarge = errors.New("http: body too large for replay")

// WithRetryPolicy sets custom retry policy.
func WithRetryPolicy(f func(*stdhttp.Response, error) (time.Duration, bool)) Option {
	return func(c *Client) {
		if f != nil {
			c.retryPolicy = f
		}
	}
}

// retryAfter parses Retry-After header value.
func retryAfter(h string) time.Duration {
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(h); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := stdhttp.ParseTime(h); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0
		}
		return d
	}
	return 0
}

// redactURL returns redacted URL string.
func (c *Client) redactURL(u *url.URL) string {
	if c.urlRedactor != nil {
		return c.urlRedactor(u)
	}
	return u.Redacted()
}

// drainAndClose drains up to 512KB from body and closes it.
func drainAndClose(b io.ReadCloser) {
	if b == nil {
		return
	}
	_, _ = io.CopyN(io.Discard, b, 512<<10)
	_ = b.Close()
}

// retryInfo determines if request should be retried and returns optional delay.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		if ne, ok := ue.Err.(net.Error); ok && ne.Timeout() {
			return true
		}
		if oe, ok := ue.Err.(*net.OpError); ok {
			if se, ok := oe.Err.(*os.SyscallError); ok {
				switch se.Err {
				case syscall.ECONNRESET, syscall.ECONNREFUSED, syscall.ECONNABORTED,
					syscall.ENETDOWN, syscall.ENETUNREACH, syscall.EPIPE,
					syscall.EHOSTUNREACH, syscall.ETIMEDOUT:
					return true
				}
			}
		}
		var dnsErr *net.DNSError
		if errors.As(ue.Err, &dnsErr) && dnsErr.IsTemporary {
			return true
		}
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	return false
}

func retryInfo(resp *stdhttp.Response, err error) (time.Duration, bool) {
	if err != nil {
		if isRetryableError(err) {
			return 0, true
		}
		return 0, false
	}
	switch resp.StatusCode {
	case 408, 421, 425:
		drainAndClose(resp.Body)
		return 0, true
	case 429, 503:
		delay := retryAfter(resp.Header.Get("Retry-After"))
		drainAndClose(resp.Body)
		return delay, true
	default:
		if resp.StatusCode >= 500 {
			delay := retryAfter(resp.Header.Get("Retry-After"))
			drainAndClose(resp.Body)
			return delay, true
		}
		return 0, false
	}
}

// Do sends HTTP request with context, logging and retries.
func (c *Client) Do(ctx context.Context, req *stdhttp.Request) (*stdhttp.Response, error) {
	if req.Body != nil && req.GetBody == nil {
		var body []byte
		var err error
		if c.maxReplayBody > 0 {
			limited := io.LimitReader(req.Body, c.maxReplayBody+1)
			body, err = io.ReadAll(limited)
			if err != nil {
				return nil, err
			}
			if int64(len(body)) > c.maxReplayBody {
				return nil, ErrReplayBodyTooLarge
			}
		} else {
			body, err = io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
		}
		req.Body.Close()
		req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
		rc, _ := req.GetBody()
		req.Body = rc
	}

	retries := c.retries
	if _, ok := c.retryMethods[req.Method]; !ok {
		if !(req.Method == stdhttp.MethodPost && req.Header.Get("Idempotency-Key") != "") && !c.retryNonIdem {
			retries = 0
		}
	}

	var lastErr error
	var budgetExceeded bool
	start := time.Now()
	for attempt := 1; attempt <= retries+1; attempt++ {
		r := req.Clone(ctx)
		for k, v := range c.headers {
			if r.Header.Get(k) == "" {
				r.Header.Set(k, v)
			}
		}
		if r.GetBody != nil {
			rc, err := r.GetBody()
			if err != nil {
				return nil, err
			}
			r.Body = rc
		}
		u := c.redactURL(r.URL)
		st := time.Now()
		resp, err := c.hc.Do(r)
		dur := time.Since(st)
		delay, retry := c.retryPolicy(resp, err)
		retryAfterDelay := delay > 0
		if resp != nil && resp.StatusCode == 421 {
			if tr, ok := c.hc.Transport.(interface{ CloseIdleConnections() }); ok {
				tr.CloseIdleConnections()
			}
		}
		if !retry {
			if err != nil {
				c.log.Warn("http request error", slog.String("method", r.Method), slog.String("url", u), slog.Int("attempt", attempt), slog.Any("error", err))
				return nil, err
			}
			c.log.Info("http request", slog.String("method", r.Method), slog.String("url", u), slog.Int("status", resp.StatusCode), slog.Duration("dur", dur), slog.Int("attempt", attempt))
			return resp, nil
		}
		wait := c.baseBackoff * time.Duration(1<<uint(attempt-1))
		truncatedRetryAfter := false
		if delay > 0 {
			wait = delay
		} else if wait > 0 {
			wait += time.Duration(randv2.Int64N(int64(wait)))
		}
		if c.maxBackoff > 0 && wait > c.maxBackoff {
			wait = c.maxBackoff
		}
		if deadline, ok := ctx.Deadline(); ok && wait > 0 {
			if rem := time.Until(deadline); rem <= 0 {
				return nil, context.DeadlineExceeded
			} else if wait > rem {
				if retryAfterDelay {
					truncatedRetryAfter = true
				}
				wait = rem
			}
		}
		attemptsLeft := retries - attempt
		if attemptsLeft < 0 {
			attemptsLeft = 0
		}
		if err != nil {
			lastErr = err
			c.log.Warn("http request error", slog.String("method", r.Method), slog.String("url", u), slog.Int("attempt", attempt), slog.Int("attempts_left", attemptsLeft), slog.Duration("wait", wait), slog.Duration("retry_after", delay), slog.Bool("idempotency_key", r.Header.Get("Idempotency-Key") != ""), slog.Any("error", err))
		} else {
			lastErr = fmt.Errorf("%s %s: unexpected status %d", r.Method, c.redactURL(r.URL), resp.StatusCode)
			c.log.Warn("http request status", slog.String("method", r.Method), slog.String("url", u), slog.Int("attempt", attempt), slog.Int("attempts_left", attemptsLeft), slog.Duration("wait", wait), slog.Duration("retry_after", delay), slog.Bool("idempotency_key", r.Header.Get("Idempotency-Key") != ""), slog.Int("status", resp.StatusCode))
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if attempt <= retries {
			if c.maxRetryDuration > 0 {
				elapsed := time.Since(start)
				if elapsed+wait > c.maxRetryDuration {
					budgetExceeded = true
					break
				}
			}
			timer := time.NewTimer(wait)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}
			if truncatedRetryAfter {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				return nil, context.DeadlineExceeded
			}
			// Check context again after waiting
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
	}
	if budgetExceeded && lastErr != nil {
		return nil, fmt.Errorf("retry budget exceeded: %w", lastErr)
	}
	return nil, lastErr
}
