package sqlite

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	migrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// BuildMigrateURL строит корректный URL для golang-migrate с учётом особенностей ОС.
// На Windows для путей вида "C:\..." создаёт "sqlite:///C:/...",
// на Unix для "/..." создаёт "sqlite:///...".
func BuildMigrateURL(dbPath string) (string, error) {
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Нормализуем слеши для URL
	urlPath := filepath.ToSlash(absPath)

	// На Windows добавляем дополнительный слеш перед диском
	if runtime.GOOS == "windows" && len(urlPath) >= 2 && urlPath[1] == ':' {
		// C:/path -> /C:/path для правильного URL
		urlPath = "/" + urlPath
	}

	// Убеждаемся что путь начинается с /
	if !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}

	return "sqlite://" + urlPath, nil
}

// ApplyMigrations применяет все доступные миграции к SQLite базе данных.
// Функция безопасна для повторного вызова - если миграции уже применены,
// ошибки не будет.
//
// Параметры:
//   - dbPath: путь к SQLite базе данных
//   - migrationsPath: путь к директории с миграциями (например, "file://migrations/sqlite")
//
// Возвращает ошибку только в случае реальных проблем с миграцией.
// migrate.ErrNoChange (нет новых миграций) не считается ошибкой.
func ApplyMigrations(dbPath, migrationsPath string) error {
	// Создаем отдельное соединение для миграций
	// golang-migrate может безопасно закрыть это соединение
	databaseURL, err := BuildMigrateURL(dbPath)
	if err != nil {
		return fmt.Errorf("failed to build database URL: %w", err)
	}

	m, err := migrate.New(migrationsPath, databaseURL)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer func() {
		// Закрываем ресурсы migrate, игнорируя ошибки закрытия
		_, _ = m.Close()
	}()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	return nil
}

// GetMigrationVersion возвращает текущую версию примененных миграций.
// Полезно для логирования и отладки.
func GetMigrationVersion(dbPath, migrationsPath string) (uint, bool, error) {
	// Создаем отдельное соединение для проверки версии миграций
	databaseURL, err := BuildMigrateURL(dbPath)
	if err != nil {
		return 0, false, fmt.Errorf("failed to build database URL: %w", err)
	}

	m, err := migrate.New(migrationsPath, databaseURL)
	if err != nil {
		return 0, false, fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer func() {
		_, _ = m.Close()
	}()

	version, dirty, err := m.Version()
	if err != nil {
		// Если миграции еще не применялись, это не ошибка
		if errors.Is(err, migrate.ErrNilVersion) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("failed to get migration version: %w", err)
	}

	return version, dirty, nil
}

// DowngradeToVersion откатывает миграции до указанной версии.
// Используется для тестирования или отката проблемных миграций.
func DowngradeToVersion(dbPath, migrationsPath string, version uint) error {
	// Создаем отдельное соединение для отката миграций
	databaseURL, err := BuildMigrateURL(dbPath)
	if err != nil {
		return fmt.Errorf("failed to build database URL: %w", err)
	}

	m, err := migrate.New(migrationsPath, databaseURL)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer func() {
		_, _ = m.Close()
	}()

	if err := m.Migrate(version); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("failed to downgrade to version %d: %w", version, err)
	}

	return nil
}

// ResetMigrations откатывает все миграции (опасная операция!).
// Используется только в тестах или при необходимости полного сброса схемы.
func ResetMigrations(dbPath, migrationsPath string) error {
	// Создаем отдельное соединение для сброса миграций
	databaseURL, err := BuildMigrateURL(dbPath)
	if err != nil {
		return fmt.Errorf("failed to build database URL: %w", err)
	}

	m, err := migrate.New(migrationsPath, databaseURL)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer func() {
		_, _ = m.Close()
	}()

	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("failed to reset migrations: %w", err)
	}

	return nil
}
