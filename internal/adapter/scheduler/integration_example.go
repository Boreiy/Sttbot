package scheduler

import (
	"context"
	"log/slog"
	"time"
)

// IntegrationExample демонстрирует интеграцию планировщика в основное приложение.
// Этот файл служит документацией и примером для разработчиков.

// StartScheduler инициализирует и запускает планировщик с типичными задачами для бота.
// Возвращает планировщик для управления и функцию остановки.
func StartScheduler(ctx context.Context, logger *slog.Logger) (*Scheduler, func()) {
	// Настраиваем хуки для мониторинга
	hooks := JobHooks{
		OnJobStart: func(jobName string) {
			logger.Debug("job started", "job", jobName)
		},
		OnJobFinish: func(jobName string, duration time.Duration, err error) {
			if err != nil {
				logger.Warn("job failed", "job", jobName, "duration", duration, "error", err)
			} else {
				logger.Debug("job completed", "job", jobName, "duration", duration)
			}
		},
		OnJobError: func(jobName string, err error) {
			logger.Error("job error", "job", jobName, "error", err)
		},
	}

	scheduler := NewWithContext(ctx, Config{
		Logger:   logger,
		JobHooks: hooks,
	})

	// Настраиваем типичные задачи для бота
	if err := setupBotJobs(scheduler, logger); err != nil {
		logger.Error("failed to setup bot jobs", "error", err)
		return nil, nil
	}

	// Запускаем планировщик
	scheduler.Start()
	logger.Info("scheduler started successfully")

	// Возвращаем функцию остановки с таймаутом
	stopFunc := func() {
		logger.Info("gracefully shutting down scheduler")

		// Пытаемся остановить в течение 30 секунд
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := scheduler.StopContext(ctx); err != nil {
			logger.Warn("scheduler stop deadline exceeded", "error", err)
		}
	}

	return scheduler, stopFunc
}

// setupBotJobs настраивает типичные задачи для телеграм-бота.
func setupBotJobs(scheduler *Scheduler, logger *slog.Logger) error {
	// Очистка временных файлов каждые 6 часов
	if _, err := scheduler.AddCronJobWithOptions("0 0 */6 * * *", func(ctx context.Context) error {
		logger.Info("starting temp files cleanup")
		// Здесь была бы логика очистки временных файлов
		return nil
	}, JobOptions{
		Name:          "temp-cleanup",
		Timeout:       10 * time.Minute,
		OverlapPolicy: SkipIfRunning,
	}); err != nil {
		return err
	}

	// Обновление статистики пользователей каждые 30 минут
	if _, err := scheduler.AddCronJobWithOptions("0 */30 * * * *", func(ctx context.Context) error {
		logger.Info("updating user statistics")
		// Здесь была бы логика обновления статистики
		return nil
	}, JobOptions{
		Name:          "user-stats",
		Timeout:       5 * time.Minute,
		OverlapPolicy: DelayIfRunning,
	}); err != nil {
		return err
	}

	// Отправка ежедневных уведомлений в 9:00
	if _, err := scheduler.AddCronJobWithOptions("0 0 9 * * *", func(ctx context.Context) error {
		logger.Info("sending daily notifications")
		// Здесь была бы логика отправки уведомлений
		return nil
	}, JobOptions{
		Name:          "daily-notifications",
		Timeout:       30 * time.Minute,
		OverlapPolicy: SkipIfRunning,
	}); err != nil {
		return err
	}

	// Проверка состояния внешних сервисов каждую минуту
	scheduler.AddTickerJobWithOptions(1*time.Minute, func(ctx context.Context) error {
		// Здесь была бы логика проверки health-check'ов
		return nil
	}, JobOptions{
		Name:          "health-check",
		Timeout:       30 * time.Second,
		OverlapPolicy: SkipIfRunning,
	})

	// Сбор метрик каждые 5 минут
	scheduler.AddTickerJobWithOptions(5*time.Minute, func(ctx context.Context) error {
		// Здесь была бы логика сбора метрик для мониторинга
		return nil
	}, JobOptions{
		Name:          "metrics-collection",
		Timeout:       2 * time.Minute,
		OverlapPolicy: AllowOverlap,
	})

	return nil
}

// Пример интеграции в cmd/bot/main.go:
//
// func main() {
//     // ... инициализация логгера, конфига и т.д.
//
//     ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
//     defer cancel()
//
//     // Запуск планировщика
//     _, stopScheduler := scheduler.StartScheduler(ctx, logger)
//     if stopScheduler != nil {
//         defer stopScheduler()
//     }
//
//     // ... запуск бота и других сервисов
//
//     <-ctx.Done()
//     logger.Info("shutting down...")
// }

// Примеры cron-расписаний:
//
// Секунда Минута Час День Месяц ДеньНедели
// *       *      *   *    *     *           - каждую секунду
// 0       *      *   *    *     *           - каждую минуту
// 0       0      *   *    *     *           - каждый час
// 0       0      9   *    *     *           - каждый день в 9:00
// 0       0      9   *    *     1           - каждый понедельник в 9:00
// 0       */15   *   *    *     *           - каждые 15 минут
// 0       0      */6 *    *     *           - каждые 6 часов
// 0       0      2   *    *     *           - каждый день в 2:00 ночи
//
// Предопределенные расписания:
// @yearly   = 0 0 0 1 1 * (или @annually)
// @monthly  = 0 0 0 1 * *
// @weekly   = 0 0 0 * * 0
// @daily    = 0 0 0 * * * (или @midnight)
// @hourly   = 0 0 * * * *
// @every <duration> (например: @every 5m)
