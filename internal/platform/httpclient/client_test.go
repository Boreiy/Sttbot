package httpclient_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

    httpclient "sttbot/internal/platform/httpclient"

	"github.com/stretchr/testify/require"
)

func TestClient_Do_Retries(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(1, 0),
	)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	resp, err := c.Do(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 2, attempts)
}

func TestClient_Do_Retry408(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusRequestTimeout)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(1, 0),
	)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	resp, err := c.Do(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 2, attempts)
}

func TestClient_Do_Retry429RetryAfter(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(1, 0),
	)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	start := time.Now()
	resp, err := c.Do(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 2, attempts)
	require.GreaterOrEqual(t, time.Since(start), time.Second)
}

func TestClient_Do_Retry421(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusMisdirectedRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(1, 0),
	)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	resp, err := c.Do(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 2, attempts)
}

func TestClient_Do_Retry425(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooEarly)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(1, 0),
	)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	resp, err := c.Do(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 2, attempts)
}

func TestClient_Do_RetryNetworkError(t *testing.T) {
	var attempts int32
	rt := rtFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, &net.OpError{
			Op:  "read",
			Net: "tcp",
			Err: &os.SyscallError{Syscall: "read", Err: syscall.ECONNRESET},
		}
	})

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(1, 0),
		httpclient.WithTransport(rt),
	)
	req, err := http.NewRequest(http.MethodGet, "http://example.invalid", nil)
	require.NoError(t, err)

	_, err = c.Do(context.Background(), req)
	require.Error(t, err)
	require.Equal(t, int32(2), atomic.LoadInt32(&attempts))
}

func TestClient_Do_RetryNetErrClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var attempts int
	rt := rtFunc(func(req *http.Request) (*http.Response, error) {
		if attempts == 0 {
			attempts++
			return nil, net.ErrClosed
		}
		attempts++
		return http.DefaultTransport.RoundTrip(req)
	})
	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(1, 0),
		httpclient.WithTransport(rt),
	)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	resp, err := c.Do(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 2, attempts)
}

func TestClient_Do_Headers(t *testing.T) {
	var header string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Get("X-Test")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.WithHeaders(map[string]string{"X-Test": "1"}))
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	_, err = c.Do(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, "1", header)
}

func TestClient_Do_WithoutHeaders(t *testing.T) {
	var headerA, headerB string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headerA = r.Header.Get("X-A")
		headerB = r.Header.Get("X-B")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithHeaders(map[string]string{"X-A": "1", "X-B": "2"}),
		httpclient.WithoutHeaders("X-B"),
	)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	_, err = c.Do(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, "1", headerA)
	require.Empty(t, headerB)
}

func TestClient_Do_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithRetries(1, time.Second),
	)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err = c.Do(ctx, req)
	require.ErrorIs(t, err, context.Canceled)
	require.Less(t, time.Since(start), time.Second)
}

func TestClient_Do_ExponentialBackoff(t *testing.T) {
	var (
		mu    sync.Mutex
		times []time.Time
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(2, 50*time.Millisecond),
	)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	_, err = c.Do(context.Background(), req)
	require.Error(t, err)
	require.Len(t, times, 3)

	diff1 := times[1].Sub(times[0])
	diff2 := times[2].Sub(times[1])

	require.GreaterOrEqual(t, diff1, 50*time.Millisecond)
	require.GreaterOrEqual(t, diff2, 100*time.Millisecond)
	require.Less(t, diff1, 200*time.Millisecond)
	require.Less(t, diff2, 400*time.Millisecond)
}

func TestClient_Do_RetryBody(t *testing.T) {
	var (
		attempts int
		bodies   []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(1, 0),
	)
	req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader("payload"))
	require.NoError(t, err)
	req.Header.Set("Idempotency-Key", "k")

	resp, err := c.Do(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 2, attempts)
	require.Equal(t, []string{"payload", "payload"}, bodies)
}

func TestClient_Do_BodyTooLarge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(1, 0),
		httpclient.WithMaxReplayBodySize(10),
	)
	body := io.NopCloser(strings.NewReader("0123456789ABC"))
	req, err := http.NewRequest(http.MethodPost, srv.URL, body)
	require.NoError(t, err)
	req.Header.Set("Idempotency-Key", "k")

	_, err = c.Do(context.Background(), req)
	require.ErrorIs(t, err, httpclient.ErrReplayBodyTooLarge)
}

func TestClient_Do_PostNoRetryWithoutIdempotencyKey(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(1, 0),
	)
	req, err := http.NewRequest(http.MethodPost, srv.URL, nil)
	require.NoError(t, err)

	_, err = c.Do(context.Background(), req)
	require.Error(t, err)
	require.Equal(t, 1, attempts)
}

func TestClient_Do_RetryPostWithOption(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(1, 0),
		httpclient.WithRetryNonIdempotent(true),
	)
	req, err := http.NewRequest(http.MethodPost, srv.URL, nil)
	require.NoError(t, err)

	resp, err := c.Do(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 2, attempts)
}

func TestClient_Do_Retry503RetryAfter(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(1, 0),
	)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	start := time.Now()
	resp, err := c.Do(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 2, attempts)
	require.GreaterOrEqual(t, time.Since(start), time.Second)
}

func TestClient_Do_RetryAfterPast(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			past := time.Now().UTC().Add(-time.Minute).Format(http.TimeFormat)
			w.Header().Set("Retry-After", past)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(1, 0),
	)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	start := time.Now()
	resp, err := c.Do(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 2, attempts)
	require.Less(t, time.Since(start), 500*time.Millisecond)
}

func TestClient_Do_MaxRetryDuration(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(5, 50*time.Millisecond),
		httpclient.WithMaxRetryDuration(120*time.Millisecond),
	)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	_, err = c.Do(context.Background(), req)
	require.Error(t, err)
	require.ErrorContains(t, err, "retry budget exceeded")
	require.Equal(t, 2, attempts)
}

func TestClient_Do_URLRedactor(t *testing.T) {
	var buf bytes.Buffer
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	var called bool
	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(&buf, nil))),
		httpclient.WithRetries(0, 0),
		httpclient.WithURLRedactor(func(u *url.URL) string {
			called = true
			return "redacted"
		}),
	)
	req, err := http.NewRequest(http.MethodGet, srv.URL+"?token=secret", nil)
	require.NoError(t, err)

	_, _ = c.Do(context.Background(), req)
	require.True(t, called)
	require.Contains(t, buf.String(), "url=redacted")
}

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestClient_WithTransport(t *testing.T) {
	var used bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rt := rtFunc(func(req *http.Request) (*http.Response, error) {
		used = true
		return http.DefaultTransport.RoundTrip(req)
	})
	c := httpclient.New(httpclient.WithTransport(rt))

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := c.Do(context.Background(), req)
	require.NoError(t, err)
	resp.Body.Close()
	require.True(t, used)
}

func TestClient_Do_MaxBackoff(t *testing.T) {
	var (
		mu    sync.Mutex
		times []time.Time
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(2, 50*time.Millisecond),
		httpclient.WithMaxBackoff(60*time.Millisecond),
	)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	_, err = c.Do(context.Background(), req)
	require.Error(t, err)
	require.Len(t, times, 3)

	diff1 := times[1].Sub(times[0])
	diff2 := times[2].Sub(times[1])
	require.Less(t, diff1, 100*time.Millisecond)
	require.Less(t, diff2, 100*time.Millisecond)
}

func TestClient_Do_RequestHeaderPriority(t *testing.T) {
	var header string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Get("X-Test")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.WithHeaders(map[string]string{"X-Test": "a"}))
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	req.Header.Set("X-Test", "b")

	_, err = c.Do(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, "b", header)
}

func TestClient_Do_NoRetryOn4xx(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(3, 0),
	)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	resp, err := c.Do(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.Equal(t, int32(1), atomic.LoadInt32(&attempts))
}

func TestClient_Do_RetryPATCH(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(1, 0),
		httpclient.WithRetryMethods(http.MethodPatch),
	)
	req, err := http.NewRequest(http.MethodPatch, srv.URL, nil)
	require.NoError(t, err)

	resp, err := c.Do(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, int32(2), atomic.LoadInt32(&attempts))
}

type closingRT struct {
	http.RoundTripper
	closed bool
}

func (c *closingRT) CloseIdleConnections() { c.closed = true }

func TestClient_Do_421ClosesIdle(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.WriteHeader(http.StatusMisdirectedRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rt := &closingRT{RoundTripper: http.DefaultTransport}
	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(1, 0),
		httpclient.WithTransport(rt),
	)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	resp, err := c.Do(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.True(t, rt.closed)
}

func TestClient_Do_DNSTemporary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var rtAttempts int32
	rt := rtFunc(func(req *http.Request) (*http.Response, error) {
		if atomic.AddInt32(&rtAttempts, 1) == 1 {
			return nil, &url.Error{Op: "Get", URL: req.URL.String(), Err: &net.DNSError{IsTemporary: true}}
		}
		return http.DefaultTransport.RoundTrip(req)
	})

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(1, 0),
		httpclient.WithTransport(rt),
	)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	resp, err := c.Do(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, int32(2), atomic.LoadInt32(&rtAttempts))
}

func TestClient_Do_RetryAfterContextDeadline(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}

		<-r.Context().Done()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(1, time.Second),
	)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = c.Do(ctx, req)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), 500*time.Millisecond)
}

func TestClient_Do_ReusesConnection(t *testing.T) {
	var attempts int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, err := w.Write([]byte("fail"))
			if err != nil {
				// log write error without failing test
				t.Logf("write error: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewUnstartedServer(handler)
	var conns int32
	srv.Config.ConnState = func(c net.Conn, s http.ConnState) {
		if s == http.StateNew {
			atomic.AddInt32(&conns, 1)
		}
	}
	srv.Start()
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(1, 0),
	)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	resp, err := c.Do(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, int32(2), atomic.LoadInt32(&attempts))
	require.Equal(t, int32(1), atomic.LoadInt32(&conns))
}

func TestClient_Do_Parallel(t *testing.T) {
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&count, 1)%5 == 0 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := httpclient.New(
		httpclient.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		httpclient.WithRetries(1, 0),
	)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
			_, err := c.Do(context.Background(), req)
			_ = err // ignore error; not relevant for this test
		}()
	}
	wg.Wait()
}
