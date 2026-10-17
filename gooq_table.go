package gooq

type FieldMeta struct {
	ColumnName    string
	LocalType     LocalType
	Primary       bool
	AutoIncrement bool
	SoftDelete    bool
	Tenant        bool
	Unique        bool
	Comment       string
}

type TableMeta struct {
	TableName  string
	Schema     string       // 空则不渲染前缀。
	Sharding   ShardingRule // nil 表示不分片。
	ShardNames []string     // 生成时库里的物理分表名。普通表为空。
	Fields     []FieldMeta
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

func (m *TableMeta) TenantField() *FieldMeta {
	if m == nil {
		return nil
	}
	for i := range m.Fields {
		if m.Fields[i].Tenant {
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
	meta          *TableMeta
	alias         string
	shardValue    any
	hasShardValue bool
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

// SetShard 在这个表对象上记下分片值。nil 也算设置过。
func (t *TableBase) SetShard(value any) {
	t.shardValue = value
	t.hasShardValue = true
}

// CopyShard 把这个对象上的分片值复制到 dst。没调用过 ShardBy 时 dst 也保持未设置。
func (t *TableBase) CopyShard(dst *TableBase) {
	if t == nil || dst == nil {
		return
	}
	dst.shardValue = t.shardValue
	dst.hasShardValue = t.hasShardValue
}

// TableBaseOf 返回表对象底层的 *TableBase。没有嵌入 TableBase 时返回 nil。
func TableBaseOf(table Table) *TableBase {
	if table == nil {
		return nil
	}
	carrier, ok := table.(interface{ BoundBase() *TableBase })
	if !ok {
		return nil
	}
	return carrier.BoundBase()
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
