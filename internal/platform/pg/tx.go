package pg

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// txKey используется как ключ для хранения транзакции в context.Context
type txKey struct{}

// Querier объединяет методы выполнения запросов, общие для пула и транзакции.
// Позволяет репозиториям работать с одним интерфейсом независимо от того,
// выполняется ли запрос в транзакции или через пул.
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Убедимся на этапе компиляции, что типы реализуют интерфейс
var (
	_ Querier = (*pgxpool.Pool)(nil)
	_ Querier = (pgx.Tx)(nil)
)

// TxRunner предоставляет возможность выполнения кода внутри транзакции.
// Реализует паттерн "функция обратного вызова" для гарантированного
// коммита или отката транзакции.
type TxRunner struct {
	Pool *pgxpool.Pool
}

// NewTxRunner создает новый TxRunner с указанным пулом подключений.
func NewTxRunner(pool *pgxpool.Pool) *TxRunner {
	return &TxRunner{Pool: pool}
}

// WithinTx выполняет функцию fn внутри транзакции с опциями по умолчанию.
// Если fn возвращает ошибку, транзакция откатывается.
// Если fn выполняется успешно (возвращает nil), транзакция коммитится.
// Транзакция доступна внутри fn через функцию PgxTx(ctx).
func (r *TxRunner) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		// Сохраняем транзакцию в контексте для доступа внутри fn
		ctx = context.WithValue(ctx, txKey{}, tx)
		return fn(ctx)
	})
}

// WithinTxWithOptions выполняет функцию fn внутри транзакции с заданными опциями.
// Если fn возвращает ошибку, транзакция откатывается.
// Если fn выполняется успешно (возвращает nil), транзакция коммитится.
// Транзакция доступна внутри fn через функцию PgxTx(ctx).
func (r *TxRunner) WithinTxWithOptions(ctx context.Context, txOptions pgx.TxOptions, fn func(ctx context.Context) error) error {
	return pgx.BeginTxFunc(ctx, r.Pool, txOptions, func(tx pgx.Tx) error {
		// Сохраняем транзакцию в контексте для доступа внутри fn
		ctx = context.WithValue(ctx, txKey{}, tx)
		return fn(ctx)
	})
}

// PgxTx извлекает активную транзакцию из контекста.
// Возвращает транзакцию и флаг, указывающий была ли найдена транзакция в контексте.
// Если транзакция не найдена, следует использовать обычный пул для выполнения запросов.
func PgxTx(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
}

// GetQuerier возвращает объект для выполнения запросов.
// Если в контексте есть активная транзакция - возвращает её,
// иначе возвращает пул подключений.
// Возвращаемый объект реализует интерфейс Querier.
func (r *TxRunner) GetQuerier(ctx context.Context) Querier {
	if tx, ok := PgxTx(ctx); ok {
		return tx
	}
	return r.Pool
}
