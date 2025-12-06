package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // SQLite драйвер
)

// TxLockMode определяет режим блокировки транзакций SQLite
type TxLockMode string

const (
	// TxLockDeferred - откладывает блокировку до первого чтения/записи (по умолчанию SQLite)
	TxLockDeferred TxLockMode = "DEFERRED"
	// TxLockImmediate - немедленно захватывает RESERVED блокировку для избежания SQLITE_BUSY при записи
	TxLockImmediate TxLockMode = "IMMEDIATE"
	// TxLockExclusive - немедленно захватывает EXCLUSIVE блокировку
	TxLockExclusive TxLockMode = "EXCLUSIVE"
)

// AccessMode определяет режим доступа к SQLite базе данных
type AccessMode string

const (
	// AccessModeReadWrite - режим чтения и записи (по умолчанию)
	AccessModeReadWrite AccessMode = "rw"
	// AccessModeReadOnly - режим только для чтения
	AccessModeReadOnly AccessMode = "ro"
	// AccessModeReadWriteCreate - режим чтения/записи с созданием файла если не существует
	AccessModeReadWriteCreate AccessMode = "rwc"
)

// DBOptions содержит настройки для SQLite базы данных.
type DBOptions struct {
	// ConnMaxLifetime - максимальное время жизни соединения
	ConnMaxLifetime time.Duration
	// ConnMaxIdleTime - максимальное время простоя соединения
	ConnMaxIdleTime time.Duration
	// MaxOpenConns - максимальное количество открытых соединений
	MaxOpenConns int
	// MaxIdleConns - максимальное количество idle соединений
	MaxIdleConns int
	// PingTimeout - таймаут для проверки соединения при создании БД
	PingTimeout time.Duration
	// WALMode - использовать ли WAL режим для лучшей производительности
	WALMode bool
	// ForeignKeys - включить ли проверку внешних ключей
	ForeignKeys bool
	// BusyTimeout - таймаут ожидания при SQLITE_BUSY (в миллисекундах)
	BusyTimeout time.Duration
	// TxLockMode - режим блокировки для новых транзакций
	TxLockMode TxLockMode
	// EnableWriteQueue - включить очередь для сериализации операций записи
	EnableWriteQueue bool
	// WriteQueueSize - размер буфера очереди записи (по умолчанию 100)
	WriteQueueSize int
	// AccessMode - режим доступа к базе данных
	AccessMode AccessMode
}

// DefaultDBOptions возвращает настройки по умолчанию, оптимизированные для embedded использования.
func DefaultDBOptions() DBOptions {
	return DBOptions{
		ConnMaxLifetime:  time.Hour,
		ConnMaxIdleTime:  10 * time.Minute,
		MaxOpenConns:     4, // Снижено для SQLite (один писатель)
		MaxIdleConns:     1,
		PingTimeout:      5 * time.Second,
		WALMode:          true,                // WAL режим для лучшей производительности
		ForeignKeys:      true,                // Включаем проверку внешних ключей
		BusyTimeout:      5 * time.Second,     // 5 секунд ожидания при блокировке
		TxLockMode:       TxLockDeferred,      // По умолчанию стандартный режим для совместимости
		EnableWriteQueue: false,               // По умолчанию отключена
		WriteQueueSize:   100,                 // Размер буфера очереди
		AccessMode:       AccessModeReadWrite, // По умолчанию чтение и запись
	}
}

// NewDB создает новое подключение к SQLite базе данных с настройками по умолчанию.
// Параметры оптимизированы для embedded использования в приложении.
func NewDB(ctx context.Context, dbPath string) (*sql.DB, error) {
	return NewDBWithOptions(ctx, dbPath, DefaultDBOptions())
}

// NewReadOnlyDB создает подключение к SQLite базе данных в режиме только для чтения.
func NewReadOnlyDB(ctx context.Context, dbPath string) (*sql.DB, error) {
	opts := DefaultDBOptions()
	opts.AccessMode = AccessModeReadOnly
	opts.EnableWriteQueue = false // Очередь записи не нужна для read-only
	return NewDBWithOptions(ctx, dbPath, opts)
}

// NewDBWithMode создает подключение к SQLite с указанным режимом доступа.
func NewDBWithMode(ctx context.Context, dbPath string, mode AccessMode) (*sql.DB, error) {
	opts := DefaultDBOptions()
	opts.AccessMode = mode
	if mode == AccessModeReadOnly {
		opts.EnableWriteQueue = false // Очередь записи не нужна для read-only
	}
	return NewDBWithOptions(ctx, dbPath, opts)
}

// NewDBFromDSN создает подключение к SQLite используя готовую DSN строку.
// Эта функция полезна когда нужен полный контроль над DSN или для совместимости.
func NewDBFromDSN(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Применяем базовые настройки пула соединений
	opts := DefaultDBOptions()
	db.SetConnMaxLifetime(opts.ConnMaxLifetime)
	db.SetConnMaxIdleTime(opts.ConnMaxIdleTime)
	db.SetMaxOpenConns(opts.MaxOpenConns)
	db.SetMaxIdleConns(opts.MaxIdleConns)

	// Проверяем соединение с БД
	pingCtx, cancel := context.WithTimeout(ctx, opts.PingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	// Примечание: PRAGMA настройки не применяются автоматически при использовании DSN.
	// Если нужны PRAGMA настройки, используйте NewDBWithOptions().

	return db, nil
}

// NewDBWithOptions создает новое подключение к SQLite с заданными параметрами.
func NewDBWithOptions(ctx context.Context, dbPath string, opts DBOptions) (*sql.DB, error) {
	// Создаем директорию для БД если её нет
	if dir := filepath.Dir(dbPath); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Строим DSN с параметрами
	dsn := buildDSN(dbPath, opts)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Применяем настройки соединения
	db.SetConnMaxLifetime(opts.ConnMaxLifetime)
	db.SetConnMaxIdleTime(opts.ConnMaxIdleTime)
	db.SetMaxOpenConns(opts.MaxOpenConns)
	db.SetMaxIdleConns(opts.MaxIdleConns)

	// Проверяем соединение с БД с настраиваемым таймаутом
	pingCtx, cancel := context.WithTimeout(ctx, opts.PingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	// Применяем PRAGMA настройки после открытия соединения
	if err := applyPragmaSettings(ctx, db, opts); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to apply PRAGMA settings: %w", err)
	}

	return db, nil
}

// buildDSN строит DSN строку для SQLite с минимальными параметрами.
// Большинство настроек теперь применяется через PRAGMA после открытия.
func buildDSN(dbPath string, opts DBOptions) string {
	params := []string{}

	// Добавляем режим доступа только если он отличается от умолчания
	if opts.AccessMode != "" && opts.AccessMode != AccessModeReadWrite {
		params = append(params, fmt.Sprintf("mode=%s", opts.AccessMode))
	}

	// Добавляем только базовые параметры через DSN
	if opts.BusyTimeout > 0 {
		timeoutMs := int(opts.BusyTimeout.Milliseconds())
		params = append(params, fmt.Sprintf("_busy_timeout=%d", timeoutMs))
	}

	// Если есть параметры - добавляем их к пути
	if len(params) > 0 {
		return dbPath + "?" + strings.Join(params, "&")
	}

	return dbPath
}

// NewInMemoryDB создает in-memory SQLite базу данных для тестов.
// Ограничивает пул соединений до 1 для обеспечения единого состояния схемы.
func NewInMemoryDB(ctx context.Context) (*sql.DB, error) {
	opts := DefaultDBOptions()
	opts.WALMode = false          // WAL не поддерживается для in-memory БД
	opts.MaxOpenConns = 1         // Критично для in-memory БД - одно соединение
	opts.MaxIdleConns = 1         // Одно idle соединение
	opts.EnableWriteQueue = false // Не нужно для одного соединения

	return NewDBWithOptions(ctx, ":memory:", opts)
}

// NewTestDB создает временную SQLite базу данных для тестов.
// БД будет создана в системной временной директории с уникальным именем.
func NewTestDB(ctx context.Context) (*sql.DB, string, error) {
	// Создаем временный файл
	tmpFile, err := os.CreateTemp("", "test_db_*.sqlite")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close() // Закрываем файл, будем работать через sql.DB

	db, err := NewDB(ctx, tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath) // Убираем файл если не удалось подключиться
		return nil, "", err
	}

	return db, tmpPath, nil
}

// CleanupTestDB закрывает тестовую БД и удаляет файл.
func CleanupTestDB(db *sql.DB, dbPath string) error {
	if db != nil {
		_ = db.Close()
	}
	if dbPath != "" && dbPath != ":memory:" {
		return os.Remove(dbPath)
	}
	return nil
}

// applyPragmaSettings применяет PRAGMA настройки к открытому соединению.
// Это обеспечивает надёжность применения настроек независимо от драйвера.
func applyPragmaSettings(ctx context.Context, db *sql.DB, opts DBOptions) error {
	pragmas := make([]string, 0, 5)

	// Включаем проверку внешних ключей
	if opts.ForeignKeys {
		pragmas = append(pragmas, "PRAGMA foreign_keys = ON")
	}

	// Устанавливаем режим журнала
	if opts.WALMode {
		pragmas = append(pragmas, "PRAGMA journal_mode = WAL")
	}

	// Устанавливаем уровень синхронизации
	pragmas = append(pragmas, "PRAGMA synchronous = NORMAL")

	// Устанавливаем busy timeout если указан
	if opts.BusyTimeout > 0 {
		timeoutMs := int(opts.BusyTimeout.Milliseconds())
		pragmas = append(pragmas, fmt.Sprintf("PRAGMA busy_timeout = %d", timeoutMs))
	}

	// Применяем все PRAGMA настройки
	for _, pragma := range pragmas {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("failed to execute %s: %w", pragma, err)
		}
	}

	return nil
}
