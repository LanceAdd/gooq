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

// Structs 将结果扫入结构体切片（等价 gdb.Result.Structs，按 orm/json tag 映射）。
func (r Result) Structs(pointer any) error {
	result := make(gdb.Result, len(r))
	for i, record := range r {
		result[i] = gdb.Record(record)
	}
	return result.Structs(pointer)
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

// isEmptyResult 判断查询结果是否为空（空集合或零值标量）；struct 不判定（无空语义）。
func isEmptyResult(v any) bool {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
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

func isStructSlice(t reflect.Type) bool {
	elem := t.Elem()
	if elem.Kind() == reflect.Struct {
		return true
	}
	return elem.Kind() == reflect.Ptr && elem.Elem().Kind() == reflect.Struct
}
