package gooq

import (
	"github.com/gogf/gf/v2/database/gdb"
)

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
