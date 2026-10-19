package gooq

// ColumnNamer 是能给出列名的对象。Field 和生成模型里的字段都满足它。
type ColumnNamer interface {
	ColumnName() string
}
