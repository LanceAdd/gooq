package gooq

type FieldMeta struct {
	ColumnName    string    // 列名。
	LocalType     LocalType // 本地类型标记（json/jsonb/切片数组等）。
	Primary       bool      // 是否主键。
	AutoIncrement bool      // 是否自增。
	SoftDelete    bool      // 是否软删除字段。
	Unique        bool      // 是否唯一索引列。
	Comment       string    // 列注释。
}

type TableMeta struct {
	TableName string      // 表名。
	Fields    []FieldMeta // 字段列表（含顺序）。
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
	Alias() string
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

func (t *TableBase) Alias() string {
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
