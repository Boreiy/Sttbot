package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTxRunner(t *testing.T) {
	ctx := context.Background()
	db, err := NewInMemoryDB(ctx)
	require.NoError(t, err)
	defer db.Close()

	runner := NewTxRunner(db)
	assert.NotNil(t, runner)
	assert.Equal(t, db, runner.DB)
}

func TestTxRunner_WithinTx_Success(t *testing.T) {
	ctx := context.Background()
	db, err := NewInMemoryDB(ctx)
	require.NoError(t, err)
	defer db.Close()

	// Создаем таблицу для теста
	_, err = db.ExecContext(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	require.NoError(t, err)

	runner := NewTxRunner(db)

	// Выполняем операцию в транзакции
	err = runner.WithinTx(ctx, func(ctx context.Context) error {
		// Проверяем что транзакция доступна в контексте
		tx, ok := SqlTx(ctx)
		assert.True(t, ok)
		assert.NotNil(t, tx)

		// Вставляем данные через транзакцию
		_, err := tx.ExecContext(ctx, "INSERT INTO test (value) VALUES (?)", "test_value")
		return err
	})

	require.NoError(t, err)

	// Проверяем что данные действительно сохранились (транзакция закоммичена)
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestTxRunner_WithinTx_Rollback(t *testing.T) {
	ctx := context.Background()
	db, err := NewInMemoryDB(ctx)
	require.NoError(t, err)
	defer db.Close()

	// Создаем таблицу для теста
	_, err = db.ExecContext(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	require.NoError(t, err)

	runner := NewTxRunner(db)
	testError := errors.New("test error")

	// Выполняем операцию в транзакции, которая должна провалиться
	err = runner.WithinTx(ctx, func(ctx context.Context) error {
		tx, ok := SqlTx(ctx)
		assert.True(t, ok)
		assert.NotNil(t, tx)

		// Вставляем данные
		_, err := tx.ExecContext(ctx, "INSERT INTO test (value) VALUES (?)", "test_value")
		if err != nil {
			return err
		}

		// Возвращаем ошибку для отката
		return testError
	})

	// Проверяем что получили ожидаемую ошибку
	assert.ErrorIs(t, err, testError)

	// Проверяем что данные НЕ сохранились (транзакция откачена)
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestTxRunner_GetQuerier_WithTx(t *testing.T) {
	ctx := context.Background()
	db, err := NewInMemoryDB(ctx)
	require.NoError(t, err)
	defer db.Close()

	runner := NewTxRunner(db)

	err = runner.WithinTx(ctx, func(ctx context.Context) error {
		// Внутри транзакции GetQuerier должен вернуть транзакцию
		querier := runner.GetQuerier(ctx)
		assert.NotNil(t, querier)

		// Проверяем что это именно транзакция
		tx, ok := SqlTx(ctx)
		assert.True(t, ok)
		assert.Equal(t, tx, querier)

		return nil
	})

	require.NoError(t, err)
}

func TestTxRunner_GetQuerier_WithoutTx(t *testing.T) {
	ctx := context.Background()
	db, err := NewInMemoryDB(ctx)
	require.NoError(t, err)
	defer db.Close()

	runner := NewTxRunner(db)

	// Вне транзакции GetQuerier должен вернуть саму БД
	querier := runner.GetQuerier(ctx)
	assert.NotNil(t, querier)
	assert.Equal(t, db, querier)

	// Проверяем что транзакции нет в контексте
	tx, ok := SqlTx(ctx)
	assert.False(t, ok)
	assert.Nil(t, tx)
}

func TestTxRunner_BeginTx(t *testing.T) {
	ctx := context.Background()
	db, err := NewInMemoryDB(ctx)
	require.NoError(t, err)
	defer db.Close()

	runner := NewTxRunner(db)

	// Начинаем транзакцию вручную
	newCtx, tx, err := runner.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, tx)

	defer func() {
		if err := tx.Rollback(); err != nil {
			t.Logf("Failed to rollback transaction: %v", err)
		}
	}() // На случай если тест упадет

	// Проверяем что транзакция доступна в новом контексте
	txFromCtx, ok := SqlTx(newCtx)
	assert.True(t, ok)
	assert.Equal(t, tx, txFromCtx)

	// Проверяем что в старом контексте транзакции нет
	_, ok = SqlTx(ctx)
	assert.False(t, ok)

	// Коммитим транзакцию
	err = tx.Commit()
	assert.NoError(t, err)
}

func TestSqlTx_NoTxInContext(t *testing.T) {
	ctx := context.Background()

	tx, ok := SqlTx(ctx)
	assert.False(t, ok)
	assert.Nil(t, tx)
}

func TestQuerier_Interface(t *testing.T) {
	ctx := context.Background()
	db, err := NewInMemoryDB(ctx)
	require.NoError(t, err)
	defer db.Close()

	// Проверяем что sql.DB реализует Querier
	var _ Querier = db

	// Проверяем что sql.Tx реализует Querier
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() {
		if err := tx.Rollback(); err != nil {
			t.Logf("Failed to rollback transaction: %v", err)
		}
	}()

	var _ Querier = tx
}

func TestTxRunner_ImmediateLockMode(t *testing.T) {
	ctx := context.Background()
	db, err := NewInMemoryDB(ctx)
	require.NoError(t, err)
	defer db.Close()

	// Создаем таблицу для теста
	_, err = db.ExecContext(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	require.NoError(t, err)

	// Создаем TxRunner с IMMEDIATE режимом блокировки
	opts := DefaultDBOptions()
	opts.TxLockMode = TxLockImmediate
	runner := NewTxRunnerWithOptions(db, opts)

	// Выполняем операцию записи
	err = runner.WithinTx(ctx, func(ctx context.Context) error {
		querier := runner.GetQuerier(ctx)
		_, err := querier.ExecContext(ctx, "INSERT INTO test (value) VALUES (?)", "immediate_test")
		return err
	})

	require.NoError(t, err)

	// Проверяем что данные сохранились
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestTxRunner_WithWriteQueue(t *testing.T) {
	ctx := context.Background()
	db, err := NewInMemoryDB(ctx)
	require.NoError(t, err)
	defer db.Close()

	// Создаем таблицу для теста
	_, err = db.ExecContext(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	require.NoError(t, err)

	// Создаем TxRunner с включенной очередью записи
	opts := DefaultDBOptions()
	opts.EnableWriteQueue = true
	opts.WriteQueueSize = 10
	runner := NewTxRunnerWithOptions(db, opts)
	defer runner.Close()

	// Выполняем несколько операций записи параллельно
	const numOps = 5
	errCh := make(chan error, numOps)

	for i := 0; i < numOps; i++ {
		value := fmt.Sprintf("test_value_%d", i)
		go func(val string) {
			err := runner.WithinTxWrite(ctx, func(ctx context.Context) error {
				querier := runner.GetQuerier(ctx)
				_, err := querier.ExecContext(ctx, "INSERT INTO test (value) VALUES (?)", val)
				return err
			})
			errCh <- err
		}(value)
	}

	// Ждем завершения всех операций
	for i := 0; i < numOps; i++ {
		err := <-errCh
		require.NoError(t, err)
	}

	// Проверяем что все данные сохранились
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, numOps, count)
}

func TestTxRunner_ReadWriteSeparation(t *testing.T) {
	ctx := context.Background()
	db, err := NewInMemoryDB(ctx)
	require.NoError(t, err)
	defer db.Close()

	// Создаем таблицу и данные для теста
	_, err = db.ExecContext(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO test (value) VALUES ('existing')")
	require.NoError(t, err)

	// Создаем TxRunner с включенной очередью записи
	opts := DefaultDBOptions()
	opts.EnableWriteQueue = true
	runner := NewTxRunnerWithOptions(db, opts)
	defer runner.Close()

	// Операция чтения не должна идти через очередь
	var value string
	err = runner.WithinTxRead(ctx, func(ctx context.Context) error {
		querier := runner.GetQuerier(ctx)
		row := querier.QueryRowContext(ctx, "SELECT value FROM test WHERE id = ?", 1)
		return row.Scan(&value)
	})

	require.NoError(t, err)
	assert.Equal(t, "existing", value)

	// Операция записи идет через очередь
	err = runner.WithinTxWrite(ctx, func(ctx context.Context) error {
		querier := runner.GetQuerier(ctx)
		_, err := querier.ExecContext(ctx, "UPDATE test SET value = ? WHERE id = ?", "updated", 1)
		return err
	})

	require.NoError(t, err)

	// Проверяем что данные обновились
	err = db.QueryRowContext(ctx, "SELECT value FROM test WHERE id = ?", 1).Scan(&value)
	require.NoError(t, err)
	assert.Equal(t, "updated", value)
}

func TestTxRunner_SavepointSuccess(t *testing.T) {
	ctx := context.Background()
	db, err := NewInMemoryDB(ctx)
	require.NoError(t, err)
	defer db.Close()

	// Создаем таблицу для теста
	_, err = db.ExecContext(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	require.NoError(t, err)

	runner := NewTxRunner(db)

	// Выполняем операцию в savepoint
	err = runner.WithinSavepoint(ctx, func(ctx context.Context) error {
		querier := runner.GetQuerier(ctx)
		_, err := querier.ExecContext(ctx, "INSERT INTO test (value) VALUES (?)", "savepoint_test")
		return err
	})

	require.NoError(t, err)

	// Проверяем что данные сохранились
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestTxRunner_SavepointRollback(t *testing.T) {
	ctx := context.Background()
	db, err := NewInMemoryDB(ctx)
	require.NoError(t, err)
	defer db.Close()

	// Создаем таблицу для теста
	_, err = db.ExecContext(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	require.NoError(t, err)

	runner := NewTxRunner(db)
	testError := errors.New("test rollback error")

	// Выполняем операцию в savepoint с ошибкой
	err = runner.WithinSavepoint(ctx, func(ctx context.Context) error {
		querier := runner.GetQuerier(ctx)
		_, err := querier.ExecContext(ctx, "INSERT INTO test (value) VALUES (?)", "should_be_rolled_back")
		if err != nil {
			return err
		}
		// Возвращаем ошибку для отката savepoint
		return testError
	})

	// Проверяем что получили ожидаемую ошибку
	assert.ErrorIs(t, err, testError)

	// Проверяем что данные НЕ сохранились (savepoint откачен)
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestTxRunner_NestedSavepoints(t *testing.T) {
	ctx := context.Background()
	db, err := NewInMemoryDB(ctx)
	require.NoError(t, err)
	defer db.Close()

	// Создаем таблицу для теста
	_, err = db.ExecContext(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	require.NoError(t, err)

	runner := NewTxRunner(db)

	// Выполняем вложенные savepoints
	err = runner.WithinTx(ctx, func(outerCtx context.Context) error {
		querier := runner.GetQuerier(outerCtx)

		// Вставляем данные во внешней транзакции
		_, err := querier.ExecContext(outerCtx, "INSERT INTO test (value) VALUES (?)", "outer_data")
		if err != nil {
			return err
		}

		// Создаём успешный savepoint
		err = runner.WithinSavepoint(outerCtx, func(sp1Ctx context.Context) error {
			querier := runner.GetQuerier(sp1Ctx)
			_, err := querier.ExecContext(sp1Ctx, "INSERT INTO test (value) VALUES (?)", "savepoint1_data")
			return err
		})
		if err != nil {
			return err
		}

		// Создаём неуспешный savepoint
		err = runner.WithinSavepoint(outerCtx, func(sp2Ctx context.Context) error {
			querier := runner.GetQuerier(sp2Ctx)
			_, err := querier.ExecContext(sp2Ctx, "INSERT INTO test (value) VALUES (?)", "savepoint2_data_should_rollback")
			if err != nil {
				return err
			}
			return errors.New("savepoint2 error")
		})

		// Ошибка savepoint не должна прерывать внешнюю транзакцию
		assert.Error(t, err)

		return nil
	})

	require.NoError(t, err)

	// Проверяем что сохранились только данные из внешней транзакции и первого savepoint
	var values []string
	rows, err := db.QueryContext(ctx, "SELECT value FROM test ORDER BY id")
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var value string
		require.NoError(t, rows.Scan(&value))
		values = append(values, value)
	}

	assert.Equal(t, []string{"outer_data", "savepoint1_data"}, values)
}

func TestTxRunner_NestedTransactions(t *testing.T) {
	ctx := context.Background()
	db, err := NewInMemoryDB(ctx)
	require.NoError(t, err)
	defer db.Close()

	// Создаем таблицу для теста
	_, err = db.ExecContext(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	require.NoError(t, err)

	runner := NewTxRunner(db)

	// SQLite не поддерживает истинные вложенные транзакции
	// Попытка создать вложенную транзакцию должна вернуть ошибку
	err = runner.WithinTx(ctx, func(outerCtx context.Context) error {
		outerTx, ok := SqlTx(outerCtx)
		assert.True(t, ok)
		assert.NotNil(t, outerTx)

		// Вставляем данные через внешнюю транзакцию
		_, err := outerTx.ExecContext(outerCtx, "INSERT INTO test (value) VALUES (?)", "outer_test")
		if err != nil {
			return err
		}

		// Попытка запуска вложенной транзакции должна привести к ошибке
		// поскольку SQLite не поддерживает вложенные транзакции
		innerErr := runner.WithinTx(outerCtx, func(innerCtx context.Context) error {
			innerTx, ok := SqlTx(innerCtx)
			if !ok {
				return errors.New("no transaction in context")
			}
			_, err := innerTx.ExecContext(innerCtx, "INSERT INTO test (value) VALUES (?)", "inner_test")
			return err
		})

		// Для SQLite вложенные транзакции не поддерживаются, поэтому ожидаем ошибку
		assert.Error(t, innerErr)
		assert.True(t, strings.Contains(innerErr.Error(), "nested transactions are not supported"))

		return nil
	})

	require.NoError(t, err)

	// Проверяем что данные из внешней транзакции сохранились
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}
