package dsl

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"

	"github.com/gogf/gf/v2/util/gconv"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

type executor interface {
	Dialect() gooq.Dialect
	GetAll(ctx context.Context, query string, args ...any) (gooq.Result, error)
	GetOne(ctx context.Context, query string, args ...any) (gooq.Record, error)
	GetScan(ctx context.Context, dest any, query string, args ...any) error
	Exec(ctx context.Context, query string, args ...any) (sql.Result, error)
	GetValue(ctx context.Context, query string, args ...any) (any, error)
}

func (b *SelectBuilder) bind(ctx context.Context) (func(), error) {
	return bindBuilder(ctx, &b.executor, b.route, b.group, b.groupSet)
}

func (b *DMLBuilder) bind(ctx context.Context) (func(), error) {
	return bindBuilder(ctx, &b.executor, b.route, b.group, b.groupSet)
}

func bindBuilder(ctx context.Context, slot *executor, route db.Route, group string, groupSet bool) (func(), error) {
	stored, active, err := resolveExecutor(ctx, *slot, route, group, groupSet)
	if err != nil {
		return nil, err
	}
	*slot = active
	return func() { *slot = stored }, nil
}

func resolveExecutor(ctx context.Context, current executor, route db.Route, group string, groupSet bool) (stored executor, active executor, err error) {
	switch current.(type) {
	case nil, *db.DB, *db.Tx:
	default:
		return current, current, nil
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
	if tx := db.TxFromContext(ctx, database.Group()); tx != nil {
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

func scanExec(ctx context.Context, exec executor, query string, args []any, dest any) error {
	if err := gooq.CheckScan(dest); err != nil {
		return err
	}
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("gooq: Scan dest must be a non-nil pointer")
	}
	switch rv.Elem().Kind() {
	case reflect.Slice, reflect.Array, reflect.Struct:
		return exec.GetScan(ctx, dest, query, args...)
	case reflect.Pointer:
		if rv.Elem().Type().Elem().Kind() == reflect.Struct {
			return exec.GetScan(ctx, dest, query, args...)
		}
	default:
	}
	value, err := exec.GetValue(ctx, query, args...)
	if err != nil {
		return err
	}
	return assignAny(rv.Elem(), value)
}

func assignAny(dst reflect.Value, src any) error {
	if !dst.CanAddr() {
		return fmt.Errorf("gooq: destination cannot be addressed")
	}
	return gooq.Assign(dst.Addr().Interface(), src)
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

func execColumn(ctx context.Context, exec executor, query string, args []any, dest any) error {
	switch typed := exec.(type) {
	case *db.DB:
		return typed.Column(ctx, dest, query, args...)
	case *db.Tx:
		return typed.Column(ctx, dest, query, args...)
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

// Ensure *db.DB and *db.Tx satisfy the executor contract.
var (
	_ executor = (*db.DB)(nil)
	_ executor = (*db.Tx)(nil)
)

// isEmptyResult reports whether a scanned value has no rows or a zero scalar.
func isEmptyResult(value any) bool {
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return false
	}
	rv = rv.Elem()
	switch rv.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map:
		return rv.Len() == 0
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return rv.IsZero()
	default:
		return false
	}
}
