package gooq

import (
	"github.com/gogf/gf/v2/database/gdb"
)

func Get[T any](record gdb.Record, field Field[T]) T {
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
