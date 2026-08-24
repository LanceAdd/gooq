package gooq

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
)

type executor interface {
	GetAll(ctx context.Context, sql string, args ...any) (gdb.Result, error)
	GetOne(ctx context.Context, sql string, args ...any) (gdb.Record, error)
	GetScan(ctx context.Context, pointer any, sql string, args ...any) error
	GetArray(ctx context.Context, sql string, args ...any) (gdb.Array, error)
	Exec(ctx context.Context, sql string, args ...any) (sql.Result, error)
	GetValue(ctx context.Context, sql string, args ...any) (gdb.Value, error)
}

type txExecutor struct {
	tx gdb.TX
}

func (e *txExecutor) GetAll(ctx context.Context, sql string, args ...any) (gdb.Result, error) {
	return e.tx.Ctx(ctx).GetAll(sql, args...)
}

func (e *txExecutor) GetOne(ctx context.Context, sql string, args ...any) (gdb.Record, error) {
	return e.tx.Ctx(ctx).GetOne(sql, args...)
}

func (e *txExecutor) GetScan(ctx context.Context, pointer any, sql string, args ...any) error {
	return e.tx.Ctx(ctx).GetScan(pointer, sql, args...)
}

func (e *txExecutor) Exec(ctx context.Context, sql string, args ...any) (sql.Result, error) {
	return e.tx.Ctx(ctx).Exec(sql, args...)
}

func (e *txExecutor) GetArray(ctx context.Context, sql string, args ...any) (gdb.Array, error) {
	rows, err := e.tx.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var array gdb.Array
	for rows.Next() {
		var value any
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		array = append(array, gvar.New(value))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return array, nil
}

func (e *txExecutor) GetValue(ctx context.Context, sql string, args ...any) (gdb.Value, error) {
	return e.tx.Ctx(ctx).GetValue(sql, args...)
}

func autoDialect(e executor) Dialect {
	if db, ok := e.(gdb.DB); ok {
		if cfg := db.GetConfig(); cfg != nil && cfg.Type != "" {
			return Dialect(cfg.Type)
		}
	}
	if g, ok := e.(interface{ GetDB() gdb.DB }); ok {
		if db := g.GetDB(); db != nil {
			if cfg := db.GetConfig(); cfg != nil && cfg.Type != "" {
				return Dialect(cfg.Type)
			}
		}
	}
	return DialectMySQL
}

func (e *txExecutor) GetDB() gdb.DB {
	if g, ok := e.tx.(interface{ GetDB() gdb.DB }); ok {
		return g.GetDB()
	}
	return nil
}

func Get[T any](record Record, field Field[T]) T {
	var zero T
	if record == nil {
		return zero
	}
	v, ok := record[field.ColumnName()]
	if !ok || v == nil {
		return zero
	}
	var value T
	_ = v.Scan(&value)
	return value
}

func scanExec(ctx context.Context, e executor, sql string, args []any, dest any) error {
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("gooq: Scan dest must be a non-nil pointer")
	}
	switch rv.Elem().Kind() {
	case reflect.Struct:
		return e.GetScan(ctx, dest, sql, args...)
	case reflect.Slice, reflect.Array:
		if isStructSlice(rv.Elem().Type()) {
			return e.GetScan(ctx, dest, sql, args...)
		}
		array, err := e.GetArray(ctx, sql, args...)
		if err != nil {
			return err
		}
		return array.Scan(dest)
	default:
		value, err := e.GetValue(ctx, sql, args...)
		if err != nil {
			return err
		}
		return value.Scan(dest)
	}
}

func isStructSlice(t reflect.Type) bool {
	elem := t.Elem()
	if elem.Kind() == reflect.Struct {
		return true
	}
	return elem.Kind() == reflect.Ptr && elem.Elem().Kind() == reflect.Struct
}
