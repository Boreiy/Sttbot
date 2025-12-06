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
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// customError implements temporary interface for testing
type customError struct {
	message   string
	temporary bool
}

func (e customError) Error() string   { return e.message }
func (e customError) Temporary() bool { return e.temporary }

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxAttempts != 3 {
		t.Errorf("expected MaxAttempts=3, got %d", cfg.MaxAttempts)
	}
	if cfg.InitialDelay != 100*time.Millisecond {
		t.Errorf("expected InitialDelay=100ms, got %v", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 30*time.Second {
		t.Errorf("expected MaxDelay=30s, got %v", cfg.MaxDelay)
	}
	if cfg.Multiplier != 2.0 {
		t.Errorf("expected Multiplier=2.0, got %f", cfg.Multiplier)
	}
	if !cfg.Jitter {
		t.Error("expected Jitter=true")
	}
}

func TestDefaultRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"context canceled", context.Canceled, false},
		{"context deadline exceeded", context.DeadlineExceeded, true},
		{"temporary error", customError{"temp", true}, true},
		{"non-temporary error", customError{"not temp", false}, false},
		{"regular error", errors.New("regular"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DefaultRetryable(tt.err)
			if result != tt.expected {
				t.Errorf("DefaultRetryable(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestCalculateDelay(t *testing.T) {
	config := Config{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 100 * time.Millisecond}, // 100 * 2^0
		{2, 200 * time.Millisecond}, // 100 * 2^1
		{3, 400 * time.Millisecond}, // 100 * 2^2
		{4, 800 * time.Millisecond}, // 100 * 2^3
		{5, 1 * time.Second},        // 100 * 2^4 = 1600ms, capped at 1s
		{6, 1 * time.Second},        // still capped
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			result := config.calculateDelay(tt.attempt)
			if result != tt.expected {
				t.Errorf("calculateDelay(%d) = %v, want %v", tt.attempt, result, tt.expected)
			}
		})
	}
}

func TestDoSuccess(t *testing.T) {
	ctx := context.Background()
	config := Config{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       false, // disable for predictable testing
	}

	var attempts int32
	fn := func(ctx context.Context) error {
		atomic.AddInt32(&attempts, 1)
		return nil // success on first try
	}

	err := Do(ctx, config, fn)
	if err != nil {
		t.Errorf("expected success, got error: %v", err)
	}

	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestDoRetryableError(t *testing.T) {
	ctx := context.Background()
	config := Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond, // very short for testing
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       false,
	}

	var attempts int32
	fn := func(ctx context.Context) error {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			return customError{"temporary failure", true}
		}
		return nil // success on 3rd try
	}

	err := Do(ctx, config, fn)
	if err != nil {
		t.Errorf("expected success after retries, got error: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestDoNonRetryableError(t *testing.T) {
	ctx := context.Background()
	config := DefaultConfig()

	var attempts int32
	expectedErr := errors.New("permanent error")

	fn := func(ctx context.Context) error {
		atomic.AddInt32(&attempts, 1)
		return expectedErr
	}

	err := DoWithRetryable(ctx, config, fn, func(err error) bool {
		return false // never retry
	})

	if err != expectedErr {
		t.Errorf("expected permanent error, got: %v", err)
	}

	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retries), got %d", attempts)
	}
}

func TestDoMaxAttemptsReached(t *testing.T) {
	ctx := context.Background()
	config := Config{
		MaxAttempts:  2,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       false,
	}

	var attempts int32
	expectedErr := customError{"always fails", true}

	fn := func(ctx context.Context) error {
		atomic.AddInt32(&attempts, 1)
		return expectedErr
	}

	err := Do(ctx, config, fn)
	var retryErr *RetriesExceededError
	if !errors.As(err, &retryErr) {
		t.Fatalf("expected RetriesExceededError, got %T", err)
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("should be able to unwrap to original error")
	}

	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestDoContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	config := Config{
		MaxAttempts:  5,
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       false,
	}

	var attempts int32
	fn := func(ctx context.Context) error {
		count := atomic.AddInt32(&attempts, 1)
		if count == 2 {
			cancel() // cancel after second attempt
		}
		return customError{"retryable", true}
	}

	err := Do(ctx, config, fn)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}

	// Should have 2 attempts, then cancelled during delay
	if attempts < 2 {
		t.Errorf("expected at least 2 attempts, got %d", attempts)
	}
}

func TestDoContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	config := Config{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond, // longer than context timeout
		MaxDelay:     200 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       false,
	}

	var attempts int32
	fn := func(ctx context.Context) error {
		atomic.AddInt32(&attempts, 1)
		time.Sleep(5 * time.Millisecond) // ensure we use some time
		return customError{"retryable", true}
	}

	err := Do(ctx, config, fn)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got: %v", err)
	}

	// Should timeout within a reasonable number of attempts
	if attempts > 3 {
		t.Errorf("expected fewer attempts before timeout, got %d", attempts)
	}
}

func TestDoInvalidConfig(t *testing.T) {
	ctx := context.Background()
	config := Config{MaxAttempts: 0} // invalid

	fn := func(ctx context.Context) error {
		return nil
	}

	err := Do(ctx, config, fn)
	if err == nil || err.Error() != "retry: MaxAttempts must be positive" {
		t.Errorf("expected validation error, got: %v", err)
	}
}

func TestRetryConvenienceFunctions(t *testing.T) {
	ctx := context.Background()

	t.Run("Retry", func(t *testing.T) {
		var attempts int32
		fn := func(ctx context.Context) error {
			if atomic.AddInt32(&attempts, 1) == 1 {
				return customError{"temp", true}
			}
			return nil
		}

		err := Retry(ctx, fn)
		if err != nil {
			t.Errorf("Retry failed: %v", err)
		}
		if attempts != 2 {
			t.Errorf("expected 2 attempts, got %d", attempts)
		}
	})

	t.Run("RetryWithAttempts", func(t *testing.T) {
		var attempts int32
		fn := func(ctx context.Context) error {
			atomic.AddInt32(&attempts, 1)
			return customError{"always fails", true}
		}

		err := RetryWithAttempts(ctx, 5, fn)
		if err == nil {
			t.Error("expected error after 5 attempts")
		}
		if attempts != 5 {
			t.Errorf("expected 5 attempts, got %d", attempts)
		}
	})
}

func TestJitterVariation(t *testing.T) {
	config := Config{
		MaxAttempts:    2,
		InitialDelay:   100 * time.Millisecond,
		MinDelay:       50 * time.Millisecond,
		MaxDelay:       200 * time.Millisecond,
		Multiplier:     2.0,
		JitterStrategy: JitterDecorrelated,
		Rand:           rand.New(rand.NewSource(42)), // deterministic for testing
	}

	// Normalize to set up Rand and other fields
	if err := config.Normalize(); err != nil {
		t.Fatalf("config normalize failed: %v", err)
	}

	// Test that applyJitter produces different delays from the base delay
	baseDelay := config.calculateDelay(1) // 100ms
	jitteredDelays := make([]time.Duration, 10)

	for i := 0; i < 10; i++ {
		jitteredDelays[i] = config.applyJitter(baseDelay)
	}

	// Check that jitter produces variations
	allSame := true
	for i := 1; i < len(jitteredDelays); i++ {
		if jitteredDelays[i] != jitteredDelays[0] {
			allSame = false
			break
		}
	}

	if allSame {
		t.Error("jitter should produce different delays, but all were the same")
	}

	// Check that all delays are within bounds
	for i, delay := range jitteredDelays {
		if delay < config.MinDelay || delay > config.MaxDelay {
			t.Errorf("jittered delay[%d] = %v is outside bounds [%v, %v]",
				i, delay, config.MinDelay, config.MaxDelay)
		}
	}
}

func TestDoWithRetryableCustomCheck(t *testing.T) {
	ctx := context.Background()
	config := Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       false,
	}

	var attempts int32
	specificErr := errors.New("specific error")

	fn := func(ctx context.Context) error {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			return specificErr
		}
		return nil
	}

	// Custom retryable function that only retries specific error
	isRetryable := func(err error) bool {
		return errors.Is(err, specificErr)
	}

	err := DoWithRetryable(ctx, config, fn, isRetryable)
	if err != nil {
		t.Errorf("expected success after retries, got: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestConfigNormalize(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		wantError bool
		checkFunc func(Config) bool
	}{
		{
			name:      "zero attempts",
			config:    Config{MaxAttempts: 0},
			wantError: true,
		},
		{
			name:      "zero initial delay",
			config:    Config{MaxAttempts: 1, InitialDelay: 0},
			wantError: true,
		},
		{
			name: "auto-set min delay",
			config: Config{
				MaxAttempts:  1,
				InitialDelay: 100 * time.Millisecond,
				Multiplier:   2.0, // need to set explicitly
			},
			checkFunc: func(c Config) bool {
				return c.MinDelay == c.InitialDelay
			},
		},
		{
			name: "multiplier too small",
			config: Config{
				MaxAttempts:  1,
				InitialDelay: 100 * time.Millisecond,
				Multiplier:   0.5,
			},
			wantError: true,
		},
		{
			name: "valid config",
			config: Config{
				MaxAttempts:  3,
				InitialDelay: 100 * time.Millisecond,
				MaxDelay:     1 * time.Second,
				Multiplier:   2.0,
			},
			checkFunc: func(c Config) bool {
				return c.Rand != nil && c.Now != nil && c.After != nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Normalize()
			if tt.wantError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantError && tt.checkFunc != nil && !tt.checkFunc(tt.config) {
				t.Error("config check failed")
			}
		})
	}
}

func TestMaxElapsedTime(t *testing.T) {
	config := Config{
		MaxAttempts:    10,
		InitialDelay:   10 * time.Millisecond,
		MaxDelay:       50 * time.Millisecond,
		MaxElapsedTime: 100 * time.Millisecond,
		Multiplier:     2.0,
		JitterStrategy: JitterNone,
	}

	var attempts int32
	temporaryErr := customError{"temporary failure", true}
	fn := func(ctx context.Context) error {
		atomic.AddInt32(&attempts, 1)
		return temporaryErr
	}

	start := time.Now()
	err := Do(context.Background(), config, fn)
	elapsed := time.Since(start)

	var retryErr *RetriesExceededError
	if !errors.As(err, &retryErr) {
		t.Fatalf("expected RetriesExceededError, got %T", err)
	}

	if retryErr.Reason != "max elapsed time exceeded" {
		t.Errorf("expected 'max elapsed time exceeded', got %q", retryErr.Reason)
	}

	// Should stop before 10 attempts due to time limit
	if attempts >= 10 {
		t.Errorf("expected fewer than 10 attempts, got %d", attempts)
	}

	// Should respect the time budget approximately
	if elapsed > 200*time.Millisecond {
		t.Errorf("took too long: %v", elapsed)
	}
}

func TestOnRetryCallback(t *testing.T) {
	var callbackAttempts []int
	var callbackErrors []error
	var callbackDelays []time.Duration

	config := Config{
		MaxAttempts:    3,
		InitialDelay:   1 * time.Millisecond,
		MaxDelay:       10 * time.Millisecond,
		Multiplier:     2.0,
		JitterStrategy: JitterNone,
		OnRetry: func(attempt int, err error, delay time.Duration) {
			callbackAttempts = append(callbackAttempts, attempt)
			callbackErrors = append(callbackErrors, err)
			callbackDelays = append(callbackDelays, delay)
		},
	}

	var attempts int32
	expectedErr := customError{"temporary failure", true}
	fn := func(ctx context.Context) error {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			return expectedErr
		}
		return nil // success on 3rd attempt
	}

	err := Do(context.Background(), config, fn)
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}

	if len(callbackAttempts) != 2 {
		t.Errorf("expected 2 callback calls, got %d", len(callbackAttempts))
	}

	for i, attempt := range callbackAttempts {
		if attempt != i+1 {
			t.Errorf("callback[%d]: expected attempt %d, got %d", i, i+1, attempt)
		}
		if !errors.Is(callbackErrors[i], expectedErr) {
			t.Errorf("callback[%d]: expected error %v, got %v", i, expectedErr, callbackErrors[i])
		}
	}
}

func TestEnhancedRetryableErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"io.EOF", io.EOF, true},
		{"io.ErrUnexpectedEOF", io.ErrUnexpectedEOF, true},
		{"net.ErrClosed", net.ErrClosed, true},
		{"url error with timeout", &url.Error{
			Op:  "Get",
			URL: "http://example.com",
			Err: &net.OpError{Op: "dial", Err: &os.SyscallError{Syscall: "connect", Err: syscall.ETIMEDOUT}},
		}, true},
		{"dns temporary error", &url.Error{
			Op:  "Get",
			URL: "http://example.com",
			Err: &net.DNSError{IsTemporary: true},
		}, true},
		{"regular error", errors.New("regular"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DefaultRetryable(tt.err)
			if result != tt.expected {
				t.Errorf("DefaultRetryable(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestRetriesExceededError(t *testing.T) {
	config := Config{
		MaxAttempts:  2,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
	}

	originalErr := customError{"temporary failure", true}
	fn := func(ctx context.Context) error {
		return originalErr
	}

	err := Do(context.Background(), config, fn)

	var retryErr *RetriesExceededError
	if !errors.As(err, &retryErr) {
		t.Fatalf("expected RetriesExceededError, got %T", err)
	}

	if retryErr.Attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", retryErr.Attempts)
	}

	if !errors.Is(err, originalErr) {
		t.Error("should be able to unwrap to original error")
	}

	if retryErr.TotalDuration <= 0 {
		t.Error("total duration should be positive")
	}

	errorMsg := retryErr.Error()
	if errorMsg == "" {
		t.Error("error message should not be empty")
	}
}
