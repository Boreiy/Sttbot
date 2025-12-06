package pg

import (
	"errors"
	"fmt"
	"io/fs"

	migrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// ApplyMigrations применяет все доступные миграции к базе данных.
// Функция безопасна для повторного вызова - если миграции уже применены,
// ошибки не будет.
//
// Параметры:
//   - dsn: строка подключения к PostgreSQL
//   - migrationsPath: путь к директории с миграциями (например, "file://migrations")
//
// Возвращает информацию о миграциях и ошибку.
// migrate.ErrNoChange (нет новых миграций) не считается ошибкой.
func ApplyMigrations(dsn, migrationsPath string) (MigrationInfo, error) {
	m, err := migrate.New(migrationsPath, dsn)
	if err != nil {
		return MigrationInfo{}, fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer func() {
		sourceErr, dbErr := m.Close()
		_, _ = sourceErr, dbErr
	}()

	info := MigrationInfo{Applied: false, Dirty: false}

	// Получаем текущую версию до применения
	currentVersion, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return MigrationInfo{}, fmt.Errorf("failed to get current version: %w", err)
	}
	info.CurrentVersion = currentVersion
	info.Dirty = dirty

	if dirty {
		return info, fmt.Errorf("database is in dirty state at version %d", currentVersion)
	}

	// Применяем миграции
	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			// Нет новых миграций - это нормально
			return info, nil
		}
		return info, fmt.Errorf("failed to apply migrations: %w", err)
	}

	info.Applied = true
	// Получаем финальную версию
	finalVersion, _, err := m.Version()
	if err == nil {
		info.FinalVersion = finalVersion
	}

	return info, nil
}

// ApplyMigrationsLegacy применяет миграции с совместимостью старого API.
// Возвращает только ошибку для обратной совместимости.
// DEPRECATED: используйте ApplyMigrations для получения дополнительной информации.
func ApplyMigrationsLegacy(dsn, migrationsPath string) error {
	_, err := ApplyMigrations(dsn, migrationsPath)
	return err
}

// ApplyMigrationsFromFS применяет миграции из файловой системы (fs.FS).
// Полезно для встраивания миграций в бинарник с помощью embed.FS.
//
// Параметры:
//   - dsn: строка подключения к PostgreSQL
//   - fsys: файловая система с миграциями
//   - dirName: имя директории в fsys с файлами миграций
//
// Возвращает информацию о миграциях и ошибку.
func ApplyMigrationsFromFS(dsn string, fsys fs.FS, dirName string) (MigrationInfo, error) {
	sourceDriver, err := iofs.New(fsys, dirName)
	if err != nil {
		return MigrationInfo{}, fmt.Errorf("failed to create iofs source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", sourceDriver, dsn)
	if err != nil {
		return MigrationInfo{}, fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer func() {
		sourceErr, dbErr := m.Close()
		if sourceErr != nil || dbErr != nil {
			// Логируем ошибки закрытия, но не возвращаем их
			_, _ = sourceErr, dbErr
		}
	}()

	info := MigrationInfo{Applied: false, Dirty: false}

	// Получаем текущую версию до применения
	currentVersion, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return MigrationInfo{}, fmt.Errorf("failed to get current version: %w", err)
	}
	info.CurrentVersion = currentVersion
	info.Dirty = dirty

	if dirty {
		return info, fmt.Errorf("database is in dirty state at version %d", currentVersion)
	}

	// Применяем миграции
	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			// Нет новых миграций - это нормально
			return info, nil
		}
		return info, fmt.Errorf("failed to apply migrations: %w", err)
	}

	info.Applied = true
	// Получаем финальную версию
	finalVersion, _, err := m.Version()
	if err == nil {
		info.FinalVersion = finalVersion
	}

	return info, nil
}

// MigrationInfo содержит информацию о результате применения миграций.
type MigrationInfo struct {
	Applied        bool // Были ли применены новые миграции
	CurrentVersion uint // Версия до применения
	FinalVersion   uint // Версия после применения
	Dirty          bool // Находится ли БД в "грязном" состоянии
}

// GetMigrationVersion возвращает текущую версию примененных миграций.
// Полезно для логирования и отладки.
func GetMigrationVersion(dsn, migrationsPath string) (uint, bool, error) {
	m, err := migrate.New(migrationsPath, dsn)
	if err != nil {
		return 0, false, fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer func() {
		sourceErr, dbErr := m.Close()
		_, _ = sourceErr, dbErr
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

// GetMigrationVersionFromFS возвращает текущую версию миграций из fs.FS.
func GetMigrationVersionFromFS(dsn string, fsys fs.FS, dirName string) (uint, bool, error) {
	sourceDriver, err := iofs.New(fsys, dirName)
	if err != nil {
		return 0, false, fmt.Errorf("failed to create iofs source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", sourceDriver, dsn)
	if err != nil {
		return 0, false, fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer func() {
		sourceErr, dbErr := m.Close()
		_, _ = sourceErr, dbErr
	}()

	version, dirty, err := m.Version()
	if err != nil {
		if errors.Is(err, migrate.ErrNilVersion) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("failed to get migration version: %w", err)
	}

	return version, dirty, nil
}
