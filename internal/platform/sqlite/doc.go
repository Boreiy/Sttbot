// Package sqlite предоставляет инфраструктурные компоненты для работы с SQLite.
//
// Основные возможности:
// - Инициализация БД с оптимизированными настройками
// - Управление транзакциями с поддержкой savepoints
// - Система миграций с кроссплатформенной поддержкой
// - Управление конкуренцией записи (ретраи, очереди, блокировки)
// - Режимы доступа (read-only, read-write-create)
// - Тестовые хелперы для удобного тестирования
//
// # Быстрый старт
//
// Создание базы данных с настройками по умолчанию:
//
//	ctx := context.Background()
//	db, err := sqlite.NewDB(ctx, "app.db")
//	if err != nil {
//		return err
//	}
//	defer db.Close()
//
// # Управление транзакциями
//
// Простые транзакции:
//
//	runner := sqlite.NewTxRunner(db)
//	err = runner.WithinTx(ctx, func(ctx context.Context) error {
//		querier := runner.GetQuerier(ctx)
//		_, err := querier.ExecContext(ctx, "INSERT INTO users (name) VALUES (?)", "John")
//		return err
//	})
//
// Savepoints для вложенных транзакций:
//
//	err = runner.WithinTx(ctx, func(outerCtx context.Context) error {
//		// Выполняем операции во внешней транзакции
//		return runner.WithinSavepoint(outerCtx, func(innerCtx context.Context) error {
//			// Операции в savepoint - могут быть отменены независимо
//			return nil
//		})
//	})
//
// Разделение операций чтения и записи:
//
//	// Операция чтения (не использует очередь записи)
//	err = runner.WithinTxRead(ctx, func(ctx context.Context) error { ... })
//
//	// Операция записи (использует очередь если включена)
//	err = runner.WithinTxWrite(ctx, func(ctx context.Context) error { ... })
//
// # Настройки конкуренции
//
// Для высоконагруженных приложений можно включить очередь записи:
//
//	opts := sqlite.DefaultDBOptions()
//	opts.EnableWriteQueue = true
//	opts.TxLockMode = sqlite.TxLockImmediate  // Ранний захват блокировок
//	db, err := sqlite.NewDBWithOptions(ctx, "app.db", opts)
//
// # Режимы доступа
//
// Read-only база данных:
//
//	db, err := sqlite.NewReadOnlyDB(ctx, "app.db")
//
// Указание конкретного режима:
//
//	db, err := sqlite.NewDBWithMode(ctx, "app.db", sqlite.AccessModeReadWriteCreate)
//
// # Миграции
//
// Применение миграций из директории:
//
//	err = sqlite.ApplyMigrations("app.db", "file://migrations/sqlite")
//
// # Тестирование
//
// In-memory база для тестов:
//
//	func TestSomething(t *testing.T) {
//		testDB := sqlite.NewTestDBInMemory(t)
//		// testDB.DB, testDB.TxRunner доступны для использования
//		// Автоматическая очистка после теста
//	}
//
// Файловая база для интеграционных тестов:
//
//	func TestWithMigrations(t *testing.T) {
//		testDB := sqlite.NewTestDBFile(t)
//		testDB.ApplyTestMigrations(t, "file://migrations")
//		// Работаем с настоящей БД, автоматическая очистка
//	}
package sqlite
