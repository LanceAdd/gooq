package db

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"reflect"

	"github.com/lanceadd/gooq"
)

func readAll(ctx context.Context, query queryer, statement string, args ...any) (result gooq.Result, err error) {
	rows, err := query.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = closeRows(rows, err)
	}()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		record, err := scanRecord(rows, columns)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func readOne(ctx context.Context, query queryer, statement string, args ...any) (gooq.Record, error) {
	result, err := readAll(ctx, query, statement, args...)
	if err != nil || len(result) == 0 {
		return nil, err
	}
	return result[0], nil
}

func readValue(ctx context.Context, query queryer, statement string, args ...any) (value any, err error) {
	rows, err := query.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = closeRows(rows, err)
	}()
	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	record, err := scanRecord(rows, columns)
	if err != nil {
		return nil, err
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(columns) == 0 {
		return nil, nil
	}
	return record[columns[0]], nil
}

func scanDest(ctx context.Context, query queryer, dest any, statement string, args ...any) error {
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("gooq: scan destination must be a non-nil pointer")
	}
	elem := rv.Elem()
	switch elem.Kind() {
	case reflect.Slice:
		if isStructSlice(elem.Type()) {
			result, err := readAll(ctx, query, statement, args...)
			if err != nil {
				return err
			}
			if result == nil {
				result = gooq.Result{}
			}
			return result.Structs(dest)
		}
		return scanScalarSlice(ctx, query, elem, statement, args...)
	case reflect.Pointer:
		if elem.Type().Elem().Kind() == reflect.Struct {
			result, err := readAll(ctx, query, statement, args...)
			if err != nil {
				return err
			}
			if len(result) == 0 {
				return nil
			}
			return result[0].Struct(dest)
		}
	case reflect.Struct:
		result, err := readAll(ctx, query, statement, args...)
		if err != nil {
			return err
		}
		if len(result) == 0 {
			return sql.ErrNoRows
		}
		return result[0].Struct(dest)
	default:
	}
	value, err := readValue(ctx, query, statement, args...)
	if err != nil {
		return err
	}
	return gooqAssign(elem, value)
}

func scanScalarSlice(ctx context.Context, query queryer, slice reflect.Value, statement string, args ...any) error {
	values, err := readColumn(ctx, query, statement, args...)
	if err != nil {
		return err
	}
	out := reflect.MakeSlice(slice.Type(), 0, len(values))
	elemType := slice.Type().Elem()
	for _, value := range values {
		elem := reflect.New(elemType).Elem()
		if err = gooqAssign(elem, value); err != nil {
			return err
		}
		out = reflect.Append(out, elem)
	}
	slice.Set(out)
	return nil
}

func readColumn(ctx context.Context, query queryer, statement string, args ...any) (values []any, err error) {
	rows, err := query.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = closeRows(rows, err)
	}()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		record, scanErr := scanRecord(rows, columns)
		if scanErr != nil {
			return nil, scanErr
		}
		if len(columns) == 0 {
			values = append(values, nil)
			continue
		}
		values = append(values, record[columns[0]])
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func closeRows(rows *sql.Rows, err error) error {
	closeErr := rows.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func isStructSlice(sliceType reflect.Type) bool {
	elem := sliceType.Elem()
	if elem.Kind() == reflect.Struct {
		return true
	}
	return elem.Kind() == reflect.Pointer && elem.Elem().Kind() == reflect.Struct
}

func scanRecord(rows *sql.Rows, columns []string) (gooq.Record, error) {
	values := make([]any, len(columns))
	targets := make([]any, len(columns))
	for i := range values {
		targets[i] = &values[i]
	}
	if err := rows.Scan(targets...); err != nil {
		return nil, err
	}
	record := make(gooq.Record, len(columns))
	for i, column := range columns {
		record[column] = normalize(values[i])
	}
	return record, nil
}

func normalize(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []byte:
		return string(typed)
	case sql.RawBytes:
		return string(typed)
	default:
		if reader, ok := value.(io.Reader); ok {
			payload, err := io.ReadAll(reader)
			if err == nil {
				return string(payload)
			}
		}
		return value
	}
}
