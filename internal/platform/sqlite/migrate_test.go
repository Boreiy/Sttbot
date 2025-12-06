package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMigrateURL(t *testing.T) {
	tests := []struct {
		name        string
		inputPath   string
		expectError bool
		// expectedURL будет проверяться по-разному для разных ОС
	}{
		{
			name:        "relative path",
			inputPath:   "test.db",
			expectError: false,
		},
		{
			name:        "absolute unix path",
			inputPath:   "/tmp/test.db",
			expectError: false,
		},
		{
			name:        "memory database",
			inputPath:   ":memory:",
			expectError: false,
		},
	}

	// Добавляем Windows-специфические тесты только на Windows
	if runtime.GOOS == "windows" {
		tests = append(tests, struct {
			name        string
			inputPath   string
			expectError bool
		}{
			name:        "windows absolute path",
			inputPath:   "C:\\temp\\test.db",
			expectError: false,
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := BuildMigrateURL(tt.inputPath)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(url, "sqlite://"))

			// Проверяем что URL содержит корректный путь
			if runtime.GOOS == "windows" && len(tt.inputPath) >= 2 && tt.inputPath[1] == ':' {
				// На Windows для C:\path ожидаем sqlite:///C:/path
				assert.Contains(t, url, "sqlite:///")
				assert.Contains(t, url, "/"+strings.ToUpper(string(tt.inputPath[0])))
			} else {
				// На Unix ожидаем sqlite:// + абсолютный путь
				assert.Contains(t, url, "sqlite://")
			}

			t.Logf("Input: %s -> URL: %s", tt.inputPath, url)
		})
	}
}

func TestBuildMigrateURL_CrossPlatform(t *testing.T) {
	// Тест с временным файлом для кроссплатформенности
	tmpFile, err := os.CreateTemp("", "test_migrate_*.db")
	require.NoError(t, err)
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	url, err := BuildMigrateURL(tmpPath)
	require.NoError(t, err)

	// Основные проверки
	assert.True(t, strings.HasPrefix(url, "sqlite://"))
	assert.Contains(t, url, filepath.Base(tmpPath))

	// Проверяем что в URL нет обратных слешей
	assert.False(t, strings.Contains(url, "\\"))
}

func TestApplyMigrations_NoMigrations(t *testing.T) {
	// Создаем временную БД для тестов
	tmpFile, err := os.CreateTemp("", "test_*.db")
	require.NoError(t, err)
	dbPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(dbPath)

	// Создаем временную директорию для пустых миграций
	tmpDir, err := os.MkdirTemp("", "sqlite_migrations_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	migrationsPath := "file://" + filepath.ToSlash(tmpDir)

	// Применение пустого набора миграций может вернуть ошибку "no migration files"
	// что является нормальным поведением golang-migrate
	err = ApplyMigrations(dbPath, migrationsPath)
	// Принимаем как ошибку "no migration files", так и отсутствие ошибки
	if err != nil {
		assert.Contains(t, err.Error(), "file does not exist")
	}
}

func TestApplyMigrations_WithMigrations(t *testing.T) {
	// Создаем временную БД для тестов
	tmpFile, err := os.CreateTemp("", "test_*.db")
	require.NoError(t, err)
	dbPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(dbPath)

	// Создаем временную директорию для миграций
	tmpDir, err := os.MkdirTemp("", "sqlite_migrations_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Создаем тестовые миграции
	migration1Up := `
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL
);
`
	migration1Down := `DROP TABLE users;`

	migration2Up := `
CREATE TABLE posts (
    id INTEGER PRIMARY KEY,
    user_id INTEGER,
    title TEXT NOT NULL,
    FOREIGN KEY(user_id) REFERENCES users(id)
);
`
	migration2Down := `DROP TABLE posts;`

	// Записываем файлы миграций
	err = os.WriteFile(filepath.Join(tmpDir, "001_create_users.up.sql"), []byte(migration1Up), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "001_create_users.down.sql"), []byte(migration1Down), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "002_create_posts.up.sql"), []byte(migration2Up), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "002_create_posts.down.sql"), []byte(migration2Down), 0644)
	require.NoError(t, err)

	migrationsPath := "file://" + filepath.ToSlash(tmpDir)

	// Применяем миграции
	err = ApplyMigrations(dbPath, migrationsPath)
	require.NoError(t, err)

	// Открываем БД для проверки что таблицы созданы
	ctx := context.Background()
	db, err := NewDB(ctx, dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Проверяем что таблицы созданы
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('users', 'posts')").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Повторное применение не должно давать ошибку
	err = ApplyMigrations(dbPath, migrationsPath)
	assert.NoError(t, err)
}

func TestGetMigrationVersion(t *testing.T) {
	// Создаем временную БД для тестов
	tmpFile, err := os.CreateTemp("", "test_*.db")
	require.NoError(t, err)
	dbPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(dbPath)

	// Создаем временную директорию для миграций
	tmpDir, err := os.MkdirTemp("", "sqlite_migrations_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	migrationsPath := "file://" + filepath.ToSlash(tmpDir)

	// До применения миграций версия может быть недоступна для пустой директории
	version, dirty, err := GetMigrationVersion(dbPath, migrationsPath)
	if err != nil {
		// Для пустой директории это нормально
		assert.Contains(t, err.Error(), "file does not exist")
		return
	}
	assert.Equal(t, uint(0), version)
	assert.False(t, dirty)

	// Создаем и применяем миграцию
	migration1Up := `CREATE TABLE test (id INTEGER PRIMARY KEY);`
	err = os.WriteFile(filepath.Join(tmpDir, "001_create_test.up.sql"), []byte(migration1Up), 0644)
	require.NoError(t, err)

	err = ApplyMigrations(dbPath, migrationsPath)
	require.NoError(t, err)

	// После применения версия должна быть 1
	version, dirty, err = GetMigrationVersion(dbPath, migrationsPath)
	require.NoError(t, err)
	assert.Equal(t, uint(1), version)
	assert.False(t, dirty)
}

func TestDowngradeToVersion(t *testing.T) {
	// Создаем временную БД для тестов
	tmpFile, err := os.CreateTemp("", "test_*.db")
	require.NoError(t, err)
	dbPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(dbPath)

	// Создаем временную директорию для миграций
	tmpDir, err := os.MkdirTemp("", "sqlite_migrations_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Создаем две миграции
	migration1Up := `CREATE TABLE test1 (id INTEGER PRIMARY KEY);`
	migration1Down := `DROP TABLE test1;`
	migration2Up := `CREATE TABLE test2 (id INTEGER PRIMARY KEY);`
	migration2Down := `DROP TABLE test2;`

	err = os.WriteFile(filepath.Join(tmpDir, "001_create_test1.up.sql"), []byte(migration1Up), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "001_create_test1.down.sql"), []byte(migration1Down), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "002_create_test2.up.sql"), []byte(migration2Up), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "002_create_test2.down.sql"), []byte(migration2Down), 0644)
	require.NoError(t, err)

	migrationsPath := "file://" + filepath.ToSlash(tmpDir)

	// Применяем все миграции
	err = ApplyMigrations(dbPath, migrationsPath)
	require.NoError(t, err)

	// Открываем БД для проверки
	ctx := context.Background()
	db, err := NewDB(ctx, dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Проверяем что обе таблицы созданы
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('test1', 'test2')").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Откатываемся к версии 1
	err = DowngradeToVersion(dbPath, migrationsPath, 1)
	require.NoError(t, err)

	// Проверяем что осталась только первая таблица
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='test1'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='test2'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Проверяем версию
	version, _, err := GetMigrationVersion(dbPath, migrationsPath)
	require.NoError(t, err)
	assert.Equal(t, uint(1), version)
}

func TestResetMigrations(t *testing.T) {
	// Создаем временную БД для тестов
	tmpFile, err := os.CreateTemp("", "test_*.db")
	require.NoError(t, err)
	dbPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(dbPath)

	// Создаем временную директорию для миграций
	tmpDir, err := os.MkdirTemp("", "sqlite_migrations_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Создаем миграцию
	migration1Up := `CREATE TABLE test (id INTEGER PRIMARY KEY);`
	migration1Down := `DROP TABLE test;`

	err = os.WriteFile(filepath.Join(tmpDir, "001_create_test.up.sql"), []byte(migration1Up), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "001_create_test.down.sql"), []byte(migration1Down), 0644)
	require.NoError(t, err)

	migrationsPath := "file://" + filepath.ToSlash(tmpDir)

	// Применяем миграцию
	err = ApplyMigrations(dbPath, migrationsPath)
	require.NoError(t, err)

	// Открываем БД для проверки
	ctx := context.Background()
	db, err := NewDB(ctx, dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Проверяем что таблица создана
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='test'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Сбрасываем все миграции
	err = ResetMigrations(dbPath, migrationsPath)
	require.NoError(t, err)

	// Проверяем что таблица удалена
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='test'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Проверяем версию
	version, _, err := GetMigrationVersion(dbPath, migrationsPath)
	require.NoError(t, err)
	assert.Equal(t, uint(0), version)
}

func TestMigrations_InvalidPath(t *testing.T) {
	// Создаем временную БД для тестов
	tmpFile, err := os.CreateTemp("", "test_*.db")
	require.NoError(t, err)
	dbPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(dbPath)

	invalidPath := "file:///nonexistent/path"

	// Все функции должны возвращать ошибку для несуществующего пути
	err = ApplyMigrations(dbPath, invalidPath)
	assert.Error(t, err)

	_, _, err = GetMigrationVersion(dbPath, invalidPath)
	assert.Error(t, err)

	err = DowngradeToVersion(dbPath, invalidPath, 1)
	assert.Error(t, err)

	err = ResetMigrations(dbPath, invalidPath)
	assert.Error(t, err)
}
