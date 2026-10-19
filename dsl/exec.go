package dsl

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/gogf/gf/v2/util/gconv"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

type executor interface {
	Dialect() gooq.Dialect
	GetAll(ctx context.Context, query string, args ...any) (db.Result, error)
	GetOne(ctx context.Context, query string, args ...any) (db.Record, error)
	Exec(ctx context.Context, query string, args ...any) (sql.Result, error)
	GetValue(ctx context.Context, query string, args ...any) (any, error)
}

func (b *SelectBuilder) bind(ctx context.Context) (func(), error) {
	b.stampTenant(ctx)
	return bindBuilder(ctx, &b.executor, b.route, b.group, b.groupSet, b.explicitTx)
}

func (b *DMLBuilder) bind(ctx context.Context) (func(), error) {
	b.stampTenant(ctx)
	return bindBuilder(ctx, &b.executor, b.route, b.group, b.groupSet, b.explicitTx)
}

func bindBuilder(ctx context.Context, slot *executor, route db.Route, group string, groupSet bool, explicit *db.Tx) (func(), error) {
	stored, active, err := resolveExecutor(ctx, *slot, route, group, groupSet, explicit)
	if err != nil {
		return nil, err
	}
	*slot = active
	return func() { *slot = stored }, nil
}

func resolveExecutor(ctx context.Context, current executor, route db.Route, group string, groupSet bool, explicit *db.Tx) (stored executor, active executor, err error) {
	switch current.(type) {
	case nil, *db.DB, *db.Tx:
	default:
		return current, current, nil
	}
	if explicit != nil {
		return explicit.DB(), explicit, nil
	}
	var database *db.DB
	if groupSet {
		database, err = db.Instance(group)
	} else {
		database, err = db.Instance()
	}
	if err != nil {
		return nil, nil, err
	}
	if tx := database.TxFromContext(ctx); tx != nil {
		return database, tx, nil
	}
	return database, database.WithRoute(route), nil
}

func autoDialect(exec executor) gooq.Dialect {
	if exec == nil {
		return gooq.DialectMySQL
	}
	if dialect := exec.Dialect(); dialect != "" {
		return dialect
	}
	return gooq.DialectMySQL
}

func execValue(ctx context.Context, exec executor, query string, args []any, dest any) error {
	switch typed := exec.(type) {
	case *db.DB:
		return typed.Value(ctx, dest, query, args...)
	case *db.Tx:
		return typed.Value(ctx, dest, query, args...)
	default:
		return fmt.Errorf("gooq: Value requires a database executor")
	}
}

func execValueExact(ctx context.Context, exec executor, query string, args []any, dest any) error {
	switch typed := exec.(type) {
	case *db.DB:
		return typed.ValueExact(ctx, dest, query, args...)
	case *db.Tx:
		return typed.ValueExact(ctx, dest, query, args...)
	default:
		return fmt.Errorf("gooq: Value requires a database executor")
	}
}

func execColumnExact(ctx context.Context, exec executor, query string, args []any, dest any) error {
	switch typed := exec.(type) {
	case *db.DB:
		return typed.ColumnExact(ctx, dest, query, args...)
	case *db.Tx:
		return typed.ColumnExact(ctx, dest, query, args...)
	default:
		return fmt.Errorf("gooq: Column requires a database executor")
	}
}

func toInt64(value any) (int64, error) {
	var out int64
	if err := gconv.Scan(value, &out); err != nil {
		return 0, err
	}
	return out, nil
}

// 确认 *db.DB 与 *db.Tx 满足 executor 约定。
var (
	_ executor = (*db.DB)(nil)
	_ executor = (*db.Tx)(nil)
)

