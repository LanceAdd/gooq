package db

import (
	"database/sql"
	"fmt"
	"reflect"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/util/gconv"

	"github.com/lanceadd/gooq"
)

var (
	timeType    = reflect.TypeOf(time.Time{})
	scannerType = reflect.TypeOf((*sql.Scanner)(nil)).Elem()
)

// Assign 把一个驱动值写进非空指针。
// NULL 写零值，指针目标写成 nil。
// time.Time 按日期时间解析；sql.Scanner（包括 sql.Null*）直接收驱动值。
// 行结构体不是单元格，用 Record.Struct。
func Assign(pointer any, src any) error {
	rv := reflect.ValueOf(pointer)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("gooq: assign destination must be a non-nil pointer")
	}
	if !rv.Elem().CanSet() {
		return fmt.Errorf("gooq: destination cannot be set")
	}
	return assignValue(rv.Elem(), src)
}

// Value 从一行里读一个带类型的列。
// 缺列和 NULL 返回零值；转换失败返回错误。
func Value[T any](record Record, field gooq.Field[T]) (T, error) {
	var zero T
	if record == nil {
		return zero, nil
	}
	value, ok := record[field.ColumnName()]
	if !ok {
		return zero, nil
	}
	var out T
	if err := Assign(&out, value); err != nil {
		return zero, err
	}
	return out, nil
}

// ColumnOf 按结果顺序读出每一行的同一列。
func ColumnOf[T any](result Result, field gooq.Field[T]) ([]T, error) {
	out := make([]T, 0, len(result))
	for _, row := range result {
		value, err := Value[T](row, field)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

// Cell 返回某一列的动态视图。
// nil 记录、缺列和 NULL 都是 IsNil。
// Int64、Time 这类具名转换会吞掉解析错误，返回零值。
func Cell(record Record, column string) *gvar.Var {
	if record == nil {
		return gvar.New(nil)
	}
	value, ok := record[column]
	if !ok {
		return gvar.New(nil)
	}
	return gvar.New(value)
}

// CheckValue 报告 dest 不能接收单个单元格的情况。
func CheckValue(dest any) error {
	base, ok := storedType(dest)
	if !ok {
		return fmt.Errorf("gooq: Value destination must be a non-nil pointer")
	}
	if base.Kind() == reflect.Slice || base.Kind() == reflect.Array || base.Kind() == reflect.Map {
		return fmt.Errorf("gooq: Value reads one column; use Column for a slice")
	}
	elem := base
	for elem.Kind() == reflect.Pointer {
		elem = elem.Elem()
	}
	if elem.Kind() == reflect.Struct && !isCellStruct(elem) {
		return fmt.Errorf("gooq: Value reads one column; use Record.Struct for a row struct")
	}
	return nil
}

// CheckColumn 报告 dest 不能接收一列单元格的情况。
func CheckColumn(dest any) error {
	base, ok := storedType(dest)
	if !ok || base.Kind() != reflect.Slice {
		return fmt.Errorf("gooq: Column destination must be a pointer to a slice")
	}
	elem := base.Elem()
	for elem.Kind() == reflect.Pointer {
		elem = elem.Elem()
	}
	if elem.Kind() == reflect.Struct && !isCellStruct(elem) {
		return fmt.Errorf("gooq: Column reads one column; use Result.Structs for a row slice")
	}
	return nil
}

func storedType(dest any) (reflect.Type, bool) {
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return nil, false
	}
	return rv.Elem().Type(), true
}

func isCellStruct(t reflect.Type) bool {
	if t == timeType {
		return true
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	return t.Implements(scannerType) || reflect.PointerTo(t).Implements(scannerType)
}

func assignValue(dst reflect.Value, src any) error {
	if dst.Kind() == reflect.Pointer {
		if src == nil {
			dst.SetZero()
			return nil
		}
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		return assignValue(dst.Elem(), src)
	}
	if src == nil {
		if scanner, ok := scannerOf(dst); ok {
			return scanner.Scan(nil)
		}
		dst.SetZero()
		return nil
	}
	if dst.Type() == timeType {
		converted, err := toTime(src)
		if err != nil {
			return err
		}
		dst.Set(reflect.ValueOf(converted))
		return nil
	}
	if scanner, ok := scannerOf(dst); ok {
		return scanDriver(scanner, src)
	}
	if dst.Kind() == reflect.Interface {
		value := reflect.ValueOf(src)
		if !value.Type().AssignableTo(dst.Type()) {
			return fmt.Errorf("gooq: cannot assign %T to %s", src, dst.Type())
		}
		dst.Set(value)
		return nil
	}
	if !dst.CanAddr() {
		return fmt.Errorf("gooq: destination cannot be addressed")
	}
	return gconv.Scan(src, dst.Addr().Interface())
}

// scanDriver 把值交给 sql.Scanner。
// database/sql 允许字符串进 NullString，但 NullTime 只收 time.Time。
// 第一次 Scan 失败且值是日期时间字符串时，解析成 time.Time 再试一次。
func scanDriver(scanner sql.Scanner, src any) error {
	err := scanner.Scan(src)
	if err == nil {
		return nil
	}
	text, ok := driverText(src)
	if !ok || text == "" {
		return err
	}
	converted, convErr := timeFromString(text)
	if convErr != nil || converted.IsZero() {
		return err
	}
	if retryErr := scanner.Scan(converted); retryErr != nil {
		return err
	}
	return nil
}

func driverText(src any) (string, bool) {
	switch value := src.(type) {
	case string:
		return value, true
	case []byte:
		return string(value), true
	default:
		return "", false
	}
}

func scannerOf(dst reflect.Value) (sql.Scanner, bool) {
	if !dst.CanAddr() {
		return nil, false
	}
	if dst.Type().Implements(scannerType) {
		scanner, ok := dst.Interface().(sql.Scanner)
		return scanner, ok
	}
	pointer := dst.Addr()
	if pointer.Type().Implements(scannerType) {
		scanner, ok := pointer.Interface().(sql.Scanner)
		return scanner, ok
	}
	return nil, false
}

func toTime(src any) (time.Time, error) {
	switch value := src.(type) {
	case time.Time:
		return value, nil
	case *time.Time:
		if value == nil {
			return time.Time{}, nil
		}
		return *value, nil
	case string:
		return timeFromString(value)
	case []byte:
		return timeFromString(string(value))
	default:
		if unix, ok := unixSeconds(src); ok {
			return time.Unix(unix, 0), nil
		}
		return time.Time{}, fmt.Errorf("gooq: cannot convert %T to time.Time", src)
	}
}

func timeFromString(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	for _, layout := range timeLayouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	for _, layout := range localTimeLayouts {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("gooq: cannot parse time %q", value)
}

var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05Z07:00",
}

var localTimeLayouts = []string{
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02",
	"15:04:05",
}

func unixSeconds(src any) (int64, bool) {
	switch value := src.(type) {
	case int:
		return int64(value), true
	case int8:
		return int64(value), true
	case int16:
		return int64(value), true
	case int32:
		return int64(value), true
	case int64:
		return value, true
	case uint:
		return int64(value), true
	case uint8:
		return int64(value), true
	case uint16:
		return int64(value), true
	case uint32:
		return int64(value), true
	case uint64:
		return int64(value), true
	default:
		return 0, false
	}
}
