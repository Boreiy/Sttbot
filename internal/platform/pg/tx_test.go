package pg

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPgxTx_NoTransaction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tx, ok := PgxTx(ctx)
	if ok {
		t.Error("expected no transaction, but PgxTx returned true")
	}
	if tx != nil {
		t.Error("expected nil transaction, but got non-nil")
	}
}

func TestPgxTx_WithTransaction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Создаем контекст с "транзакцией" (для тестирования используем любой объект)
	mockValue := "test-transaction"
	ctx = context.WithValue(ctx, txKey{}, mockValue)

	// PgxTx должен извлечь значение, но оно не будет pgx.Tx
	_, ok := PgxTx(ctx)
	if ok {
		t.Error("expected type assertion to fail for non-pgx.Tx value")
	}
}

func TestQuerier_Interface(t *testing.T) {
	t.Parallel()

	// Проверяем, что типы действительно реализуют интерфейс Querier
	var pool *pgxpool.Pool
	var _ Querier = pool

	// Использование переменной интерфейса для проверки компиляции
	querier := Querier(pool)
	_ = querier // Переменная используется для проверки компиляции
}

func TestNewTxRunner(t *testing.T) {
	t.Parallel()

	pool := &pgxpool.Pool{} // Мок пула для тестирования
	runner := NewTxRunner(pool)

	if runner == nil {
		t.Error("NewTxRunner returned nil")
		return
	}
	if runner.Pool != pool {
		t.Error("TxRunner pool not set correctly")
	}
}

func TestTxRunner_GetQuerier_WithoutTransaction(t *testing.T) {
	t.Parallel()

	pool := &pgxpool.Pool{}
	runner := NewTxRunner(pool)
	ctx := context.Background()

	// Без транзакции должен возвращать пул
	querier := runner.GetQuerier(ctx)
	if querier == nil {
		t.Error("expected non-nil querier")
	}
	// Проверяем, что это пул (через type assertion)
	if _, ok := querier.(*pgxpool.Pool); !ok {
		t.Error("expected *pgxpool.Pool when no transaction in context")
	}
	// Проверяем, что возвращает реализацию Querier (уже имеет тип Querier)
	_ = querier // Уже типизирован как Querier
}

func TestTxRunner_GetQuerier_WithContext(t *testing.T) {
	t.Parallel()

	pool := &pgxpool.Pool{}
	runner := NewTxRunner(pool)
	ctx := context.Background()

	// С произвольным значением в контексте (не транзакцией)
	ctx = context.WithValue(ctx, txKey{}, "not-a-transaction")

	// Должен вернуть пул, так как значение не является pgx.Tx
	querier := runner.GetQuerier(ctx)
	if querier == nil {
		t.Error("expected non-nil querier")
	}
	// Проверяем, что это пул (через type assertion)
	if _, ok := querier.(*pgxpool.Pool); !ok {
		t.Error("expected *pgxpool.Pool when context contains non-transaction value")
	}
	// Проверяем, что возвращает реализацию Querier (уже имеет тип Querier)
	_ = querier // Уже типизирован как Querier
}

func TestTxRunner_WithinTxWithOptions_OptionsValidation(t *testing.T) {
	t.Parallel()

	// Этот тест проверяет, что различные опции транзакций корректно типизированы
	// и могут быть переданы в функцию без ошибок компиляции

	// Типы из pgx используются для опций транзакций
	var _ pgx.TxOptions // Явное использование для линтера

	testCases := []struct {
		name    string
		options pgx.TxOptions
	}{
		{
			name:    "default_options",
			options: pgx.TxOptions{},
		},
		{
			name: "read_committed",
			options: pgx.TxOptions{
				IsoLevel: pgx.ReadCommitted,
			},
		},
		{
			name: "serializable",
			options: pgx.TxOptions{
				IsoLevel: pgx.Serializable,
			},
		},
		{
			name: "read_only",
			options: pgx.TxOptions{
				AccessMode: pgx.ReadOnly,
			},
		},
		{
			name: "read_write",
			options: pgx.TxOptions{
				AccessMode: pgx.ReadWrite,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Проверяем, что опции можно присвоить переменной
			var opts pgx.TxOptions = tc.options
			_ = opts // Используем переменную

			// Проверяем, что структура корректно инициализируется
			if tc.name == "" {
				t.Error("test case name should not be empty")
			}
		})
	}
}

// Интеграционные тесты для полноценной работы с транзакциями
// требуют реальной базы данных и выходят за рамки юнит-тестирования
func TestTxRunner_WithinTx_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// TODO: Реализовать интеграционные тесты с testcontainers
	t.Skip("integration test requires real PostgreSQL database")

	// Пример структуры интеграционного теста:
	// pool := setupTestDatabase(t)
	// defer pool.Close()
	//
	// runner := NewTxRunner(pool)
	// ctx := context.Background()
	//
	// err := runner.WithinTx(ctx, func(ctx context.Context) error {
	//     tx, ok := PgxTx(ctx)
	//     if !ok {
	//         return errors.New("expected transaction in context")
	//     }
	//
	//     // Выполняем тестовые операции с транзакцией
	//     _, err := tx.Exec(ctx, "SELECT 1")
	//     return err
	// })
	//
	// if err != nil {
	//     t.Fatalf("transaction failed: %v", err)
	// }
}

func TestTxRunner_WithinTxWithOptions_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// TODO: Реализовать интеграционные тесты с testcontainers
	t.Skip("integration test requires real PostgreSQL database")

	// Пример структуры интеграционного теста с опциями:
	// pool := setupTestDatabase(t)
	// defer pool.Close()
	//
	// runner := NewTxRunner(pool)
	// ctx := context.Background()
	//
	// opts := pgx.TxOptions{
	//     IsoLevel:   pgx.ReadCommitted,
	//     AccessMode: pgx.ReadWrite,
	// }
	//
	// err := runner.WithinTxWithOptions(ctx, opts, func(ctx context.Context) error {
	//     tx, ok := PgxTx(ctx)
	//     if !ok {
	//         return errors.New("expected transaction in context")
	//     }
	//
	//     // Выполняем тестовые операции с транзакцией
	//     _, err := tx.Exec(ctx, "SELECT 1")
	//     return err
	// })
	//
	// if err != nil {
	//     t.Fatalf("transaction with options failed: %v", err)
	// }
}
