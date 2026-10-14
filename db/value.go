package db

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
)

// GetAll 读取全部行。没有行时返回 nil。
func (d *DB) GetAll(ctx context.Context, query string, args ...any) (Result, error) {
	var result Result
	err := d.observe(ctx, query, args, func(ctx context.Context) error {
		conn, err := d.executor(ctx, false)
		if err != nil {
			return err
		}
		result, err = readAll(ctx, conn, d.driver, query, d.bindArgs(args)...)
		return err
	})
	return result, err
}

// GetAll 在事务内读取全部行。没有行时返回 nil。
func (t *Tx) GetAll(ctx context.Context, query string, args ...any) (Result, error) {
	var result Result
	err := t.db.observe(ctx, query, args, func(ctx context.Context) error {
		var err error
		result, err = readAll(ctx, t.sql, t.db.driver, query, t.db.bindArgs(args)...)
		return err
	})
	return result, err
}

// GetOne 读取第一行。没有行时返回 nil。
func (d *DB) GetOne(ctx context.Context, query string, args ...any) (Record, error) {
	var record Record
	err := d.observe(ctx, query, args, func(ctx context.Context) error {
		conn, err := d.executor(ctx, false)
		if err != nil {
			return err
		}
		record, err = readOne(ctx, conn, d.driver, query, d.bindArgs(args)...)
		return err
	})
	return record, err
}

// GetOne 在事务内读取第一行。没有行时返回 nil。
func (t *Tx) GetOne(ctx context.Context, query string, args ...any) (Record, error) {
	var record Record
	err := t.db.observe(ctx, query, args, func(ctx context.Context) error {
		var err error
		record, err = readOne(ctx, t.sql, t.db.driver, query, t.db.bindArgs(args)...)
		return err
	})
	return record, err
}

// GetValue 读取第一行的第一列。没有行或值为 NULL 时返回 nil。
func (d *DB) GetValue(ctx context.Context, query string, args ...any) (any, error) {
	var value any
	err := d.observe(ctx, query, args, func(ctx context.Context) error {
		conn, err := d.executor(ctx, false)
		if err != nil {
			return err
		}
		value, err = readValue(ctx, conn, d.driver, query, d.bindArgs(args)...)
		return err
	})
	return value, err
}

// GetValue 在事务内读取第一行的第一列。没有行或值为 NULL 时返回 nil。
func (t *Tx) GetValue(ctx context.Context, query string, args ...any) (any, error) {
	var value any
	err := t.db.observe(ctx, query, args, func(ctx context.Context) error {
		var err error
		value, err = readValue(ctx, t.sql, t.db.driver, query, t.db.bindArgs(args)...)
		return err
	})
	return value, err
}

// Value 把第一行的第一列写入 dest。
// dest 是指向一个单元格的非空指针，例如 *time.Time、*sql.NullString 或 *int。
// 没有行时返回 sql.ErrNoRows。NULL 写成零值。行结构体用 GetOne 再 Record.Struct。
func (d *DB) Value(ctx context.Context, dest any, query string, args ...any) error {
	if err := CheckValue(dest); err != nil {
		return err
	}
	if d != nil && d.dryRun != nil && d.dryRun.Val() {
		return d.observe(ctx, query, args, func(context.Context) error { return nil })
	}
	value, found, _, err := d.scalar(ctx, query, args...)
	if err != nil {
		return err
	}
	if !found {
		return sql.ErrNoRows
	}
	return Assign(dest, value)
}

// Value 在事务内把第一行的第一列写入 dest。约束与 DB.Value 相同。
func (t *Tx) Value(ctx context.Context, dest any, query string, args ...any) error {
	if err := CheckValue(dest); err != nil {
		return err
	}
	if t != nil && t.db != nil && t.db.dryRun != nil && t.db.dryRun.Val() {
		return t.db.observe(ctx, query, args, func(context.Context) error { return nil })
	}
	value, found, _, err := t.scalar(ctx, query, args...)
	if err != nil {
		return err
	}
	if !found {
		return sql.ErrNoRows
	}
	return Assign(dest, value)
}

// ValueExact 写入恰好一列的第一行。结果列数不是 1 时返回错误。没有行时返回 sql.ErrNoRows。
func (d *DB) ValueExact(ctx context.Context, dest any, query string, args ...any) error {
	if err := CheckValue(dest); err != nil {
		return err
	}
	if d != nil && d.dryRun != nil && d.dryRun.Val() {
		return d.observe(ctx, query, args, func(context.Context) error { return nil })
	}
	value, found, width, err := d.scalar(ctx, query, args...)
	if err != nil {
		return err
	}
	if err = oneColumnWidth(width); err != nil {
		return err
	}
	if !found {
		return sql.ErrNoRows
	}
	return Assign(dest, value)
}

// ValueExact 在事务内写入恰好一列的第一行。约束与 DB.ValueExact 相同。
func (t *Tx) ValueExact(ctx context.Context, dest any, query string, args ...any) error {
	if err := CheckValue(dest); err != nil {
		return err
	}
	if t != nil && t.db != nil && t.db.dryRun != nil && t.db.dryRun.Val() {
		return t.db.observe(ctx, query, args, func(context.Context) error { return nil })
	}
	value, found, width, err := t.scalar(ctx, query, args...)
	if err != nil {
		return err
	}
	if err = oneColumnWidth(width); err != nil {
		return err
	}
	if !found {
		return sql.ErrNoRows
	}
	return Assign(dest, value)
}

// Column 把每一行的第一列写入 dest。
// dest 是单元格切片的指针。没有行时写成空切片。
func (d *DB) Column(ctx context.Context, dest any, query string, args ...any) error {
	if err := CheckColumn(dest); err != nil {
		return err
	}
	if d != nil && d.dryRun != nil && d.dryRun.Val() {
		return d.observe(ctx, query, args, func(context.Context) error { return nil })
	}
	values, _, err := d.column(ctx, query, args...)
	if err != nil {
		return err
	}
	return assignColumn(dest, values)
}

// Column 在事务内把每一行的第一列写入 dest。没有行时写成空切片。
func (t *Tx) Column(ctx context.Context, dest any, query string, args ...any) error {
	if err := CheckColumn(dest); err != nil {
		return err
	}
	if t != nil && t.db != nil && t.db.dryRun != nil && t.db.dryRun.Val() {
		return t.db.observe(ctx, query, args, func(context.Context) error { return nil })
	}
	values, _, err := t.column(ctx, query, args...)
	if err != nil {
		return err
	}
	return assignColumn(dest, values)
}

// ColumnExact 把恰好一列的每一行写入 dest。结果列数不是 1 时返回错误。没有行时写成空切片。
func (d *DB) ColumnExact(ctx context.Context, dest any, query string, args ...any) error {
	if err := CheckColumn(dest); err != nil {
		return err
	}
	if d != nil && d.dryRun != nil && d.dryRun.Val() {
		return d.observe(ctx, query, args, func(context.Context) error { return nil })
	}
	values, width, err := d.column(ctx, query, args...)
	if err != nil {
		return err
	}
	if err = oneColumnWidth(width); err != nil {
		return err
	}
	return assignColumn(dest, values)
}

// ColumnExact 在事务内把恰好一列的每一行写入 dest。约束与 DB.ColumnExact 相同。
func (t *Tx) ColumnExact(ctx context.Context, dest any, query string, args ...any) error {
	if err := CheckColumn(dest); err != nil {
		return err
	}
	if t != nil && t.db != nil && t.db.dryRun != nil && t.db.dryRun.Val() {
		return t.db.observe(ctx, query, args, func(context.Context) error { return nil })
	}
	values, width, err := t.column(ctx, query, args...)
	if err != nil {
		return err
	}
	if err = oneColumnWidth(width); err != nil {
		return err
	}
	return assignColumn(dest, values)
}

func oneColumnWidth(width int) error {
	if width == 1 {
		return nil
	}
	return fmt.Errorf("gooq: result has %d columns, expected 1", width)
}

func (d *DB) scalar(ctx context.Context, query string, args ...any) (any, bool, int, error) {
	var (
		value any
		found bool
		width int
	)
	err := d.observe(ctx, query, args, func(ctx context.Context) error {
		conn, err := d.executor(ctx, false)
		if err != nil {
			return err
		}
		value, found, width, err = readScalar(ctx, conn, d.driver, query, d.bindArgs(args)...)
		return err
	})
	return value, found, width, err
}

func (t *Tx) scalar(ctx context.Context, query string, args ...any) (any, bool, int, error) {
	var (
		value any
		found bool
		width int
	)
	err := t.db.observe(ctx, query, args, func(ctx context.Context) error {
		var err error
		value, found, width, err = readScalar(ctx, t.sql, t.db.driver, query, t.db.bindArgs(args)...)
		return err
	})
	return value, found, width, err
}

func (d *DB) column(ctx context.Context, query string, args ...any) ([]any, int, error) {
	var (
		values []any
		width  int
	)
	err := d.observe(ctx, query, args, func(ctx context.Context) error {
		conn, err := d.executor(ctx, false)
		if err != nil {
			return err
		}
		values, width, err = readColumn(ctx, conn, d.driver, query, d.bindArgs(args)...)
		return err
	})
	return values, width, err
}

func (t *Tx) column(ctx context.Context, query string, args ...any) ([]any, int, error) {
	var (
		values []any
		width  int
	)
	err := t.db.observe(ctx, query, args, func(ctx context.Context) error {
		var err error
		values, width, err = readColumn(ctx, t.sql, t.db.driver, query, t.db.bindArgs(args)...)
		return err
	})
	return values, width, err
}

func assignColumn(dest any, values []any) error {
	slice := reflect.ValueOf(dest).Elem()
	elemType := slice.Type().Elem()
	out := reflect.MakeSlice(slice.Type(), 0, len(values))
	for _, value := range values {
		elem := reflect.New(elemType).Elem()
		if err := Assign(elem.Addr().Interface(), value); err != nil {
			return err
		}
		out = reflect.Append(out, elem)
	}
	slice.Set(out)
	return nil
}
