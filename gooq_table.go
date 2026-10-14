package gooq

type FieldMeta struct {
	ColumnName    string
	LocalType     LocalType
	Primary       bool
	AutoIncrement bool
	SoftDelete    bool
	Unique        bool
	Comment       string
}

type TableMeta struct {
	TableName string
	Schema    string // 空则不渲染前缀。
	Fields    []FieldMeta
}

func (m *TableMeta) FieldMetaOf(columnName string) *FieldMeta {
	for i := range m.Fields {
		if m.Fields[i].ColumnName == columnName {
			return &m.Fields[i]
		}
	}
	return nil
}

func (m *TableMeta) SoftDeleteField() *FieldMeta {
	for i := range m.Fields {
		if m.Fields[i].SoftDelete {
			return &m.Fields[i]
		}
	}
	return nil
}

func (m *TableMeta) AllColumns() []string {
	columns := make([]string, len(m.Fields))
	for i, f := range m.Fields {
		columns[i] = f.ColumnName
	}
	return columns
}

type Table interface {
	TableName() string
	AliasName() string
	Meta() *TableMeta
	AllColumns() []string
	Field(column string) Field[any]
}

type TableBase struct {
	meta  *TableMeta
	alias string
}

func NewTableBase(meta *TableMeta) *TableBase {
	return &TableBase{meta: meta}
}

func (t *TableBase) TableName() string {
	return t.meta.TableName
}

func (t *TableBase) AliasName() string {
	return t.alias
}

func (t *TableBase) Meta() *TableMeta {
	return t.meta
}

func (t *TableBase) AllColumns() []string {
	return t.meta.AllColumns()
}

func (t *TableBase) As(alias string) *TableBase {
	newT := *t
	newT.alias = alias
	return &newT
}

func (t *TableBase) Clone() *TableBase {
	newT := *t
	return &newT
}

func (t *TableBase) AllFields() []Field[any] {
	fields := make([]Field[any], len(t.meta.Fields))
	for i, fm := range t.meta.Fields {
		fields[i] = t.Field(fm.ColumnName)
	}
	return fields
}

func (t *TableBase) Field(column string) Field[any] {
	return Field[any]{table: t, tableName: t.meta.TableName, columnName: column}
}

// BoundBase 返回这个表实例。嵌入 *TableBase 的生成表会提升该方法。
func (t *TableBase) BoundBase() *TableBase {
	return t
}
