// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件实现表元数据（TableMeta）与表对象基础，支撑类型化表对象的生成。
package gooq

import "strings"

// FieldMeta 是单个列的表元数据。
type FieldMeta struct {
	ColumnName    string    // 列名。
	LocalType     LocalType // 本地类型标记（json/jsonb/切片数组等）。
	Primary       bool      // 是否主键。
	AutoIncrement bool      // 是否自增。
	SoftDelete    bool      // 是否软删除字段。
	Unique        bool      // 是否唯一索引列。
	Comment       string    // 列注释。
}

// TableMeta 是表的完整元数据（生成时静态化，运行时零猜测）。
type TableMeta struct {
	TableName string      // 表名。
	Fields    []FieldMeta // 字段列表（含顺序）。
}

// FieldMetaOf 按列名查找字段元数据。
func (m *TableMeta) FieldMetaOf(columnName string) *FieldMeta {
	for i := range m.Fields {
		if m.Fields[i].ColumnName == columnName {
			return &m.Fields[i]
		}
	}
	return nil
}

// SoftDeleteField 返回软删除字段元数据（无则返回 nil）。
func (m *TableMeta) SoftDeleteField() *FieldMeta {
	for i := range m.Fields {
		if m.Fields[i].SoftDelete {
			return &m.Fields[i]
		}
	}
	return nil
}

// AllColumns 返回全部列名（含顺序）。
func (m *TableMeta) AllColumns() []string {
	columns := make([]string, len(m.Fields))
	for i, f := range m.Fields {
		columns[i] = f.ColumnName
	}
	return columns
}

// Table 是 DSL 接受的表对象接口：表名、别名、元数据与全列。
type Table interface {
	TableName() string
	Alias() string
	Meta() *TableMeta
	AllColumns() []string
}

// TableBase 是表对象的基础实现（表对象生成时嵌入）。
type TableBase struct {
	meta  *TableMeta
	alias string
}

// NewTableBase 创建表对象基础实现。
func NewTableBase(meta *TableMeta) *TableBase {
	return &TableBase{meta: meta}
}

// TableName 返回表名。
func (t *TableBase) TableName() string {
	return t.meta.TableName
}

// Alias 返回当前别名。
func (t *TableBase) Alias() string {
	return t.alias
}

// Meta 返回表元数据。
func (t *TableBase) Meta() *TableMeta {
	return t.meta
}

// AllColumns 返回全部列名。
func (t *TableBase) AllColumns() []string {
	return t.meta.AllColumns()
}

// As 返回指定别名的表副本（不修改原对象，可并发使用）。
func (t *TableBase) As(alias string) *TableBase {
	newT := *t
	newT.alias = alias
	return &newT
}

// Clone 返回无别名表副本。
func (t *TableBase) Clone() *TableBase {
	newT := *t
	return &newT
}

// AllFields 是全字段标记：FieldsEx 差集与全列展开使用。
type AllFields struct {
	columns []string
}

// NewAllFields 创建全字段标记。
func NewAllFields(columns []string) AllFields {
	return AllFields{columns: columns}
}

// Columns 返回全部列名。
func (a AllFields) Columns() []string {
	return a.columns
}

// Condition 实现 Expression 接口（渲染为逗号分隔的全列）。
func (a AllFields) Condition() (string, []any) {
	return strings.Join(a.columns, ", "), nil
}

func (a AllFields) render(rc *renderContext) (string, []any) {
	columns := make([]string, len(a.columns))
	for i, c := range a.columns {
		columns[i] = rc.quote(c)
	}
	return strings.Join(columns, ", "), nil
}
