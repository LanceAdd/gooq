package db

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"reflect"

	"github.com/lanceadd/gooq"
)

func readAll(ctx context.Context, query Queryer, driver Driver, statement string, args ...any) (result gooq.Result, err error) {
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
	dbTypes := columnTypeNames(rows)
	for rows.Next() {
		record, err := scanRecord(rows, columns, dbTypes, driver)
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

func readOne(ctx context.Context, query Queryer, driver Driver, statement string, args ...any) (gooq.Record, error) {
	result, err := readAll(ctx, query, driver, statement, args...)
	if err != nil || len(result) == 0 {
		return nil, err
	}
	return result[0], nil
}

func readValue(ctx context.Context, query Queryer, driver Driver, statement string, args ...any) (any, error) {
	value, _, err := readScalar(ctx, query, driver, statement, args...)
	return value, err
}

// readScalar 读取第一行的第一列。
// 查询没有行时 found 为 false。列为 NULL 时 found 为 true，值为 nil。
func readScalar(ctx context.Context, query Queryer, driver Driver, statement string, args ...any) (value any, found bool, err error) {
	rows, err := query.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, false, err
	}
	defer func() {
		err = closeRows(rows, err)
	}()
	if !rows.Next() {
		if err = rows.Err(); err != nil {
			return nil, false, err
		}
		return nil, false, nil
	}
	columns, err := rows.Columns()
	if err != nil {
		return nil, false, err
	}
	record, err := scanRecord(rows, columns, columnTypeNames(rows), driver)
	if err != nil {
		return nil, false, err
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	if len(columns) == 0 {
		return nil, true, nil
	}
	return record[columns[0]], true, nil
}

func scanDest(ctx context.Context, query Queryer, driver Driver, dest any, statement string, args ...any) error {
	if err := gooq.CheckScan(dest); err != nil {
		return err
	}
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("gooq: scan destination must be a non-nil pointer")
	}
	elem := rv.Elem()
	switch elem.Kind() {
	case reflect.Slice:
		if isStructSlice(elem.Type()) {
			result, err := readAll(ctx, query, driver, statement, args...)
			if err != nil {
				return err
			}
			if result == nil {
				result = gooq.Result{}
			}
			return result.Structs(dest)
		}
		return scanScalarSlice(ctx, query, driver, elem, statement, args...)
	case reflect.Pointer:
		if elem.Type().Elem().Kind() == reflect.Struct {
			result, err := readAll(ctx, query, driver, statement, args...)
			if err != nil {
				return err
			}
			if len(result) == 0 {
				return nil
			}
			return result[0].Struct(dest)
		}
	case reflect.Struct:
		result, err := readAll(ctx, query, driver, statement, args...)
		if err != nil {
			return err
		}
		if len(result) == 0 {
			return sql.ErrNoRows
		}
		return result[0].Struct(dest)
	default:
	}
	value, err := readValue(ctx, query, driver, statement, args...)
	if err != nil {
		return err
	}
	return gooqAssign(elem, value)
}

func scanScalarSlice(ctx context.Context, query Queryer, driver Driver, slice reflect.Value, statement string, args ...any) error {
	values, err := readColumn(ctx, query, driver, statement, args...)
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

func readColumn(ctx context.Context, query Queryer, driver Driver, statement string, args ...any) (values []any, err error) {
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
	dbTypes := columnTypeNames(rows)
	for rows.Next() {
		record, scanErr := scanRecord(rows, columns, dbTypes, driver)
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

func columnTypeNames(rows *sql.Rows) []string {
	types, err := rows.ColumnTypes()
	if err != nil || len(types) == 0 {
		return nil
	}
	names := make([]string, len(types))
	for i, typ := range types {
		if typ != nil {
			names[i] = typ.DatabaseTypeName()
		}
	}
	return names
}

func scanRecord(rows *sql.Rows, columns, dbTypes []string, driver Driver) (gooq.Record, error) {
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
		dbType := ""
		if i < len(dbTypes) {
			dbType = dbTypes[i]
		}
		record[column] = normalizeValue(driver, dbType, values[i])
	}
	return record, nil
}

func normalizeValue(driver Driver, dbType string, value any) any {
	if dec, ok := driver.(ValueDecoder); ok {
		decoded := dec.Decode(ColumnValue{DBType: dbType, Value: value})
		if kept, same := decoded.(ColumnValue); same {
			value = kept.Value
		} else {
			return decoded
		}
	}
	return normalize(value)
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
