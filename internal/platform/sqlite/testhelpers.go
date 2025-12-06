package sqlite

import (
	"context"
	"database/sql"
	"testing"
)

// TestDB представляет тестовую SQLite базу данных с удобными хелперами.
type TestDB struct {
	DB       *sql.DB
	Path     string // Путь к файлу БД (пустой для in-memory)
	TxRunner *TxRunner
}

// NewTestDBInMemory создает in-memory SQLite БД для тестов.
// БД автоматически очищается после завершения теста.
func NewTestDBInMemory(t *testing.T) *TestDB {
	t.Helper()

	ctx := context.Background()
	db, err := NewInMemoryDB(ctx)
	if err != nil {
		t.Fatalf("Failed to create in-memory test DB: %v", err)
	}

	testDB := &TestDB{
		DB:       db,
		Path:     ":memory:",
		TxRunner: NewTxRunner(db),
	}

	// Автоматически закрываем БД после теста
	t.Cleanup(func() {
		_ = db.Close()
	})

	return testDB
}

// NewTestDBFile создает файловую SQLite БД для тестов.
// БД автоматически удаляется после завершения теста.
func NewTestDBFile(t *testing.T) *TestDB {
	t.Helper()

	ctx := context.Background()
	db, path, err := NewTestDB(ctx)
	if err != nil {
		t.Fatalf("Failed to create file test DB: %v", err)
	}

	testDB := &TestDB{
		DB:       db,
		Path:     path,
		TxRunner: NewTxRunner(db),
	}

	// Автоматически очищаем БД после теста
	t.Cleanup(func() {
		_ = CleanupTestDB(db, path)
	})

	return testDB
}

// ApplyTestMigrations применяет миграции к тестовой БД.
// Удобно для интеграционных тестов репозиториев.
func (tdb *TestDB) ApplyTestMigrations(t *testing.T, migrationsPath string) {
	t.Helper()

	if err := ApplyMigrations(tdb.Path, migrationsPath); err != nil {
		t.Fatalf("Failed to apply test migrations: %v", err)
	}
}

// Exec выполняет SQL команду и проверяет отсутствие ошибок.
func (tdb *TestDB) Exec(t *testing.T, query string, args ...any) sql.Result {
	t.Helper()

	result, err := tdb.DB.ExecContext(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("Failed to execute query: %v", err)
	}
	return result
}

// Query выполняет SQL запрос и возвращает результат.
func (tdb *TestDB) Query(t *testing.T, query string, args ...any) *sql.Rows {
	t.Helper()

	rows, err := tdb.DB.QueryContext(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("Failed to execute query: %v", err)
	}
	return rows
}

// QueryRow выполняет SQL запрос и возвращает одну строку.
func (tdb *TestDB) QueryRow(t *testing.T, query string, args ...any) *sql.Row {
	t.Helper()
	return tdb.DB.QueryRowContext(context.Background(), query, args...)
}

// TruncateTable очищает указанную таблицу.
// Полезно для очистки данных между тестами.
func (tdb *TestDB) TruncateTable(t *testing.T, tableName string) {
	t.Helper()
	tdb.Exec(t, "DELETE FROM "+tableName)
}

// TruncateAllTables очищает все таблицы в БД (кроме системных).
// Внимание: будет получен список всех таблиц и все будут очищены!
func (tdb *TestDB) TruncateAllTables(t *testing.T) {
	t.Helper()

	// Получаем список всех пользовательских таблиц
	rows := tdb.Query(t, "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name != 'schema_migrations'")
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			t.Fatalf("Failed to scan table name: %v", err)
		}
		tables = append(tables, tableName)
	}

	// Очищаем все таблицы
	for _, table := range tables {
		tdb.TruncateTable(t, table)
	}
}

// WithTx выполняет функцию в транзакции для тестов.
// Если тест падает, транзакция автоматически откатывается.
func (tdb *TestDB) WithTx(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()

	ctx := context.Background()
	err := tdb.TxRunner.WithinTx(ctx, fn)
	if err != nil {
		t.Fatalf("Transaction failed: %v", err)
	}
}

// MustSeedData вставляет тестовые данные и падает при ошибке.
// Удобно для подготовки данных в тестах.
func (tdb *TestDB) MustSeedData(t *testing.T, queries ...string) {
	t.Helper()

	for _, query := range queries {
		tdb.Exec(t, query)
	}
}

// CountRows возвращает количество строк в таблице.
func (tdb *TestDB) CountRows(t *testing.T, tableName string) int {
	t.Helper()

	var count int
	row := tdb.QueryRow(t, "SELECT COUNT(*) FROM "+tableName)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("Failed to count rows in table %s: %v", tableName, err)
	}
	return count
}

// TableExists проверяет существование таблицы.
func (tdb *TestDB) TableExists(t *testing.T, tableName string) bool {
	t.Helper()

	var count int
	row := tdb.QueryRow(t, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tableName)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("Failed to check table existence: %v", err)
	}
	return count > 0
}
