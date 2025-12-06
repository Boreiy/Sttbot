package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// txKey используется как ключ для хранения транзакции в context.Context
type txKey struct{}

// Querier объединяет методы выполнения запросов, общие для БД и транзакции.
// Позволяет репозиториям работать с одним интерфейсом независимо от того,
// выполняется ли запрос в транзакции или через основное подключение.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
}

// Убедимся на этапе компиляции, что типы реализуют интерфейс
var (
	_ Querier = (*sql.DB)(nil)
	_ Querier = (*sql.Tx)(nil)
	_ Querier = (*manualTx)(nil)
)

// writeRequest представляет запрос на выполнение операции записи в очереди
type writeRequest struct {
	fn       func(context.Context) error
	resultCh chan error
	ctx      context.Context
}

// TxRunner предоставляет возможность выполнения кода внутри транзакции.
// Реализует паттерн "функция обратного вызова" для гарантированного
// коммита или отката транзакции, с поддержкой очереди записи и ретраев.
type TxRunner struct {
	DB             *sql.DB
	TxLockMode     TxLockMode
	RetryConfig    *RetryConfig
	writeQueue     chan writeRequest
	writeQueueDone chan struct{}
	enableQueue    bool
}

// NewTxRunner создает новый TxRunner с указанным подключением к БД и настройками по умолчанию.
func NewTxRunner(db *sql.DB) *TxRunner {
	return NewTxRunnerWithOptions(db, DefaultDBOptions())
}

// RetryConfig содержит настройки для повторных попыток.
type RetryConfig struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

// NewTxRunnerWithOptions создает новый TxRunner с указанными опциями.
func NewTxRunnerWithOptions(db *sql.DB, opts DBOptions) *TxRunner {
	runner := &TxRunner{
		DB:         db,
		TxLockMode: opts.TxLockMode,
		RetryConfig: &RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     500 * time.Millisecond,
			Multiplier:   2.0,
		},
		enableQueue: opts.EnableWriteQueue,
	}

	// Запускаем очередь записи если включена
	if opts.EnableWriteQueue {
		runner.writeQueue = make(chan writeRequest, opts.WriteQueueSize)
		runner.writeQueueDone = make(chan struct{})
		go runner.runWriteQueue()
	}

	return runner
}

// Close закрывает TxRunner и очередь записи если она активна.
func (r *TxRunner) Close() error {
	if r.enableQueue && r.writeQueue != nil {
		close(r.writeQueue)
		<-r.writeQueueDone
	}
	return nil
}

// WithinTx выполняет функцию fn внутри транзакции.
// Если fn возвращает ошибку, транзакция откатывается.
// Если fn выполняется успешно (возвращает nil), транзакция коммитится.
// Транзакция доступна внутри fn через функцию SqlTx(ctx).
// Поддерживает очередь записи и ретраи на SQLITE_BUSY.
func (r *TxRunner) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	// Если включена очередь записи - направляем в неё
	if r.enableQueue {
		return r.enqueueWrite(ctx, fn)
	}

	// Иначе выполняем напрямую с ретраями
	return r.executeWithRetry(ctx, fn)
}

// WithinTxWrite выполняет операцию записи внутри транзакции.
// Всегда использует очередь если она включена, иначе выполняет с ретраями.
func (r *TxRunner) WithinTxWrite(ctx context.Context, fn func(ctx context.Context) error) error {
	return r.WithinTx(ctx, fn)
}

// WithinTxRead выполняет операцию чтения внутри транзакции.
// Игнорирует очередь записи и выполняет напрямую.
func (r *TxRunner) WithinTxRead(ctx context.Context, fn func(ctx context.Context) error) error {
	return r.executeWithRetry(ctx, fn)
}

// WithinSavepoint выполняет функцию fn внутри savepoint.
// Если уже есть активная транзакция, создаёт savepoint внутри неё.
// Если нет активной транзакции, создаёт новую транзакцию и savepoint.
// При ошибке откатывается к savepoint, при успехе - освобождает savepoint.
func (r *TxRunner) WithinSavepoint(ctx context.Context, fn func(ctx context.Context) error) error {
	// Проверяем, есть ли уже активная транзакция
	if existingQuerier, hasActiveTx := GetTxQuerier(ctx); hasActiveTx {
		// Если есть активная транзакция - создаём savepoint внутри неё
		return r.executeSavepoint(ctx, existingQuerier, fn)
	}

	// Если нет активной транзакции - создаём новую транзакцию и savepoint внутри неё
	return r.executeWithRetry(ctx, func(txCtx context.Context) error {
		querier := r.GetQuerier(txCtx)
		return r.executeSavepoint(txCtx, querier, fn)
	})
}

// SqlTx извлекает активную транзакцию из контекста.
// Возвращает транзакцию и флаг, указывающий была ли найдена транзакция в контексте.
// Если транзакция не найдена, следует использовать обычное подключение к БД для выполнения запросов.
func SqlTx(ctx context.Context) (*sql.Tx, bool) {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return tx, true
	}
	// Для manual транзакций возвращаем false, так как нет настоящего *sql.Tx
	return nil, false
}

// GetTxQuerier извлекает любой тип транзакции (sql.Tx или manualTx) из контекста как Querier.
func GetTxQuerier(ctx context.Context) (Querier, bool) {
	if querier, ok := ctx.Value(txKey{}).(Querier); ok {
		return querier, true
	}
	return nil, false
}

// GetQuerier возвращает объект для выполнения запросов.
// Если в контексте есть активная транзакция - возвращает её,
// иначе возвращает основное подключение к БД.
// Возвращаемый объект реализует интерфейс Querier.
func (r *TxRunner) GetQuerier(ctx context.Context) Querier {
	if querier, ok := GetTxQuerier(ctx); ok {
		return querier
	}
	return r.DB
}

// BeginTx начинает новую транзакцию с заданными опциями и сохраняет её в контексте.
// Возвращает новый контекст с транзакцией и саму транзакцию для ручного управления.
// Внимание: при использовании этого метода вы отвечаете за ручной коммит/откат!
func (r *TxRunner) BeginTx(ctx context.Context, opts *sql.TxOptions) (context.Context, *sql.Tx, error) {
	tx, err := r.DB.BeginTx(ctx, opts)
	if err != nil {
		return ctx, nil, err
	}

	ctx = context.WithValue(ctx, txKey{}, tx)
	return ctx, tx, nil
}

// runWriteQueue обрабатывает очередь операций записи в отдельной goroutine.
func (r *TxRunner) runWriteQueue() {
	defer close(r.writeQueueDone)

	for req := range r.writeQueue {
		select {
		case <-req.ctx.Done():
			req.resultCh <- req.ctx.Err()
		default:
			err := r.executeWithRetry(req.ctx, req.fn)
			req.resultCh <- err
		}
		close(req.resultCh)
	}
}

// enqueueWrite добавляет операцию записи в очередь.
func (r *TxRunner) enqueueWrite(ctx context.Context, fn func(context.Context) error) error {
	req := writeRequest{
		fn:       fn,
		resultCh: make(chan error, 1),
		ctx:      ctx,
	}

	select {
	case r.writeQueue <- req:
		select {
		case err := <-req.resultCh:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	case <-ctx.Done():
		return ctx.Err()
	}
}

// executeWithRetry выполняет транзакцию с ретраями на SQLITE_BUSY.
func (r *TxRunner) executeWithRetry(ctx context.Context, fn func(context.Context) error) error {
	delay := r.RetryConfig.InitialDelay

	for attempt := 1; attempt <= r.RetryConfig.MaxAttempts; attempt++ {
		err := r.executeTx(ctx, fn)

		// Если ошибки нет или это последняя попытка - возвращаем результат
		if err == nil || attempt == r.RetryConfig.MaxAttempts {
			return err
		}

		// Проверяем, является ли ошибка retryable
		if !r.isSQLiteBusyError(err) {
			return err
		}

		// Ожидаем перед следующей попыткой
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// Увеличиваем задержку для следующей попытки
			delay = time.Duration(float64(delay) * r.RetryConfig.Multiplier)
			if delay > r.RetryConfig.MaxDelay {
				delay = r.RetryConfig.MaxDelay
			}
		}
	}

	return fmt.Errorf("max retry attempts exceeded")
}

// executeTx выполняет одну попытку транзакции.
func (r *TxRunner) executeTx(ctx context.Context, fn func(context.Context) error) error {
	// Проверяем, есть ли уже активная транзакция в контексте
	if _, existingTx := GetTxQuerier(ctx); existingTx {
		return fmt.Errorf("nested transactions are not supported by SQLite")
	}

	// Для SQLite нужно использовать специальный BEGIN с режимом блокировки
	if r.TxLockMode != TxLockDeferred {
		return r.executeTxWithLockMode(ctx, fn)
	}

	// Стандартная DEFERRED транзакция
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// Сохраняем транзакцию в контексте для доступа внутри fn
	ctx = context.WithValue(ctx, txKey{}, tx)

	// Выполняем функцию и обрабатываем результат
	if err := fn(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

// executeTxWithLockMode выполняет транзакцию с указанным режимом блокировки.
func (r *TxRunner) executeTxWithLockMode(ctx context.Context, fn func(context.Context) error) error {
	// Начинаем транзакцию с указанным режимом блокировки
	beginQuery := fmt.Sprintf("BEGIN %s", r.TxLockMode)
	_, err := r.DB.ExecContext(ctx, beginQuery)
	if err != nil {
		return err
	}

	// Создаем псевдо-транзакцию для совместимости с интерфейсом
	// В SQLite нельзя получить *sql.Tx после ручного BEGIN,
	// поэтому используем специальный wrapper
	manualTxWrapper := &manualTx{db: r.DB, ctx: ctx}
	ctx = context.WithValue(ctx, txKey{}, manualTxWrapper)

	// Выполняем функцию
	if err := fn(ctx); err != nil {
		_, _ = r.DB.ExecContext(ctx, "ROLLBACK")
		return err
	}

	// Коммитим транзакцию
	_, err = r.DB.ExecContext(ctx, "COMMIT")
	return err
}

// manualTx представляет ручную транзакцию для поддержки IMMEDIATE/EXCLUSIVE режимов.
type manualTx struct {
	db  *sql.DB
	ctx context.Context
}

func (m *manualTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return m.db.ExecContext(ctx, query, args...)
}

func (m *manualTx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return m.db.QueryContext(ctx, query, args...)
}

func (m *manualTx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return m.db.QueryRowContext(ctx, query, args...)
}

func (m *manualTx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return m.db.PrepareContext(ctx, query)
}

// isSQLiteBusyError проверяет, является ли ошибка SQLITE_BUSY.
func (r *TxRunner) isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	return strings.Contains(errStr, "database is locked") ||
		strings.Contains(errStr, "SQLITE_BUSY") ||
		strings.Contains(errStr, "database table is locked")
}

// executeSavepoint выполняет функцию внутри savepoint.
func (r *TxRunner) executeSavepoint(ctx context.Context, querier Querier, fn func(context.Context) error) error {
	// Генерируем уникальное имя savepoint
	savepointName := fmt.Sprintf("sp_%d", time.Now().UnixNano())

	// Создаём savepoint
	if _, err := querier.ExecContext(ctx, "SAVEPOINT "+savepointName); err != nil {
		return fmt.Errorf("failed to create savepoint %s: %w", savepointName, err)
	}

	// Выполняем функцию
	if err := fn(ctx); err != nil {
		// При ошибке откатываемся к savepoint
		if _, rollbackErr := querier.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+savepointName); rollbackErr != nil {
			// Если не удалось откатиться к savepoint, возвращаем обе ошибки
			return fmt.Errorf("failed to rollback to savepoint %s: %v (original error: %w)", savepointName, rollbackErr, err)
		}
		// Освобождаем savepoint после отката
		_, _ = querier.ExecContext(ctx, "RELEASE SAVEPOINT "+savepointName)
		return err
	}

	// При успехе освобождаем savepoint
	if _, err := querier.ExecContext(ctx, "RELEASE SAVEPOINT "+savepointName); err != nil {
		return fmt.Errorf("failed to release savepoint %s: %w", savepointName, err)
	}

	return nil
}
