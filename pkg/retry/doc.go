// Package retry provides robust retry logic with exponential backoff, jitter strategies,
// and comprehensive error handling for Go applications.
//
// Key Features:
//   - Multiple jitter strategies (None, Equal, Decorrelated)
//   - Configurable time and attempt limits
//   - Rich network error detection
//   - Observability hooks (OnRetry callback)
//   - Custom delay policies (NextDelay override)
//   - Full testability support (time abstraction)
//   - Detailed error reporting
//
// Basic Usage:
//
//	err := retry.Retry(ctx, func(ctx context.Context) error {
//	    return someNetworkOperation()
//	})
//
// Advanced Configuration:
//
//	config := retry.Config{
//	    MaxAttempts:    5,
//	    InitialDelay:   200 * time.Millisecond,
//	    MaxDelay:       10 * time.Second,
//	    MaxElapsedTime: 60 * time.Second,
//	    JitterStrategy: retry.JitterDecorrelated,
//	    OnRetry: func(attempt int, err error, delay time.Duration) {
//	        log.Printf("Retry %d after %v: %v", attempt, delay, err)
//	    },
//	}
//	err := retry.Do(ctx, config, fn)
//
// Custom Retry Logic:
//
//	config := retry.DefaultConfig()
//	config.NextDelay = func(attempt int, err error) (time.Duration, bool) {
//	    if attempt > 3 {
//	        return 0, false // stop retrying
//	    }
//	    return time.Second * time.Duration(attempt), true
//	}
//
// For HTTP-specific retry logic, consider using internal/platform/httpclient
// which provides HTTP status code awareness and Retry-After header support.
package retry
