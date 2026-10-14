package gooq

// Property 是生成写入记录里的一个单元格。
// 零值 Property 是未设置，不会进入 SQL。
type Property[T any] struct {
	v     T
	state ValueState
}

// Set 把单元格标记为已设置，零值也会写入。
func Set[T any](v T) Property[T] {
	return Property[T]{v: v, state: ValueSet}
}

// Null 把单元格标记为 SQL NULL。
func Null[T any]() Property[T] {
	return Property[T]{state: ValueNull}
}

// Unset 返回不进入 SQL 的未设置单元格。
func Unset[T any]() Property[T] {
	return Property[T]{}
}

// Get 返回存的值；未设置和 NULL 的单元格返回零值。
func (p Property[T]) Get() T {
	return p.v
}

// Value 返回已设置的值；未设置和 NULL 的单元格第二个返回值为 false。
func (p Property[T]) Value() (T, bool) {
	if p.state != ValueSet {
		var zero T
		return zero, false
	}
	return p.v, true
}

// IsUnset 判断单元格是否不进 SQL。
func (p Property[T]) IsUnset() bool {
	return p.state == ValueUnset
}

// IsSet 判断单元格是否已设置，零值也算。
func (p Property[T]) IsSet() bool {
	return p.state == ValueSet
}

// IsNull 判断单元格是否写 SQL NULL。
func (p Property[T]) IsNull() bool {
	return p.state == ValueNull
}

// State 返回单元格是未设置、已设置还是 NULL。
func (p Property[T]) State() ValueState {
	return p.state
}

// Assignment 返回要写入的列；未设置的单元格返回 false。
func (p Property[T]) Assignment(column string) (Assignment, bool) {
	switch p.state {
	case ValueNull:
		return Assignment{Column: column, Null: true}, true
	case ValueSet:
		return Assignment{Column: column, Value: p.v}, true
	default:
		return Assignment{}, false
	}
}
