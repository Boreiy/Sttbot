package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// WaitStrategy определяет стратегию ожидания между попытками подключения.
type WaitStrategy int

const (
	// LinearWait - линейная задержка между попытками
	LinearWait WaitStrategy = iota
	// ExponentialWait - экспоненциальная задержка между попытками
	ExponentialWait
)

// HealthCheckOptions содержит опции для проверки здоровья БД.
type HealthCheckOptions struct {
	// MaxRetries - максимальное количество попыток (0 = бесконечно до таймаута контекста)
	MaxRetries int
	// InitialInterval - начальная задержка между попытками
	InitialInterval time.Duration
	// MaxInterval - максимальная задержка между попытками (для экспоненциальной стратегии)
	MaxInterval time.Duration
	// Strategy - стратегия ожидания между попытками
	Strategy WaitStrategy
	// PingTimeout - таймаут для каждой попытки ping
	PingTimeout time.Duration
}

// DefaultHealthCheckOptions возвращает опции по умолчанию для проверки здоровья БД.
func DefaultHealthCheckOptions() HealthCheckOptions {
	return HealthCheckOptions{
		MaxRetries:      10,
		InitialInterval: 1 * time.Second,
		MaxInterval:     30 * time.Second,
		Strategy:        ExponentialWait,
		PingTimeout:     5 * time.Second,
	}
}

// WaitForDB ожидает доступности базы данных с настраиваемой стратегией повторов.
// Возвращает nil при успешном подключении или ошибку при превышении лимитов.
//
// Параметры:
//   - ctx: контекст с общим таймаутом ожидания
//   - dsn: строка подключения к БД
//   - opts: опции проверки здоровья
func WaitForDB(ctx context.Context, dsn string, opts HealthCheckOptions) error {
	attempt := 0
	interval := opts.InitialInterval

	for {
		// Проверяем контекст перед попыткой
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for database: %w", ctx.Err())
		default:
		}

		attempt++

		// Пытаемся подключиться
		err := pingDatabase(ctx, dsn, opts.PingTimeout)
		if err == nil {
			return nil // Успешное подключение
		}

		// Проверяем лимит попыток
		if opts.MaxRetries > 0 && attempt >= opts.MaxRetries {
			return fmt.Errorf("database not available after %d attempts: %w", attempt, err)
		}

		// Ждем перед следующей попыткой
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled after %d attempts: %w", attempt, ctx.Err())
		case <-time.After(interval):
			// Продолжаем
		}

		// Рассчитываем следующий интервал
		interval = calculateNextInterval(interval, opts)
	}
}

// WaitForDBSimple - упрощенная версия WaitForDB с параметрами по умолчанию.
// Ожидает доступности БД с экспоненциальной задержкой до общего таймаута.
func WaitForDBSimple(ctx context.Context, dsn string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	opts := DefaultHealthCheckOptions()
	opts.MaxRetries = 0 // Бесконечно до таймаута контекста

	return WaitForDB(ctx, dsn, opts)
}

// HealthCheck выполняет разовую проверку доступности БД.
// Возвращает nil если БД доступна, иначе ошибку с деталями.
func HealthCheck(ctx context.Context, dsn string) error {
	return pingDatabase(ctx, dsn, 5*time.Second)
}

// HealthCheckPool выполняет проверку здоровья существующего пула подключений.
func HealthCheckPool(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return fmt.Errorf("pool is nil")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("pool ping failed: %w", err)
	}

	// Дополнительная проверка: выполняем простой запрос
	var result int
	err := pool.QueryRow(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		return fmt.Errorf("simple query failed: %w", err)
	}

	if result != 1 {
		return fmt.Errorf("unexpected query result: got %d, want 1", result)
	}

	return nil
}

// pingDatabase выполняет пинг БД с созданием временного подключения.
func pingDatabase(ctx context.Context, dsn string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("failed to create pool: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	return nil
}

// calculateNextInterval вычисляет следующий интервал ожидания на основе стратегии.
func calculateNextInterval(currentInterval time.Duration, opts HealthCheckOptions) time.Duration {
	switch opts.Strategy {
	case LinearWait:
		// Линейное увеличение: добавляем начальный интервал
		next := currentInterval + opts.InitialInterval
		if next > opts.MaxInterval {
			return opts.MaxInterval
		}
		return next

	case ExponentialWait:
		// Экспоненциальное увеличение: удваиваем интервал
		next := currentInterval * 2
		if next > opts.MaxInterval {
			return opts.MaxInterval
		}
		return next

	default:
		return opts.InitialInterval
	}
}

// DBStats содержит статистику подключений к БД.
type DBStats struct {
	MaxConns        int32         // Максимальное количество подключений
	OpenConns       int32         // Текущее количество открытых подключений
	InUse           int32         // Количество подключений в использовании
	Idle            int32         // Количество простаивающих подключений
	WaitCount       int64         // Количество ожиданий подключения
	WaitDuration    time.Duration // Общее время ожидания
	MaxIdleDestroys int64         // Количество закрытых idle подключений
	MaxLifeCloses   int64         // Количество закрытых подключений по lifetime
}

// GetPoolStats возвращает статистику пула подключений.
func GetPoolStats(pool *pgxpool.Pool) DBStats {
	if pool == nil {
		return DBStats{}
	}

	stats := pool.Stat()

	return DBStats{
		MaxConns:        stats.MaxConns(),
		OpenConns:       stats.TotalConns(),
		InUse:           stats.AcquiredConns(),
		Idle:            stats.IdleConns(),
		WaitCount:       stats.EmptyAcquireCount(),
		WaitDuration:    stats.AcquireDuration(),
		MaxIdleDestroys: stats.CanceledAcquireCount(),
		MaxLifeCloses:   int64(stats.ConstructingConns()),
	}
}

// IsHealthy проверяет, здоров ли пул на основе его статистики.
// Возвращает true если пул работает нормально, false если есть проблемы.
func IsHealthy(stats DBStats) bool {
	// Базовые проверки работоспособности
	if stats.MaxConns == 0 {
		return false // Пул не настроен
	}

	if stats.OpenConns == 0 {
		return false // Нет открытых подключений
	}

	// Проверяем, что не все подключения заняты (оставляем запас)
	utilizationPercent := float64(stats.InUse) / float64(stats.MaxConns) * 100
	if utilizationPercent > 90 {
		return false // Слишком высокая нагрузка
	}

	return true
}

// WaitForDBWithRetries - legacy функция для обратной совместимости.
// DEPRECATED: используйте WaitForDB с HealthCheckOptions.
func WaitForDBWithRetries(ctx context.Context, dsn string, timeout, interval time.Duration) error {
	opts := HealthCheckOptions{
		MaxRetries:      0, // До таймаута контекста
		InitialInterval: interval,
		MaxInterval:     interval * 10,
		Strategy:        LinearWait,
		PingTimeout:     5 * time.Second,
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return WaitForDB(ctx, dsn, opts)
}
