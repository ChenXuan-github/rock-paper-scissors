package database

import (
	"context"
	"database/sql"
)

// Executor 是 Repository 执行 SQL 所需的最小能力集合。
// *sql.DB 和 *sql.Tx 都拥有这些方法，因此普通操作和事务操作可以复用同一套 Repository。
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// 编译期检查：如果未来 Go 标准库的方法签名变化，这里会立即编译失败。
var (
	_ Executor = (*sql.DB)(nil)
	_ Executor = (*sql.Tx)(nil)
)
