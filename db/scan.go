package db

import (
	"context"
	"database/sql"
	"io"
)

func readAll(ctx context.Context, query Queryer, driver Driver, statement string, args ...any) (result Result, err error) {
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

func readOne(ctx context.Context, query Queryer, driver Driver, statement string, args ...any) (Record, error) {
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
	values, err := scanValues(rows, columns, columnTypeNames(rows), driver)
	if err != nil {
		return nil, false, err
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	if len(values) == 0 {
		return nil, true, nil
	}
	return values[0], true, nil
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
		scanned, scanErr := scanValues(rows, columns, dbTypes, driver)
		if scanErr != nil {
			return nil, scanErr
		}
		if len(scanned) == 0 {
			values = append(values, nil)
			continue
		}
		values = append(values, scanned[0])
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

func scanRecord(rows *sql.Rows, columns, dbTypes []string, driver Driver) (Record, error) {
	values, err := scanValues(rows, columns, dbTypes, driver)
	if err != nil {
		return nil, err
	}
	record := make(Record, len(columns))
	for i, column := range columns {
		record[column] = values[i]
	}
	return record, nil
}

func scanValues(rows *sql.Rows, columns, dbTypes []string, driver Driver) ([]any, error) {
	values := make([]any, len(columns))
	targets := make([]any, len(columns))
	for i := range values {
		targets[i] = &values[i]
	}
	if err := rows.Scan(targets...); err != nil {
		return nil, err
	}
	for i := range values {
		dbType := ""
		if i < len(dbTypes) {
			dbType = dbTypes[i]
		}
		values[i] = normalizeValue(driver, dbType, values[i])
	}
	return values, nil
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
