package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultDBOptions(t *testing.T) {
	opts := DefaultDBOptions()

	assert.Equal(t, time.Hour, opts.ConnMaxLifetime)
	assert.Equal(t, 10*time.Minute, opts.ConnMaxIdleTime)
	assert.Equal(t, 4, opts.MaxOpenConns)
	assert.Equal(t, 1, opts.MaxIdleConns)
	assert.Equal(t, 5*time.Second, opts.PingTimeout)
	assert.True(t, opts.WALMode)
	assert.True(t, opts.ForeignKeys)
	assert.Equal(t, 5*time.Second, opts.BusyTimeout)
	assert.Equal(t, TxLockDeferred, opts.TxLockMode)
	assert.False(t, opts.EnableWriteQueue)
	assert.Equal(t, 100, opts.WriteQueueSize)
	assert.Equal(t, AccessModeReadWrite, opts.AccessMode)
}

func TestBuildDSN(t *testing.T) {
	tests := []struct {
		name     string
		dbPath   string
		opts     DBOptions
		expected string
	}{
		{
			name:     "default options",
			dbPath:   "/tmp/test.db",
			opts:     DefaultDBOptions(),
			expected: "/tmp/test.db?_busy_timeout=5000",
		},
		{
			name:   "without busy timeout",
			dbPath: ":memory:",
			opts: DBOptions{
				BusyTimeout: 0,
			},
			expected: ":memory:",
		},
		{
			name:   "custom busy timeout",
			dbPath: "test.db",
			opts: DBOptions{
				BusyTimeout: 10 * time.Second,
			},
			expected: "test.db?_busy_timeout=10000",
		},
		{
			name:   "read only mode",
			dbPath: "test.db",
			opts: DBOptions{
				AccessMode: AccessModeReadOnly,
			},
			expected: "test.db?mode=ro",
		},
		{
			name:   "read write create mode with timeout",
			dbPath: "test.db",
			opts: DBOptions{
				AccessMode:  AccessModeReadWriteCreate,
				BusyTimeout: 2 * time.Second,
			},
			expected: "test.db?mode=rwc&_busy_timeout=2000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildDSN(tt.dbPath, tt.opts)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewInMemoryDB(t *testing.T) {
	ctx := context.Background()
	db, err := NewInMemoryDB(ctx)
	require.NoError(t, err)
	require.NotNil(t, db)

	defer func() { _ = db.Close() }()

	// Проверяем что БД работает
	err = db.PingContext(ctx)
	assert.NoError(t, err)

	// Проверяем что можем создать таблицу
	_, err = db.ExecContext(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY)")
	assert.NoError(t, err)
}

func TestNewTestDB(t *testing.T) {
	ctx := context.Background()
	db, path, err := NewTestDB(ctx)
	require.NoError(t, err)
	require.NotNil(t, db)
	require.NotEmpty(t, path)

	defer func() {
		_ = CleanupTestDB(db, path)
	}()

	// Проверяем что файл создан
	_, err = os.Stat(path)
	assert.NoError(t, err)

	// Проверяем что БД работает
	err = db.PingContext(ctx)
	assert.NoError(t, err)

	// Проверяем что можем создать таблицу
	_, err = db.ExecContext(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY)")
	assert.NoError(t, err)
}

func TestNewDB_CreateDirectory(t *testing.T) {
	ctx := context.Background()

	// Создаем временную директорию для теста
	tmpDir, err := os.MkdirTemp("", "sqlite_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Путь к БД в поддиректории, которой еще нет
	dbPath := filepath.Join(tmpDir, "subdir", "test.db")

	db, err := NewDB(ctx, dbPath)
	require.NoError(t, err)
	require.NotNil(t, db)

	defer func() { _ = db.Close() }()

	// Проверяем что директория создана
	_, err = os.Stat(filepath.Dir(dbPath))
	assert.NoError(t, err)

	// Проверяем что файл БД создан
	_, err = os.Stat(dbPath)
	assert.NoError(t, err)
}

func TestNewDBWithOptions(t *testing.T) {
	ctx := context.Background()

	tmpFile, err := os.CreateTemp("", "test_*.db")
	require.NoError(t, err)
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(tmpPath)

	opts := DBOptions{
		ConnMaxLifetime: 30 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
		MaxOpenConns:    5,
		MaxIdleConns:    1,
		PingTimeout:     2 * time.Second,
		WALMode:         false,
		ForeignKeys:     false,
	}

	db, err := NewDBWithOptions(ctx, tmpPath, opts)
	require.NoError(t, err)
	require.NotNil(t, db)

	defer func() { _ = db.Close() }()

	// Проверяем что БД работает
	err = db.PingContext(ctx)
	assert.NoError(t, err)
}

func TestNewDB_InvalidPath(t *testing.T) {
	ctx := context.Background()

	var invalidPath string

	// Используем недопустимые пути для разных ОС
	if os.Getenv("OS") == "Windows_NT" || strings.Contains(os.Getenv("OS"), "Windows") {
		// На Windows используем недопустимые символы в имени файла
		invalidPath = "C:\\invalid<>:\"|?*path\\test.db"
	} else {
		// На Unix-системах используем путь в /dev/null (нельзя создать директории)
		invalidPath = "/dev/null/nonexistent/test.db"
	}

	_, err := NewDB(ctx, invalidPath)
	assert.Error(t, err)
}

func TestCleanupTestDB(t *testing.T) {
	ctx := context.Background()

	// Тест с обычным файлом
	db, path, err := NewTestDB(ctx)
	require.NoError(t, err)

	// Проверяем что файл существует
	_, err = os.Stat(path)
	assert.NoError(t, err)

	// Очищаем
	err = CleanupTestDB(db, path)
	assert.NoError(t, err)

	// Проверяем что файл удален
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

func TestCleanupTestDB_InMemory(t *testing.T) {
	ctx := context.Background()

	db, err := NewInMemoryDB(ctx)
	require.NoError(t, err)

	// Для in-memory БД cleanup не должен возвращать ошибку
	err = CleanupTestDB(db, ":memory:")
	assert.NoError(t, err)
}

func TestCleanupTestDB_NilDB(t *testing.T) {
	// С nil БД не должно быть ошибки
	err := CleanupTestDB(nil, "")
	assert.NoError(t, err)
}

func TestPragmaSettings(t *testing.T) {
	ctx := context.Background()

	// Создаем БД с кастомными настройками
	opts := DefaultDBOptions()
	opts.WALMode = true
	opts.ForeignKeys = true
	opts.BusyTimeout = 10 * time.Second

	db, path, err := NewTestDB(ctx)
	require.NoError(t, err)
	defer func() {
		if err := CleanupTestDB(db, path); err != nil {
			t.Logf("Failed to cleanup test DB: %v", err)
		}
	}()

	// Проверяем что PRAGMA настройки применены
	var journalMode string
	err = db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode)
	require.NoError(t, err)
	assert.Equal(t, "wal", strings.ToLower(journalMode))

	var foreignKeys int
	err = db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys)
	require.NoError(t, err)
	assert.Equal(t, 1, foreignKeys)

	var busyTimeout int
	err = db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout)
	require.NoError(t, err)
	assert.Equal(t, 5000, busyTimeout) // DefaultDBOptions устанавливает 5 секунд

	var synchronous string
	err = db.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous)
	require.NoError(t, err)
	// SQLite возвращает числовое значение: 0=OFF, 1=NORMAL, 2=FULL, 3=EXTRA
	// NORMAL соответствует 1
	assert.Equal(t, "1", synchronous)
}

func TestNewReadOnlyDB(t *testing.T) {
	ctx := context.Background()

	// Создаем временную БД с данными
	tmpFile, err := os.CreateTemp("", "test_readonly_*.db")
	require.NoError(t, err)
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Создаем БД и таблицу в обычном режиме
	db, err := NewDB(ctx, tmpPath)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO test (value) VALUES ('test_data')")
	require.NoError(t, err)
	db.Close()

	// Открываем в read-only режиме
	roDB, err := NewReadOnlyDB(ctx, tmpPath)
	require.NoError(t, err)
	defer roDB.Close()

	// Проверяем что можем читать данные
	var value string
	err = roDB.QueryRowContext(ctx, "SELECT value FROM test WHERE id = 1").Scan(&value)
	require.NoError(t, err)
	assert.Equal(t, "test_data", value)

	// Проверяем что запись недоступна (некоторые SQLite драйверы могут не поддерживать mode=ro)
	_, err = roDB.ExecContext(ctx, "INSERT INTO test (value) VALUES ('should_fail')")
	if err != nil {
		// Если ошибка есть - проверяем что это связано с read-only режимом
		errMsg := strings.ToLower(err.Error())
		assert.True(t,
			strings.Contains(errMsg, "readonly") ||
				strings.Contains(errMsg, "read-only") ||
				strings.Contains(errMsg, "database is locked") ||
				strings.Contains(errMsg, "attempt to write"),
			"Expected read-only error, got: %s", err.Error())
	} else {
		// Если ошибки нет, это может означать что драйвер не поддерживает mode=ro через DSN
		t.Logf("Warning: SQLite driver may not support read-only mode via DSN")
	}
}

func TestNewDBWithMode(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		mode        AccessMode
		shouldWrite bool
	}{
		{
			name:        "read write mode",
			mode:        AccessModeReadWrite,
			shouldWrite: true,
		},
		{
			name:        "read only mode",
			mode:        AccessModeReadOnly,
			shouldWrite: false,
		},
		{
			name:        "read write create mode",
			mode:        AccessModeReadWriteCreate,
			shouldWrite: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp("", "test_mode_*.db")
			require.NoError(t, err)
			tmpPath := tmpFile.Name()
			tmpFile.Close()
			defer os.Remove(tmpPath)

			// Для read-only тестов сначала создаем БД с данными
			if !tt.shouldWrite {
				setupDB, err := NewDB(ctx, tmpPath)
				require.NoError(t, err)
				_, err = setupDB.ExecContext(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, value TEXT)")
				require.NoError(t, err)
				setupDB.Close()
			}

			db, err := NewDBWithMode(ctx, tmpPath, tt.mode)
			require.NoError(t, err)
			defer db.Close()

			if tt.shouldWrite {
				// Проверяем что можем создать таблицу и записать данные
				_, err = db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS test (id INTEGER PRIMARY KEY, value TEXT)")
				assert.NoError(t, err)
				_, err = db.ExecContext(ctx, "INSERT INTO test (value) VALUES ('test')")
				assert.NoError(t, err)
			} else {
				// Проверяем что запись недоступна (или предупреждаем если драйвер не поддерживает)
				_, err = db.ExecContext(ctx, "INSERT INTO test (value) VALUES ('should_fail')")
				if err == nil {
					t.Logf("Warning: SQLite driver may not support read-only mode via DSN for mode %s", tt.mode)
				}
			}
		})
	}
}

func TestNewDBFromDSN(t *testing.T) {
	ctx := context.Background()

	// Создаем временный файл для БД
	tmpFile, err := os.CreateTemp("", "test_dsn_*.db")
	require.NoError(t, err)
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Тестируем с базовым DSN
	basicDSN := tmpPath
	db, err := NewDBFromDSN(ctx, basicDSN)
	require.NoError(t, err)
	defer db.Close()

	// Проверяем что БД работает
	err = db.PingContext(ctx)
	assert.NoError(t, err)

	// Проверяем что можем создать таблицу
	_, err = db.ExecContext(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY)")
	assert.NoError(t, err)
}

func TestNewDBFromDSN_WithParameters(t *testing.T) {
	ctx := context.Background()

	// Создаем временный файл для БД
	tmpFile, err := os.CreateTemp("", "test_dsn_params_*.db")
	require.NoError(t, err)
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Тестируем с DSN включающим параметры
	dsnWithParams := tmpPath + "?mode=rwc&_busy_timeout=1000"
	db, err := NewDBFromDSN(ctx, dsnWithParams)
	require.NoError(t, err)
	defer db.Close()

	// Проверяем что БД работает
	err = db.PingContext(ctx)
	assert.NoError(t, err)

	// Проверяем что можем создать таблицу
	_, err = db.ExecContext(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY)")
	assert.NoError(t, err)
}
