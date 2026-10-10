package gooq

// ValueState records whether a generated model column was assigned.
type ValueState uint8

const (
	// ValueUnset means the column stays out of the SQL statement.
	ValueUnset ValueState = iota
	// ValueSet means the column is written, including a zero value.
	ValueSet
	// ValueNull means the column is written as SQL NULL.
	ValueNull
)

// Assignment is one column produced by a model setter.
type Assignment struct {
	// Column is the database column name.
	Column string
	// Value is written when Null is false.
	Value any
	// Null writes SQL NULL and ignores Value.
	Null bool
}

// ValueSource is a row built by model.NewXxx setters.
type ValueSource interface {
	// Assignments returns only the columns that were set or set to NULL.
	Assignments() []Assignment
}
