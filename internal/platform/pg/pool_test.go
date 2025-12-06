package pg

import (
	"context"
	"testing"
	"time"
)

func TestDefaultPoolOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		field    string
		expected interface{}
		actual   func(PoolOptions) interface{}
	}{
		{
			field:    "MaxConns",
			expected: int32(20),
			actual:   func(opts PoolOptions) interface{} { return opts.MaxConns },
		},
		{
			field:    "MinConns",
			expected: int32(2),
			actual:   func(opts PoolOptions) interface{} { return opts.MinConns },
		},
		{
			field:    "HealthCheckPeriod",
			expected: 30 * time.Second,
			actual:   func(opts PoolOptions) interface{} { return opts.HealthCheckPeriod },
		},
		{
			field:    "MaxConnLifetime",
			expected: time.Hour,
			actual:   func(opts PoolOptions) interface{} { return opts.MaxConnLifetime },
		},
		{
			field:    "MaxConnIdleTime",
			expected: 10 * time.Minute,
			actual:   func(opts PoolOptions) interface{} { return opts.MaxConnIdleTime },
		},
		{
			field:    "PingTimeout",
			expected: 5 * time.Second,
			actual:   func(opts PoolOptions) interface{} { return opts.PingTimeout },
		},
	}

	opts := DefaultPoolOptions()

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			t.Parallel()

			actual := tt.actual(opts)
			if actual != tt.expected {
				t.Errorf("%s = %v, want %v", tt.field, actual, tt.expected)
			}
		})
	}
}

func TestNewPool_ErrorCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		dsn         string
		setupOpts   func() *PoolOptions
		expectError bool
		testDesc    string
	}{
		{
			name:        "invalid_dsn",
			dsn:         "invalid-dsn",
			setupOpts:   func() *PoolOptions { return nil },
			expectError: true,
			testDesc:    "should fail with invalid DSN",
		},
		{
			name:        "unreachable_database",
			dsn:         "postgres://user:pass@localhost:9999/nonexistent?sslmode=disable",
			setupOpts:   func() *PoolOptions { return nil },
			expectError: true,
			testDesc:    "should fail with unreachable database",
		},
		{
			name: "invalid_dsn_with_options",
			dsn:  "invalid-dsn",
			setupOpts: func() *PoolOptions {
				opts := DefaultPoolOptions()
				return &opts
			},
			expectError: true,
			testDesc:    "should fail with invalid DSN even with valid options",
		},
		{
			name: "custom_options_unreachable_db",
			dsn:  "postgres://user:pass@localhost:9999/nonexistent?sslmode=disable",
			setupOpts: func() *PoolOptions {
				return &PoolOptions{
					MaxConns:          10,
					MinConns:          1,
					HealthCheckPeriod: 60 * time.Second,
					MaxConnLifetime:   2 * time.Hour,
					MaxConnIdleTime:   5 * time.Minute,
					PingTimeout:       3 * time.Second,
				}
			},
			expectError: true,
			testDesc:    "should fail with unreachable DB but not panic with custom options",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			var err error
			if opts := tt.setupOpts(); opts != nil {
				_, err = NewPoolWithOptions(ctx, tt.dsn, *opts)
			} else {
				_, err = NewPool(ctx, tt.dsn)
			}

			if tt.expectError && err == nil {
				t.Errorf("%s: expected error but got nil", tt.testDesc)
			} else if !tt.expectError && err != nil {
				t.Errorf("%s: unexpected error: %v", tt.testDesc, err)
			}
		})
	}
}

// Этот тест теперь включен в TestNewPool_ErrorCases

// Этот тест можно запускать только при наличии реальной PostgreSQL БД
// Для интеграционных тестов можно использовать testcontainers или docker-compose
func TestNewPool_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// TODO: Реализовать с использованием testcontainers для полной изоляции
	t.Skip("integration test requires real PostgreSQL database")

	// Пример для реального тестирования:
	// dsn := "postgres://test:test@localhost:5432/test?sslmode=disable"
	// ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	// defer cancel()
	//
	// pool, err := NewPool(ctx, dsn)
	// if err != nil {
	//     t.Fatalf("failed to create pool: %v", err)
	// }
	// defer pool.Close()
	//
	// // Проверяем, что можем выполнить простой запрос
	// var result int
	// err = pool.QueryRow(ctx, "SELECT 1").Scan(&result)
	// if err != nil {
	//     t.Fatalf("failed to execute test query: %v", err)
	// }
	// if result != 1 {
	//     t.Errorf("expected 1, got %d", result)
	// }
}
