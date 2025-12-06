package retry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/url"
	"os"
	"syscall"
	"time"
)

// JitterStrategy defines the jitter strategy to use
type JitterStrategy int

const (
	// JitterNone disables jitter
	JitterNone JitterStrategy = iota
	// JitterEqual applies uniform jitter (equal chance of any delay in range)
	JitterEqual
	// JitterDecorrelated applies decorrelated jitter (AWS recommended)
	JitterDecorrelated
)

// Config defines retry configuration
type Config struct {
	// MaxAttempts is the maximum number of attempts (including the first one)
	MaxAttempts int
	// InitialDelay is the initial delay between retries
	InitialDelay time.Duration
	// MinDelay is the minimum delay between retries (defaults to InitialDelay)
	MinDelay time.Duration
	// MaxDelay is the maximum delay between retries
	MaxDelay time.Duration
	// MaxElapsedTime is the maximum total time to spend on retries (0 = no limit)
	MaxElapsedTime time.Duration
	// Multiplier is the exponential backoff multiplier
	Multiplier float64
	// Jitter adds randomization to delays to avoid thundering herd
	Jitter bool
	// JitterStrategy defines the jitter algorithm to use
	JitterStrategy JitterStrategy
	// Rand is the random source for jitter (optional, uses local source if nil)
	Rand *rand.Rand
	// OnRetry is called on each retry attempt for observability
	OnRetry func(attempt int, err error, nextDelay time.Duration)
	// NextDelay allows custom delay calculation (overrides backoff+jitter if provided)
	NextDelay func(attempt int, err error) (time.Duration, bool)
	// Now returns current time (for testing, defaults to time.Now)
	Now func() time.Time
	// After creates a timer channel (for testing, defaults to time.After)
	After func(d time.Duration) <-chan time.Time
}

// DefaultConfig returns a sensible default configuration
func DefaultConfig() Config {
	return Config{
		MaxAttempts:    3,
		InitialDelay:   100 * time.Millisecond,
		MinDelay:       0, // will be set to InitialDelay during normalization
		MaxDelay:       30 * time.Second,
		MaxElapsedTime: 0, // no limit
		Multiplier:     2.0,
		Jitter:         true,
		JitterStrategy: JitterDecorrelated,
		Rand:           nil, // will create local source
		OnRetry:        nil,
		NextDelay:      nil,
		Now:            nil, // will use time.Now
		After:          nil, // will use time.After
	}
}

// Normalize validates and normalizes the configuration
func (c *Config) Normalize() error {
	if c.MaxAttempts <= 0 {
		return errors.New("retry: MaxAttempts must be positive")
	}
	if c.InitialDelay <= 0 {
		return errors.New("retry: InitialDelay must be positive")
	}
	if c.MinDelay <= 0 {
		c.MinDelay = c.InitialDelay
	}
	if c.MaxDelay <= 0 {
		c.MaxDelay = 30 * time.Second
	}
	if c.MinDelay > c.MaxDelay {
		return errors.New("retry: MinDelay cannot be greater than MaxDelay")
	}
	if c.InitialDelay < c.MinDelay || c.InitialDelay > c.MaxDelay {
		return errors.New("retry: InitialDelay must be between MinDelay and MaxDelay")
	}
	if c.Multiplier <= 0 {
		c.Multiplier = 2.0 // default multiplier
	}
	if c.Multiplier < 1.0 {
		return errors.New("retry: Multiplier must be >= 1.0")
	}
	if c.MaxElapsedTime < 0 {
		return errors.New("retry: MaxElapsedTime cannot be negative")
	}

	// Set up defaults for optional fields
	if c.Rand == nil {
		c.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.After == nil {
		c.After = time.After
	}

	// Handle legacy Jitter field
	if c.Jitter && c.JitterStrategy == JitterNone {
		c.JitterStrategy = JitterDecorrelated
	}

	return nil
}

// RetryableFunc is a function that can be retried
type RetryableFunc func(ctx context.Context) error

// IsRetryableFunc determines if an error should trigger a retry
type IsRetryableFunc func(err error) bool

// RetriesExceededError is returned when retries are exhausted
type RetriesExceededError struct {
	LastError     error
	Attempts      int
	TotalDuration time.Duration
	Reason        string
}

func (e *RetriesExceededError) Error() string {
	return "retry: " + e.Reason + " after " + e.TotalDuration.String() + " (" +
		fmt.Sprintf("%d", e.Attempts) + " attempts): " + e.LastError.Error()
}

func (e *RetriesExceededError) Unwrap() error {
	return e.LastError
}

// DefaultRetryable returns true for temporary errors and context deadline exceeded
func DefaultRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Don't retry context cancellation
	if errors.Is(err, context.Canceled) {
		return false
	}

	// Retry on deadline exceeded (timeout)
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Check for net.Error with Timeout
	type netError interface {
		Timeout() bool
	}
	if ne, ok := err.(netError); ok && ne.Timeout() {
		return true
	}

	// Check for specific network errors
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	// Check for net.ErrClosed
	if errors.Is(err, net.ErrClosed) {
		return true
	}

	// Check for URL errors wrapping network errors
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		// Check if wrapped error has Timeout
		if ne, ok := urlErr.Err.(netError); ok && ne.Timeout() {
			return true
		}

		// Check for DNS temporary errors
		var dnsErr *net.DNSError
		if errors.As(urlErr.Err, &dnsErr) && dnsErr.IsTemporary {
			return true
		}

		// Check for other network operation errors
		var opErr *net.OpError
		if errors.As(urlErr.Err, &opErr) {
			// Check for system call errors that indicate temporary conditions
			var syscallErr *os.SyscallError
			if errors.As(opErr.Err, &syscallErr) {
				// Common temporary syscall errors
				switch syscallErr.Err {
				case syscall.ECONNRESET, syscall.ECONNREFUSED, syscall.ECONNABORTED,
					syscall.ENETDOWN, syscall.ENETUNREACH, syscall.EPIPE,
					syscall.EHOSTUNREACH, syscall.ETIMEDOUT:
					return true
				}
			}
		}
	}

	// Check for temporary interface (fallback for compatibility)
	type temporary interface {
		Temporary() bool
	}
	if t, ok := err.(temporary); ok {
		return t.Temporary()
	}

	return false
}

// Do executes a function with retry logic using exponential backoff
func Do(ctx context.Context, config Config, fn RetryableFunc) error {
	return DoWithRetryable(ctx, config, fn, DefaultRetryable)
}

// DoWithRetryable executes a function with retry logic and custom retryable check
func DoWithRetryable(ctx context.Context, config Config, fn RetryableFunc, isRetryable IsRetryableFunc) error {
	// Normalize and validate config
	configCopy := config // Make a copy to avoid modifying the original
	if err := configCopy.Normalize(); err != nil {
		return err
	}

	var lastErr error
	startTime := configCopy.Now()

	for attempt := 1; attempt <= configCopy.MaxAttempts; attempt++ {
		// Check context before each attempt
		if ctx.Err() != nil {
			return ctx.Err()
		}

		lastErr = fn(ctx)
		if lastErr == nil {
			return nil // success
		}

		// If this is the last attempt, don't check for retryability
		if attempt == configCopy.MaxAttempts {
			break
		}

		// Check if error is retryable
		if !isRetryable(lastErr) {
			return lastErr // Return original error for non-retryable errors
		}

		// Calculate delay for next attempt
		var delay time.Duration
		var shouldRetry bool

		// Use custom NextDelay if provided
		if configCopy.NextDelay != nil {
			delay, shouldRetry = configCopy.NextDelay(attempt, lastErr)
			if !shouldRetry {
				return lastErr // Return original error if custom policy says stop
			}
		} else {
			delay = configCopy.calculateDelay(attempt)
		}

		// Apply jitter if enabled
		delay = configCopy.applyJitter(delay)

		// Check MaxElapsedTime budget
		if configCopy.MaxElapsedTime > 0 {
			elapsed := configCopy.Now().Sub(startTime)
			if elapsed+delay > configCopy.MaxElapsedTime {
				return &RetriesExceededError{
					LastError:     lastErr,
					Attempts:      attempt,
					TotalDuration: elapsed,
					Reason:        "max elapsed time exceeded",
				}
			}
		}

		// Respect context deadline
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			if delay > remaining {
				delay = remaining
			}
		}

		// Call OnRetry callback if provided
		if configCopy.OnRetry != nil {
			configCopy.OnRetry(attempt, lastErr, delay)
		}

		// Wait with context cancellation support
		timer := configCopy.After(delay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer:
			// Continue to next attempt
		}
	}

	// Return enhanced error with retry metadata
	return &RetriesExceededError{
		LastError:     lastErr,
		Attempts:      configCopy.MaxAttempts,
		TotalDuration: configCopy.Now().Sub(startTime),
		Reason:        "max attempts exceeded",
	}
}

// calculateDelay calculates the delay for the given attempt using exponential backoff
func (c Config) calculateDelay(attempt int) time.Duration {
	// Use integer math to avoid float precision issues
	delay := c.InitialDelay

	// Apply multiplier (attempt-1) times
	for i := 1; i < attempt; i++ {
		// Check for overflow before multiplication
		if delay > c.MaxDelay/time.Duration(c.Multiplier) {
			return c.MaxDelay
		}
		delay = time.Duration(float64(delay) * c.Multiplier)

		// Early exit if we've exceeded max delay
		if delay > c.MaxDelay {
			return c.MaxDelay
		}
	}

	// Ensure we're within bounds
	if delay < c.MinDelay {
		delay = c.MinDelay
	}
	if delay > c.MaxDelay {
		delay = c.MaxDelay
	}

	return delay
}

// applyJitter applies the configured jitter strategy to the delay
func (c Config) applyJitter(baseDelay time.Duration) time.Duration {
	if c.JitterStrategy == JitterNone && !c.Jitter {
		return baseDelay
	}

	switch c.JitterStrategy {
	case JitterEqual:
		// Equal jitter: random value between 0 and baseDelay
		jitter := time.Duration(c.Rand.Int63n(int64(baseDelay)))
		return clamp(jitter, c.MinDelay, c.MaxDelay)

	case JitterDecorrelated:
		// Decorrelated jitter: 3 * baseDelay / 2 ± baseDelay / 2
		max := 3 * baseDelay / 2
		jitter := baseDelay + time.Duration(c.Rand.Int63n(int64(max-baseDelay/2)))
		return clamp(jitter, c.MinDelay, c.MaxDelay)

	default:
		// Legacy jitter (±25% for backward compatibility)
		if c.Jitter {
			jitterRange := baseDelay / 4 // 25%
			jitter := baseDelay + time.Duration(c.Rand.Int63n(int64(2*jitterRange))) - jitterRange
			return clamp(jitter, c.MinDelay, c.MaxDelay)
		}
		return baseDelay
	}
}

// clamp ensures the value is within the specified bounds
func clamp(value, min, max time.Duration) time.Duration {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// Retry is a convenience function that uses default configuration
func Retry(ctx context.Context, fn RetryableFunc) error {
	return Do(ctx, DefaultConfig(), fn)
}

// RetryWithAttempts is a convenience function with custom max attempts
func RetryWithAttempts(ctx context.Context, maxAttempts int, fn RetryableFunc) error {
	config := DefaultConfig()
	config.MaxAttempts = maxAttempts
	return Do(ctx, config, fn)
}

// RetryWithConfig is a convenience function that validates config and retries
func RetryWithConfig(ctx context.Context, config Config, fn RetryableFunc) error {
	return Do(ctx, config, fn)
}

// RetryWithTimeout is a convenience function with timeout and max attempts
func RetryWithTimeout(ctx context.Context, timeout time.Duration, maxAttempts int, fn RetryableFunc) error {
	config := DefaultConfig()
	config.MaxAttempts = maxAttempts
	config.MaxElapsedTime = timeout
	return Do(ctx, config, fn)
}
