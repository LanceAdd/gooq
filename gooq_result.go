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

// Get reads one typed column from a row.
// A missing column, NULL, and a conversion failure all return the zero value.
// Value returns the conversion error instead.
func Get[T any](record Record, field Field[T]) T {
	value, err := Value[T](record, field)
	if err != nil {
		var zero T
		return zero
	}
	return value
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
	return gconv.StructTag(map[string]any(r), pointer, "orm")
}

// Structs fills *[]T or *[]*T from these rows.
func (r Result) Structs(pointer any) error {
	rv := reflect.ValueOf(pointer)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("gooq: Structs destination must be a pointer to a slice")
	}
	return gconv.StructsTag([]Record(r), pointer, "orm")
}
