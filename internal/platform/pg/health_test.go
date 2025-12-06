package pg

import (
	"context"
	"testing"
	"time"
)

func TestDefaultHealthCheckOptions(t *testing.T) {
	t.Parallel()

	opts := DefaultHealthCheckOptions()

	if opts.MaxRetries != 10 {
		t.Errorf("expected MaxRetries=10, got %d", opts.MaxRetries)
	}
	if opts.InitialInterval != time.Second {
		t.Errorf("expected InitialInterval=1s, got %v", opts.InitialInterval)
	}
	if opts.MaxInterval != 30*time.Second {
		t.Errorf("expected MaxInterval=30s, got %v", opts.MaxInterval)
	}
	if opts.Strategy != ExponentialWait {
		t.Errorf("expected Strategy=ExponentialWait, got %v", opts.Strategy)
	}
	if opts.PingTimeout != 5*time.Second {
		t.Errorf("expected PingTimeout=5s, got %v", opts.PingTimeout)
	}
}

func TestCalculateNextInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		currentInterval time.Duration
		opts            HealthCheckOptions
		expected        time.Duration
	}{
		{
			name:            "linear_increase",
			currentInterval: 1 * time.Second,
			opts: HealthCheckOptions{
				Strategy:        LinearWait,
				InitialInterval: 1 * time.Second,
				MaxInterval:     10 * time.Second,
			},
			expected: 2 * time.Second,
		},
		{
			name:            "linear_max_limit",
			currentInterval: 9 * time.Second,
			opts: HealthCheckOptions{
				Strategy:        LinearWait,
				InitialInterval: 2 * time.Second,
				MaxInterval:     10 * time.Second,
			},
			expected: 10 * time.Second,
		},
		{
			name:            "exponential_increase",
			currentInterval: 2 * time.Second,
			opts: HealthCheckOptions{
				Strategy:    ExponentialWait,
				MaxInterval: 30 * time.Second,
			},
			expected: 4 * time.Second,
		},
		{
			name:            "exponential_max_limit",
			currentInterval: 20 * time.Second,
			opts: HealthCheckOptions{
				Strategy:    ExponentialWait,
				MaxInterval: 30 * time.Second,
			},
			expected: 30 * time.Second,
		},
		{
			name:            "unknown_strategy_defaults",
			currentInterval: 5 * time.Second,
			opts: HealthCheckOptions{
				Strategy:        WaitStrategy(999), // неизвестная стратегия
				InitialInterval: 2 * time.Second,
			},
			expected: 2 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := calculateNextInterval(tt.currentInterval, tt.opts)
			if result != tt.expected {
				t.Errorf("calculateNextInterval() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestHealthCheck_InvalidDSN(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := HealthCheck(ctx, "invalid-dsn")
	if err == nil {
		t.Error("expected error for invalid DSN, got nil")
	}
}

func TestHealthCheck_UnreachableDB(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Используем несуществующий хост
	dsn := "postgres://user:pass@localhost:9999/nonexistent?sslmode=disable"
	err := HealthCheck(ctx, dsn)
	if err == nil {
		t.Error("expected error for unreachable database, got nil")
	}
}

func TestHealthCheckPool_NilPool(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	err := HealthCheckPool(ctx, nil)
	if err == nil {
		t.Error("expected error for nil pool, got nil")
	}
	if err.Error() != "pool is nil" {
		t.Errorf("expected 'pool is nil' error, got %q", err.Error())
	}
}

func TestWaitForDB_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	opts := HealthCheckOptions{
		MaxRetries:      0, // Бесконечно до таймаута
		InitialInterval: 50 * time.Millisecond,
		Strategy:        LinearWait,
		PingTimeout:     10 * time.Millisecond,
	}

	dsn := "postgres://user:pass@localhost:9999/nonexistent?sslmode=disable"
	err := WaitForDB(ctx, dsn, opts)

	if err == nil {
		t.Error("expected error due to context cancellation, got nil")
	}

	// Проверяем, что ошибка связана с контекстом
	if ctx.Err() == nil {
		t.Error("context should be cancelled")
	}
}

func TestWaitForDB_MaxRetries(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	opts := HealthCheckOptions{
		MaxRetries:      2, // Только 2 попытки
		InitialInterval: 10 * time.Millisecond,
		Strategy:        LinearWait,
		PingTimeout:     10 * time.Millisecond,
	}

	dsn := "postgres://user:pass@localhost:9999/nonexistent?sslmode=disable"
	start := time.Now()
	err := WaitForDB(ctx, dsn, opts)
	duration := time.Since(start)

	if err == nil {
		t.Error("expected error due to max retries exceeded, got nil")
	}

	// Проверяем, что функция завершилась быстро (не ждала долго)
	if duration > 200*time.Millisecond {
		t.Errorf("function took too long: %v, expected under 200ms", duration)
	}
}

func TestWaitForDBSimple_InvalidDSN(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	timeout := 100 * time.Millisecond

	err := WaitForDBSimple(ctx, "invalid-dsn", timeout)
	if err == nil {
		t.Error("expected error for invalid DSN, got nil")
	}
}

func TestWaitForDBWithRetries_Legacy(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	timeout := 100 * time.Millisecond
	interval := 20 * time.Millisecond

	dsn := "postgres://user:pass@localhost:9999/nonexistent?sslmode=disable"
	err := WaitForDBWithRetries(ctx, dsn, timeout, interval)

	if err == nil {
		t.Error("expected error for unreachable database, got nil")
	}
}

func TestGetPoolStats_NilPool(t *testing.T) {
	t.Parallel()

	stats := GetPoolStats(nil)

	// Все поля должны быть нулевыми для nil пула
	if stats.MaxConns != 0 {
		t.Errorf("expected MaxConns=0 for nil pool, got %d", stats.MaxConns)
	}
	if stats.OpenConns != 0 {
		t.Errorf("expected OpenConns=0 for nil pool, got %d", stats.OpenConns)
	}
}

func TestIsHealthy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		stats    DBStats
		expected bool
	}{
		{
			name: "healthy_pool",
			stats: DBStats{
				MaxConns:  10,
				OpenConns: 5,
				InUse:     2,
				Idle:      3,
			},
			expected: true,
		},
		{
			name: "no_max_conns",
			stats: DBStats{
				MaxConns:  0,
				OpenConns: 5,
				InUse:     2,
			},
			expected: false,
		},
		{
			name: "no_open_conns",
			stats: DBStats{
				MaxConns:  10,
				OpenConns: 0,
				InUse:     0,
			},
			expected: false,
		},
		{
			name: "high_utilization",
			stats: DBStats{
				MaxConns:  10,
				OpenConns: 10,
				InUse:     10, // 100% утилизация
			},
			expected: false,
		},
		{
			name: "acceptable_utilization",
			stats: DBStats{
				MaxConns:  10,
				OpenConns: 8,
				InUse:     8, // 80% утилизация
			},
			expected: true,
		},
		{
			name: "border_utilization",
			stats: DBStats{
				MaxConns:  10,
				OpenConns: 9,
				InUse:     9, // 90% утилизация - граница
			},
			expected: true,
		},
		{
			name: "over_border_utilization",
			stats: DBStats{
				MaxConns:  10,
				OpenConns: 10,
				InUse:     10, // >90% утилизация (100%)
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := IsHealthy(tt.stats)
			if result != tt.expected {
				t.Errorf("IsHealthy() = %v, want %v for stats %+v", result, tt.expected, tt.stats)
			}
		})
	}
}

func TestDBStats_Structure(t *testing.T) {
	t.Parallel()

	// Проверяем, что структура DBStats корректно инициализируется
	stats := DBStats{
		MaxConns:        20,
		OpenConns:       10,
		InUse:           5,
		Idle:            5,
		WaitCount:       100,
		WaitDuration:    time.Second,
		MaxIdleDestroys: 2,
		MaxLifeCloses:   1,
	}

	if stats.MaxConns != 20 {
		t.Errorf("expected MaxConns=20, got %d", stats.MaxConns)
	}
	if stats.WaitDuration != time.Second {
		t.Errorf("expected WaitDuration=1s, got %v", stats.WaitDuration)
	}
}

// Интеграционные тесты требуют реальной БД
func TestHealthCheck_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// TODO: Реализовать с testcontainers для полной изоляции
	t.Skip("integration test requires real PostgreSQL database")

	// Пример структуры интеграционного теста:
	// dsn := "postgres://test:test@localhost:5432/test?sslmode=disable"
	// ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	// defer cancel()
	//
	// err := HealthCheck(ctx, dsn)
	// if err != nil {
	//     t.Fatalf("HealthCheck failed: %v", err)
	// }
}
