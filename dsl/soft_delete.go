package dsl

import (
	"fmt"

	"github.com/lanceadd/gooq"
)

func errPageCache() error {
	return fmt.Errorf("gooq: PageCache only works with RowsAndCount, StructsAndCount, or ColumnAndCount")
}

// joinSoftDeleteInWhere 为 true 时，软删谓词写进 WHERE。外连接写进该 JOIN 的 ON。
func joinSoftDeleteInWhere(kind joinType) bool {
	switch kind {
	case joinInner, joinInnerLateral, joinCrossLateral:
		return true
	default:
		return false
	}
}

func boundBase(table gooq.Table) *gooq.TableBase {
	if table == nil {
		return nil
	}
	if base, ok := table.(*gooq.TableBase); ok {
		return base
	}
	binder, ok := table.(interface{ BoundBase() *gooq.TableBase })
	if !ok {
		return nil
	}
	return binder.BoundBase()
}

func (b *SelectBuilder) softReferenced(table *gooq.TableBase, column string) bool {
	if gooq.ReferencesColumn(b.conditions, table, column) {
		return true
	}
	for _, join := range b.joins {
		if gooq.ReferencesColumn(join.on, table, column) {
			return true
		}
	}
	return false
}

// softDeleteSQL 渲染一张实表的软删谓词。派生表、已显式接管的表返回空字符串。
// 不支持的列类型记入渲染错误，不生成 SQL。
func (b *SelectBuilder) softDeleteSQL(rc *gooq.RenderContext, table gooq.Table) string {
	if b.unscoped || table == nil || table.Meta() == nil {
		return ""
	}
	meta := table.Meta().SoftDeleteField()
	if meta == nil {
		return ""
	}
	base := boundBase(table)
	if base != nil && b.softReferenced(base, meta.ColumnName) {
		return ""
	}
	column := table.Field(meta.ColumnName).Render(rc)
	switch meta.LocalType {
	case gooq.LocalTypeDate, gooq.LocalTypeTime, gooq.LocalTypeDatetime:
		return column + " IS NULL"
	case gooq.LocalTypeInt, gooq.LocalTypeUint, gooq.LocalTypeInt32, gooq.LocalTypeUint32,
		gooq.LocalTypeInt64, gooq.LocalTypeUint64, gooq.LocalTypeBigInt, gooq.LocalTypeBool:
		return column + " = 0"
	default:
		rc.Fail(fmt.Errorf("gooq: soft delete column %q has unsupported type %q", meta.ColumnName, meta.LocalType))
		return ""
	}
}
