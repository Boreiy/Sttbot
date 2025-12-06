package scheduler

import (
	"context"
	"log/slog"
	"time"
)

// ExampleJobs содержит примеры типичных задач для планировщика.
type ExampleJobs struct {
	logger *slog.Logger
}

// NewExampleJobs создает новый экземпляр с примерами задач.
func NewExampleJobs(logger *slog.Logger) *ExampleJobs {
	if logger == nil {
		logger = slog.Default()
	}
	return &ExampleJobs{logger: logger}
}

// CleanTempData - пример задачи очистки временных данных.
func (e *ExampleJobs) CleanTempData(ctx context.Context) error {
	e.logger.Info("starting temp data cleanup")

	// Проверяем, не отменен ли контекст
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Здесь была бы логика очистки временных файлов, кеша и т.д.
	// Симулируем работу
	select {
	case <-time.After(100 * time.Millisecond):
		e.logger.Info("temp data cleanup completed")
		return nil
	case <-ctx.Done():
		e.logger.Warn("temp data cleanup cancelled")
		return ctx.Err()
	}
}

// SyncStats - пример задачи синхронизации статистики.
func (e *ExampleJobs) SyncStats(ctx context.Context) error {
	e.logger.Info("starting stats synchronization")

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Здесь была бы логика синхронизации со внешними системами
	select {
	case <-time.After(200 * time.Millisecond):
		e.logger.Info("stats synchronization completed")
		return nil
	case <-ctx.Done():
		e.logger.Warn("stats synchronization cancelled")
		return ctx.Err()
	}
}

// SendNotifications - пример задачи отправки уведомлений.
func (e *ExampleJobs) SendNotifications(ctx context.Context) error {
	e.logger.Info("starting notification sending")

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Здесь была бы логика отправки push-уведомлений, email и т.д.
	select {
	case <-time.After(150 * time.Millisecond):
		e.logger.Info("notifications sent successfully")
		return nil
	case <-ctx.Done():
		e.logger.Warn("notification sending cancelled")
		return ctx.Err()
	}
}

// DatabaseMaintenance - пример задачи обслуживания базы данных.
func (e *ExampleJobs) DatabaseMaintenance(ctx context.Context) error {
	e.logger.Info("starting database maintenance")

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Здесь была бы логика VACUUM, ANALYZE, очистки старых записей и т.д.
	select {
	case <-time.After(500 * time.Millisecond):
		e.logger.Info("database maintenance completed")
		return nil
	case <-ctx.Done():
		e.logger.Warn("database maintenance cancelled")
		return ctx.Err()
	}
}

// HealthCheck - пример задачи проверки состояния системы.
func (e *ExampleJobs) HealthCheck(ctx context.Context) error {
	e.logger.Debug("performing health check")

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Здесь была бы логика проверки доступности внешних сервисов,
	// состояния БД, свободного места на диске и т.д.
	select {
	case <-time.After(50 * time.Millisecond):
		e.logger.Debug("health check passed")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SetupExampleJobs настраивает типичные задачи в планировщике.
// Это пример того, как можно организовать инициализацию планировщика.
func SetupExampleJobs(scheduler *Scheduler, logger *slog.Logger) error {
	jobs := NewExampleJobs(logger)

	// Очистка временных данных каждый час
	if _, err := scheduler.AddCronJob("0 0 * * * *", jobs.CleanTempData); err != nil {
		return err
	}

	// Синхронизация статистики каждые 15 минут
	if _, err := scheduler.AddCronJob("0 */15 * * * *", jobs.SyncStats); err != nil {
		return err
	}

	// Обслуживание БД каждый день в 2:00
	if _, err := scheduler.AddCronJob("0 0 2 * * *", jobs.DatabaseMaintenance); err != nil {
		return err
	}

	// Отправка уведомлений каждые 5 минут
	scheduler.AddTickerJob(5*time.Minute, jobs.SendNotifications)

	// Проверка здоровья системы каждые 30 секунд
	scheduler.AddTickerJob(30*time.Second, jobs.HealthCheck)

	logger.Info("example jobs configured successfully")
	return nil
}
