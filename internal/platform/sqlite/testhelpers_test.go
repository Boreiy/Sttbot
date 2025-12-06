package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTestDBInMemory(t *testing.T) {
	testDB := NewTestDBInMemory(t)

	assert.NotNil(t, testDB.DB)
	assert.Equal(t, ":memory:", testDB.Path)
	assert.NotNil(t, testDB.TxRunner)

	// Проверяем что БД работает
	err := testDB.DB.PingContext(context.Background())
	assert.NoError(t, err)
}

func TestNewTestDBFile(t *testing.T) {
	testDB := NewTestDBFile(t)

	assert.NotNil(t, testDB.DB)
	assert.NotEmpty(t, testDB.Path)
	assert.NotEqual(t, ":memory:", testDB.Path)
	assert.NotNil(t, testDB.TxRunner)

	// Проверяем что файл БД создан
	_, err := os.Stat(testDB.Path)
	assert.NoError(t, err)

	// Проверяем что БД работает
	err = testDB.DB.PingContext(context.Background())
	assert.NoError(t, err)
}

func TestTestDB_ApplyTestMigrations(t *testing.T) {
	testDB := NewTestDBFile(t) // Используем файловую БД для миграций

	// Создаем временную директорию для миграций
	tmpDir, err := os.MkdirTemp("", "sqlite_test_migrations")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Создаем тестовую миграцию
	migration := `CREATE TABLE test_users (id INTEGER PRIMARY KEY, name TEXT);`
	err = os.WriteFile(filepath.Join(tmpDir, "001_create_test_users.up.sql"), []byte(migration), 0644)
	require.NoError(t, err)

	migrationsPath := "file://" + filepath.ToSlash(tmpDir)

	// Применяем миграции
	testDB.ApplyTestMigrations(t, migrationsPath)

	// Проверяем что таблица создана
	assert.True(t, testDB.TableExists(t, "test_users"))
}

func TestTestDB_Exec(t *testing.T) {
	testDB := NewTestDBInMemory(t)

	// Создаем таблицу и вставляем данные
	testDB.Exec(t, "CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	result := testDB.Exec(t, "INSERT INTO test (value) VALUES (?)", "test_value")

	// Проверяем результат
	rowsAffected, err := result.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)
}

func TestTestDB_Query(t *testing.T) {
	testDB := NewTestDBInMemory(t)

	// Создаем таблицу и вставляем данные
	testDB.Exec(t, "CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	testDB.Exec(t, "INSERT INTO test (value) VALUES (?)", "test_value1")
	testDB.Exec(t, "INSERT INTO test (value) VALUES (?)", "test_value2")

	// Выполняем запрос
	rows := testDB.Query(t, "SELECT id, value FROM test ORDER BY id")
	defer rows.Close()

	var results []struct {
		ID    int
		Value string
	}

	for rows.Next() {
		var r struct {
			ID    int
			Value string
		}
		require.NoError(t, rows.Scan(&r.ID, &r.Value))
		results = append(results, r)
	}

	assert.Len(t, results, 2)
	assert.Equal(t, "test_value1", results[0].Value)
	assert.Equal(t, "test_value2", results[1].Value)
}

func TestTestDB_QueryRow(t *testing.T) {
	testDB := NewTestDBInMemory(t)

	// Создаем таблицу и вставляем данные
	testDB.Exec(t, "CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	testDB.Exec(t, "INSERT INTO test (value) VALUES (?)", "test_value")

	// Выполняем запрос одной строки
	row := testDB.QueryRow(t, "SELECT value FROM test WHERE id = ?", 1)

	var value string
	err := row.Scan(&value)
	require.NoError(t, err)
	assert.Equal(t, "test_value", value)
}

func TestTestDB_TruncateTable(t *testing.T) {
	testDB := NewTestDBInMemory(t)

	// Создаем таблицу и вставляем данные
	testDB.Exec(t, "CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	testDB.Exec(t, "INSERT INTO test (value) VALUES (?)", "test_value1")
	testDB.Exec(t, "INSERT INTO test (value) VALUES (?)", "test_value2")

	// Проверяем что данные есть
	assert.Equal(t, 2, testDB.CountRows(t, "test"))

	// Очищаем таблицу
	testDB.TruncateTable(t, "test")

	// Проверяем что данные удалены
	assert.Equal(t, 0, testDB.CountRows(t, "test"))
}

func TestTestDB_TruncateAllTables(t *testing.T) {
	testDB := NewTestDBInMemory(t)

	// Создаем несколько таблиц с данными
	testDB.Exec(t, "CREATE TABLE test1 (id INTEGER PRIMARY KEY, value TEXT)")
	testDB.Exec(t, "CREATE TABLE test2 (id INTEGER PRIMARY KEY, value TEXT)")
	testDB.Exec(t, "INSERT INTO test1 (value) VALUES (?)", "value1")
	testDB.Exec(t, "INSERT INTO test2 (value) VALUES (?)", "value2")

	// Проверяем что данные есть
	assert.Equal(t, 1, testDB.CountRows(t, "test1"))
	assert.Equal(t, 1, testDB.CountRows(t, "test2"))

	// Очищаем все таблицы
	testDB.TruncateAllTables(t)

	// Проверяем что все данные удалены
	assert.Equal(t, 0, testDB.CountRows(t, "test1"))
	assert.Equal(t, 0, testDB.CountRows(t, "test2"))
}

func TestTestDB_WithTx(t *testing.T) {
	testDB := NewTestDBInMemory(t)

	// Создаем таблицу
	testDB.Exec(t, "CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")

	// Выполняем операцию в транзакции
	testDB.WithTx(t, func(ctx context.Context) error {
		querier := testDB.TxRunner.GetQuerier(ctx)
		_, err := querier.ExecContext(ctx, "INSERT INTO test (value) VALUES (?)", "tx_value")
		return err
	})

	// Проверяем что данные сохранились
	assert.Equal(t, 1, testDB.CountRows(t, "test"))
}

func TestTestDB_MustSeedData(t *testing.T) {
	testDB := NewTestDBInMemory(t)

	// Создаем таблицу и заполняем данными
	queries := []string{
		"CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)",
		"INSERT INTO test (value) VALUES ('seed1')",
		"INSERT INTO test (value) VALUES ('seed2')",
	}

	testDB.MustSeedData(t, queries...)

	// Проверяем что данные добавлены
	assert.Equal(t, 2, testDB.CountRows(t, "test"))
}

func TestTestDB_CountRows(t *testing.T) {
	testDB := NewTestDBInMemory(t)

	// Создаем таблицу
	testDB.Exec(t, "CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")

	// Пустая таблица
	assert.Equal(t, 0, testDB.CountRows(t, "test"))

	// Добавляем данные
	testDB.Exec(t, "INSERT INTO test (value) VALUES (?)", "test1")
	testDB.Exec(t, "INSERT INTO test (value) VALUES (?)", "test2")

	assert.Equal(t, 2, testDB.CountRows(t, "test"))
}

func TestTestDB_TableExists(t *testing.T) {
	testDB := NewTestDBInMemory(t)

	// Таблица не существует
	assert.False(t, testDB.TableExists(t, "nonexistent"))

	// Создаем таблицу
	testDB.Exec(t, "CREATE TABLE test_table (id INTEGER PRIMARY KEY)")

	// Таблица существует
	assert.True(t, testDB.TableExists(t, "test_table"))
}

func TestTestDB_Cleanup(t *testing.T) {
	// Тест проверяет что cleanup функции вызываются автоматически
	// Этот тест больше демонстрирует использование, чем проверяет функциональность

	t.Run("in_memory_cleanup", func(t *testing.T) {
		testDB := NewTestDBInMemory(t)

		// Создаем таблицу
		testDB.Exec(t, "CREATE TABLE test (id INTEGER PRIMARY KEY)")

		// Cleanup будет вызван автоматически в конце теста
		// Проверяем что БД работает до cleanup
		assert.True(t, testDB.TableExists(t, "test"))
	})

	t.Run("file_cleanup", func(t *testing.T) {
		testDB := NewTestDBFile(t)

		// Проверяем что файл существует
		_, err := os.Stat(testDB.Path)
		assert.NoError(t, err)

		// Cleanup будет вызван автоматически в конце теста
	})
}
