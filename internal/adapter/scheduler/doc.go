// Package scheduler provides background job processing with cron-based scheduling
// and interval-based ticker jobs.
//
// Features:
//   - Cron-style scheduling using github.com/robfig/cron/v3
//   - Simple interval-based jobs with time.Ticker
//   - Job overlap control policies (Allow/Skip/Delay)
//   - Per-job timeouts and named jobs
//   - Job ID management with add/remove capabilities
//   - Parent context support for lifecycle management
//   - Graceful shutdown with optional deadline (StopContext)
//   - Idempotent Start/Stop operations
//   - Error handling and panic recovery
//   - Structured logging with slog integration
//   - Optional hooks for observability
//
// Basic usage:
//
//	scheduler := New(Config{Logger: logger})
//
//	// Add cron job with ID
//	cronID, err := scheduler.AddCronJob("@hourly", func(ctx context.Context) error {
//		// Your periodic task here
//		return nil
//	})
//
//	// Add ticker job with options
//	tickerID := scheduler.AddTickerJobWithOptions(5*time.Minute, func(ctx context.Context) error {
//		// Your interval-based task here
//		return nil
//	}, JobOptions{
//		Name:          "cleanup-task",
//		Timeout:       30*time.Second,
//		OverlapPolicy: SkipIfRunning,
//	})
//
//	scheduler.Start()
//	defer scheduler.Stop()
//
//	// Remove jobs when needed
//	scheduler.RemoveCronJob(cronID)
//	scheduler.RemoveTickerJob(tickerID)
//
// Advanced usage with parent context and hooks:
//
//	hooks := JobHooks{
//		OnJobStart: func(jobName string) {
//			log.Printf("Job %s started", jobName)
//		},
//		OnJobFinish: func(jobName string, duration time.Duration, err error) {
//			log.Printf("Job %s finished in %v (error: %v)", jobName, duration, err)
//		},
//	}
//
//	scheduler := NewWithContext(parentCtx, Config{
//		Logger:   logger,
//		JobHooks: hooks,
//	})
//
// Overlap policies:
//   - AllowOverlap: Jobs can run concurrently (default)
//   - SkipIfRunning: Skip execution if previous run is still active
//   - DelayIfRunning: Wait for previous run to finish before starting
//
// Cron schedule examples:
//   - "@hourly" - every hour
//   - "@daily" - every day at midnight
//   - "@every 5m" - every 5 minutes
//   - "0 30 * * * *" - every 30 minutes
//   - "0 0 9 * * 1" - every Monday at 9:00 AM
//
// The scheduler ensures that:
//   - Jobs respect configured overlap policies
//   - Panics are recovered and logged
//   - Errors are logged but don't stop the scheduler
//   - Context cancellation stops all jobs gracefully
//   - Start/Stop operations are idempotent and thread-safe
//   - Graceful shutdown can be bounded with StopContext
package scheduler
