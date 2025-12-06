package pg

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolOptions содержит настройки для пула подключений PostgreSQL.
type PoolOptions struct {
	// MaxConns - максимальное количество соединений в пуле
	MaxConns int32
	// MinConns - минимальное количество соединений в пуле
	MinConns int32
	// HealthCheckPeriod - интервал проверки здоровья соединений
	HealthCheckPeriod time.Duration
	// MaxConnLifetime - максимальное время жизни соединения
	MaxConnLifetime time.Duration
	// MaxConnIdleTime - максимальное время простоя соединения
	MaxConnIdleTime time.Duration
	// PingTimeout - таймаут для проверки соединения при создании пула
	PingTimeout time.Duration
}

// DefaultPoolOptions возвращает настройки по умолчанию, оптимизированные для Telegram-бота.
func DefaultPoolOptions() PoolOptions {
	return PoolOptions{
		MaxConns:          20,
		MinConns:          2,
		HealthCheckPeriod: 30 * time.Second,
		MaxConnLifetime:   time.Hour,
		MaxConnIdleTime:   10 * time.Minute,
		PingTimeout:       5 * time.Second,
	}
}

// NewPool создает новый пул подключений к PostgreSQL с настройками по умолчанию.
// Параметры пула оптимизированы для типичной нагрузки Telegram-бота.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	return NewPoolWithOptions(ctx, dsn, DefaultPoolOptions())
}

// NewPoolWithOptions создает новый пул подключений к PostgreSQL с заданными параметрами.
func NewPoolWithOptions(ctx context.Context, dsn string, opts PoolOptions) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}

	// Применяем настройки из опций
	cfg.MaxConns = opts.MaxConns
	cfg.MinConns = opts.MinConns
	cfg.HealthCheckPeriod = opts.HealthCheckPeriod
	cfg.MaxConnLifetime = opts.MaxConnLifetime
	cfg.MaxConnIdleTime = opts.MaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Проверяем соединение с БД с настраиваемым таймаутом
	pingCtx, cancel := context.WithTimeout(ctx, opts.PingTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}
