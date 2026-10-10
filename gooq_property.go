package gooq

// Property is one cell in a generated write record.
// The zero Property is unset and stays out of SQL.
type Property[T any] struct {
	v     T
	state ValueState
}

// Set marks a cell as assigned, including a zero value.
func Set[T any](v T) Property[T] {
	return Property[T]{v: v, state: ValueSet}
}

// Null marks a cell as SQL NULL.
func Null[T any]() Property[T] {
	return Property[T]{state: ValueNull}
}

// Unset returns a cell that stays out of SQL.
func Unset[T any]() Property[T] {
	return Property[T]{}
}

// Get returns the stored value. Unset and NULL cells return the zero value.
func (p Property[T]) Get() T {
	return p.v
}

// Value returns the assigned value. The boolean is false for unset and NULL cells.
func (p Property[T]) Value() (T, bool) {
	if p.state != ValueSet {
		var zero T
		return zero, false
	}
	return p.v, true
}

// IsUnset reports whether the cell stays out of SQL.
func (p Property[T]) IsUnset() bool {
	return p.state == ValueUnset
}

// IsSet reports whether the cell is assigned, including a zero value.
func (p Property[T]) IsSet() bool {
	return p.state == ValueSet
}

// IsNull reports whether the cell is written as SQL NULL.
func (p Property[T]) IsNull() bool {
	return p.state == ValueNull
}

// State reports whether the cell is unset, set, or NULL.
func (p Property[T]) State() ValueState {
	return p.state
}

// Assignment returns the column to write. Unset cells return false.
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
