package gooq

import (
	"database/sql"
	"fmt"
	"reflect"

	"github.com/gogf/gf/v2/util/gconv"
)

// Record is one result row keyed by column name.
type Record map[string]any

// Result is an ordered list of rows.
type Result []Record

// Get reads one typed column from a row. A missing or NULL column returns the zero value.
func Get[T any](record Record, field Field[T]) T {
	var zero T
	if record == nil {
		return zero
	}
	value, ok := record[field.ColumnName()]
	if !ok || value == nil {
		return zero
	}
	var out T
	if err := gconv.Scan(value, &out); err != nil {
		return zero
	}
	return out
}

// Struct fills a *struct or **struct from this row.
// An empty row returns sql.ErrNoRows when the destination struct already exists,
// and leaves a nil **struct unchanged.
func (r Record) Struct(pointer any) error {
	if len(r) == 0 {
		rv := reflect.ValueOf(pointer)
		if rv.Kind() == reflect.Pointer && !rv.IsNil() && rv.Elem().Kind() == reflect.Pointer && rv.Elem().IsNil() {
			return nil
		}
		return sql.ErrNoRows
	}
	return gconv.Scan(map[string]any(r), pointer)
}

// Structs fills *[]T or *[]*T from these rows.
func (r Result) Structs(pointer any) error {
	rv := reflect.ValueOf(pointer)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("gooq: Structs destination must be a pointer to a slice")
	}
	return gconv.Structs([]Record(r), pointer)
}
