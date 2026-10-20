package dsl

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/lanceadd/gooq"
)

func (b *DMLBuilder) rejectTenantWrite(rc *gooq.RenderContext, column string, value any, isNull bool) error {
	matches := b.matchingTenantFields(rc, column)
	if len(matches) == 0 {
		return nil
	}
	field := matches[0].field
	ids, ok := rc.TenantIDs()
	if !ok || len(ids) == 0 {
		return fmt.Errorf("gooq: tenant value is required for %q.%s", matches[0].tableName, field.ColumnName)
	}
	if len(ids) != 1 {
		return fmt.Errorf("gooq: tenant column %q cannot be set from multiple tenant values", field.ColumnName)
	}
	if isNull {
		return fmt.Errorf("gooq: tenant column %q cannot be set to NULL", field.ColumnName)
	}
	if _, isExpr := value.(gooq.Expression); isExpr {
		return fmt.Errorf("gooq: tenant column %q must be set to the current tenant value", field.ColumnName)
	}
	for _, match := range matches {
		if err := gooq.CheckTenantValue(match.field.LocalType, match.field.ColumnName, ids[0]); err != nil {
			return err
		}
		if err := gooq.CheckTenantValue(match.field.LocalType, match.field.ColumnName, value); err != nil {
			return err
		}
	}
	if !reflect.DeepEqual(value, ids[0]) {
		return fmt.Errorf("gooq: tenant column %q must equal the current tenant value", field.ColumnName)
	}
	return nil
}

func (b *DMLBuilder) rejectTenantSets(rc *gooq.RenderContext, sets []columnValue) error {
	seen := map[string]struct{}{}
	for _, cv := range sets {
		if column, ok := b.repeatedTenantColumn(rc, seen, cv.column); ok {
			return errTenantColumnRepeated(column)
		}
		if err := b.rejectTenantWrite(rc, cv.column, cv.value, cv.null); err != nil {
			return err
		}
	}
	return nil
}

func (b *DMLBuilder) requireSingleTenant(rc *gooq.RenderContext, field *gooq.FieldMeta) error {
	ids, ok := rc.TenantIDs()
	if !ok || len(ids) == 0 {
		return fmt.Errorf("gooq: tenant value is required for %q.%s", b.table.TableName(), field.ColumnName)
	}
	if len(ids) != 1 {
		return fmt.Errorf("gooq: insert into tenant table %q requires a single tenant value", b.table.TableName())
	}
	return nil
}

func (b *DMLBuilder) rejectTenantInsert(rc *gooq.RenderContext) error {
	if b.untenant || b.table == nil || b.table.Meta() == nil {
		return nil
	}
	field := b.table.Meta().TenantField()
	if field == nil {
		return nil
	}
	if err := b.requireSingleTenant(rc, field); err != nil {
		return err
	}
	ids, _ := rc.TenantIDs()
	for _, row := range b.insertRows {
		found := false
		for _, cv := range row {
			if !sameColumn(rc, field.ColumnName, cv.column) {
				continue
			}
			if found {
				return errTenantColumnRepeated(field.ColumnName)
			}
			found = true
			if cv.null {
				return fmt.Errorf("gooq: tenant column %q cannot be NULL", field.ColumnName)
			}
			if _, isExpr := cv.value.(gooq.Expression); isExpr {
				return fmt.Errorf("gooq: tenant column %q must be set to the current tenant value", field.ColumnName)
			}
			if err := gooq.CheckTenantValue(field.LocalType, field.ColumnName, cv.value); err != nil {
				return err
			}
			if !reflect.DeepEqual(cv.value, ids[0]) {
				return fmt.Errorf("gooq: tenant column %q must equal the current tenant value", field.ColumnName)
			}
		}
		if !found {
			return fmt.Errorf(
				"gooq: insert into tenant table %q must set tenant column %q; call UnTenant() to opt out",
				b.table.TableName(), field.ColumnName)
		}
	}
	return nil
}

func (b *DMLBuilder) rejectTenantInsertFrom(rc *gooq.RenderContext) error {
	if b.untenant || b.table == nil || b.table.Meta() == nil {
		return nil
	}
	field := b.table.Meta().TenantField()
	if field == nil {
		return nil
	}
	if err := b.requireSingleTenant(rc, field); err != nil {
		return err
	}
	if b.selectBuilder == nil || len(b.selectBuilder.fields) == 0 {
		return fmt.Errorf("gooq: insert into tenant table %q requires every select item to be a named column", b.table.TableName())
	}
	found := false
	for _, item := range b.selectBuilder.fields {
		named, ok := item.(gooq.ColumnNamer)
		if !ok {
			return fmt.Errorf("gooq: insert into tenant table %q requires every select item to be a named column", b.table.TableName())
		}
		if !sameColumn(rc, field.ColumnName, named.ColumnName()) {
			continue
		}
		if found {
			return errTenantColumnRepeated(field.ColumnName)
		}
		found = true
	}
	if !found {
		return fmt.Errorf(
			"gooq: insert into tenant table %q must set tenant column %q; call UnTenant() to opt out",
			b.table.TableName(), field.ColumnName)
	}
	return nil
}

// upsertConflictTargetDialect 表示该方言的 upsert 使用显式冲突目标。
type upsertConflictTargetDialect interface {
	UpsertConflictTarget() bool
}

func (b *DMLBuilder) rejectTenantUpsert(rc *gooq.RenderContext) error {
	if b.upsert == nil || b.untenant {
		return nil
	}
	var field *gooq.FieldMeta
	if b.table != nil && b.table.Meta() != nil {
		field = b.table.Meta().TenantField()
	}
	if field == nil {
		return nil
	}
	if rc != nil && rc.Spec() != nil {
		if target, ok := rc.Spec().(upsertConflictTargetDialect); ok && target.UpsertConflictTarget() {
			found := false
			for _, col := range b.upsert.conflictCols {
				if sameColumn(rc, field.ColumnName, col) {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf(
					"gooq: upsert into tenant table %q requires tenant column %q in OnConflictKey",
					b.table.TableName(), field.ColumnName)
			}
		}
	}
	if b.upsert.doNothing {
		return nil
	}
	seen := map[string]struct{}{}
	for column, value := range b.upsert.updateMap {
		if repeated, ok := b.repeatedTenantColumn(rc, seen, column); ok {
			return errTenantColumnRepeated(repeated)
		}
		if err := b.rejectTenantWrite(rc, column, value, false); err != nil {
			return err
		}
	}
	return nil
}

// foldIdent 表示这个方言把大小写不同的标识符当成同一列。
// 没实现的方言按精确字符串比较。PostgreSQL 和 GaussDB 的双引号标识符区分大小写。
// 自定义方言如果会折叠标识符，要实现 FoldIdent。
type foldIdent interface {
	FoldIdent() bool
}

func sameColumn(rc *gooq.RenderContext, metaName, column string) bool {
	if metaName == column {
		return true
	}
	if rc == nil || rc.Spec() == nil {
		return false
	}
	folder, ok := rc.Spec().(foldIdent)
	return ok && folder.FoldIdent() && strings.EqualFold(metaName, column)
}

type tenantFieldRef struct {
	tableName string
	field     *gooq.FieldMeta
}

// matchingTenantFields 返回这条语句里列名对得上的租户列。
// 主表和带 Meta 的 JOIN 表都算。派生表没有租户元数据，不参与。
func (b *DMLBuilder) matchingTenantFields(rc *gooq.RenderContext, column string) []tenantFieldRef {
	if b.untenant {
		return nil
	}
	var matched []tenantFieldRef
	for _, table := range b.statementTables() {
		meta := table.Meta()
		if meta == nil {
			continue
		}
		field := meta.TenantField()
		if field == nil || !sameColumn(rc, field.ColumnName, column) {
			continue
		}
		matched = append(matched, tenantFieldRef{tableName: table.TableName(), field: field})
	}
	return matched
}

func (b *DMLBuilder) statementTables() []gooq.Table {
	var tables []gooq.Table
	if b.table != nil {
		tables = append(tables, b.table)
	}
	for _, join := range b.joins {
		if join != nil && join.table != nil {
			tables = append(tables, join.table)
		}
	}
	return tables
}

// repeatedTenantColumn 按列名记录已经赋值的租户列。
// 折叠方言把 tenant_id 和 TENANT_ID 当成同一列。不同列名互不影响。
func (b *DMLBuilder) repeatedTenantColumn(rc *gooq.RenderContext, seen map[string]struct{}, column string) (string, bool) {
	matches := b.matchingTenantFields(rc, column)
	if len(matches) == 0 {
		return "", false
	}
	key := column
	if rc != nil && rc.Spec() != nil {
		if folder, ok := rc.Spec().(foldIdent); ok && folder.FoldIdent() {
			key = strings.ToLower(column)
		}
	}
	if _, ok := seen[key]; ok {
		return matches[0].field.ColumnName, true
	}
	seen[key] = struct{}{}
	return "", false
}

func errTenantColumnRepeated(column string) error {
	return fmt.Errorf("gooq: tenant column %q is assigned more than once", column)
}
