package gooq

// ValueState 记录生成模型的列是否被设置。
type ValueState uint8

const (
	// ValueUnset 表示列不进 SQL 语句。
	ValueUnset ValueState = iota
	// ValueSet 表示列会写入，零值也写。
	ValueSet
	// ValueNull 表示列写成 SQL NULL。
	ValueNull
)

// Assignment 是模型 setter 产生的一列。
type Assignment struct {
	// Column 是数据库列名。
	Column string
	// Value 在 Null 为 false 时写入。
	Value any
	// Null 写 SQL NULL，忽略 Value。
	Null bool
}

// ValueSource 是由 model.NewXxxRecord 的 setter 构造的一行。
type ValueSource interface {
	// Assignments 只返回已设置或设为 NULL 的列。
	Assignments() []Assignment
}
