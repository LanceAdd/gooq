package db

import (
	"database/sql"
	"fmt"
	"reflect"

	"github.com/gogf/gf/v2/util/gconv"

	"github.com/lanceadd/gooq"
)

// Record 是一行结果，按列名索引。
type Record map[string]any

// Result 是保持顺序的行列表。
type Result []Record

// Get 从一行里读一个带类型的列。
// 缺列、NULL 和转换失败都返回零值，包括 Value 会返回的错误。
func Get[T any](record Record, field gooq.Field[T]) T {
	value, err := Value[T](record, field)
	if err != nil {
		var zero T
		return zero
	}
	return value
}

// Struct 把这行填进 *struct 或 **struct。
// 空行且目标结构体已存在时返回 sql.ErrNoRows；
// 目标是 nil 的 **struct 时保持不变。
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

// Structs 把这些行填进 *[]T 或 *[]*T。
func (r Result) Structs(pointer any) error {
	rv := reflect.ValueOf(pointer)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("gooq: Structs destination must be a pointer to a slice")
	}
	return gconv.StructsTag([]Record(r), pointer, "orm")
}
