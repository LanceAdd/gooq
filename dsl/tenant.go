package dsl

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/lanceadd/gooq"
)

type tenantCtxKey struct{}

// WithTenant 把这次请求的租户值放进 ctx。列名不在 ctx 里。
func WithTenant(ctx context.Context, id any) context.Context {
	return context.WithValue(ctx, tenantCtxKey{}, id)
}

// TenantFromContext 读取 WithTenant 放进 ctx 的值。
func TenantFromContext(ctx context.Context) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	id := ctx.Value(tenantCtxKey{})
	if id == nil {
		return nil, false
	}
	return id, true
}

func (b *SelectBuilder) Tenant(ids ...any) *SelectBuilder {
	b.untenant = false
	b.hasTenantIDs, b.tenantIDs = tenantArgs(ids)
	return b
}

func (b *SelectBuilder) UnTenant() *SelectBuilder {
	b.untenant = true
	return b
}

func (b *SelectBuilder) stampTenant(ctx context.Context) {
	b.ctxTenant, b.hasCtxTenant = TenantFromContext(ctx)
}

func (b *DMLBuilder) Tenant(ids ...any) *DMLBuilder {
	b.untenant = false
	b.hasTenantIDs, b.tenantIDs = tenantArgs(ids)
	return b
}

func (b *DMLBuilder) UnTenant() *DMLBuilder {
	b.untenant = true
	return b
}

func (b *DMLBuilder) stampTenant(ctx context.Context) {
	b.ctxTenant, b.hasCtxTenant = TenantFromContext(ctx)
}

func tenantArgs(ids []any) (bool, []any) {
	if len(ids) == 0 {
		return false, nil
	}
	return true, slices.Clone(ids)
}

func copyTenantIDs(ids []any) []any {
	if ids == nil {
		return nil
	}
	return slices.Clone(ids)
}

// seedTenant 把执行路径记在构建器上的 ctx 值放进新建的渲染上下文。
// UnTenant 也放，子查询仍能看见这个值。
func seedTenant(rc *gooq.RenderContext, hasCtx bool, id any) {
	if hasCtx {
		rc.SetTenant([]any{id})
	}
}

// bindTenantLayer 在这一层有显式租户值时盖住外层。UnTenant 不改上下文。
// ctx 只由顶层入口放进渲染上下文，子查询不读构建器上记下的值。
func bindTenantLayer(rc *gooq.RenderContext, untenant, hasIDs bool, ids []any) func() {
	if untenant || !hasIDs {
		return func() {}
	}
	prev := rc.PushTenant(ids)
	return func() { rc.PopTenant(prev) }
}

func tenantMarked(table gooq.Table) bool {
	return table != nil && table.Meta() != nil && table.Meta().TenantField() != nil
}

func appendTenantName(names []string, table gooq.Table) []string {
	if !tenantMarked(table) {
		return names
	}
	name := table.TableName()
	for _, existing := range names {
		if existing == name {
			return names
		}
	}
	return append(names, name)
}

func checkTenantJoins(rc *gooq.RenderContext, untenant bool, main gooq.Table, joins []*joinClause) {
	if untenant {
		return
	}
	names := appendTenantName(nil, main)
	var blocked string
	for _, j := range joins {
		names = appendTenantName(names, j.table)
		if blocked == "" && (j.joinType == joinRight || j.joinType == joinFull) {
			blocked = joinKeyword[j.joinType]
		}
	}
	if len(names) == 0 || blocked == "" {
		return
	}
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = fmt.Sprintf("%q", name)
	}
	rc.Fail(fmt.Errorf(
		"gooq: %s is not supported in a query that touches tenant table %s; swap to LEFT JOIN or call UnTenant()",
		blocked, strings.Join(quoted, ", ")))
}

func (b *SelectBuilder) tenantSQL(rc *gooq.RenderContext, table gooq.Table) string {
	if b.untenant {
		return ""
	}
	return tenantPredicate(rc, table)
}

func (b *DMLBuilder) tenantSQL(rc *gooq.RenderContext, table gooq.Table) string {
	if b.untenant {
		return ""
	}
	return tenantPredicate(rc, table)
}

func tenantPredicate(rc *gooq.RenderContext, table gooq.Table) string {
	if table == nil || table.Meta() == nil {
		return ""
	}
	meta := table.Meta().TenantField()
	if meta == nil {
		return ""
	}
	ids, ok := rc.TenantIDs()
	if !ok || len(ids) == 0 {
		rc.Fail(errTenantRequired(table.TableName(), meta.ColumnName))
		return ""
	}
	for _, id := range ids {
		if err := gooq.CheckTenantValue(meta.LocalType, meta.ColumnName, id); err != nil {
			rc.Fail(err)
			return ""
		}
	}
	column := table.Field(meta.ColumnName).Render(rc)
	if len(ids) == 1 {
		return column + " = " + rc.AddArg(ids[0])
	}
	placeholders := make([]string, len(ids))
	for i, id := range ids {
		placeholders[i] = rc.AddArg(id)
	}
	return column + " IN (" + strings.Join(placeholders, ", ") + ")"
}

func (b *DMLBuilder) rejectTenantWrite(rc *gooq.RenderContext, column string, value any, isNull bool) error {
	matches := b.matchingTenantFields(rc, column)
	if len(matches) == 0 {
		return nil
	}
	field := matches[0].field
	ids, ok := rc.TenantIDs()
	if !ok || len(ids) == 0 {
		return errTenantRequired(matches[0].tableName, field.ColumnName)
	}
	if len(ids) != 1 {
		return fmt.Errorf("gooq: tenant column %q cannot be set from multiple tenant values", field.ColumnName)
	}
	if isNull {
		return fmt.Errorf("gooq: tenant column %q cannot be set to NULL", field.ColumnName)
	}
	if _, isExpr := value.(gooq.Expression); isExpr {
		return errTenantMustBeCurrent(field.ColumnName)
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
		return errTenantMustEqual(field.ColumnName)
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
		return errTenantRequired(b.table.TableName(), field.ColumnName)
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
				return errTenantMustBeCurrent(field.ColumnName)
			}
			if err := gooq.CheckTenantValue(field.LocalType, field.ColumnName, cv.value); err != nil {
				return err
			}
			if !reflect.DeepEqual(cv.value, ids[0]) {
				return errTenantMustEqual(field.ColumnName)
			}
		}
		if !found {
			return errTenantInsertMissing(b.table.TableName(), field.ColumnName)
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
		return errTenantInsertMissing(b.table.TableName(), field.ColumnName)
	}
	return nil
}

func (b *DMLBuilder) rejectTenantUpsert(rc *gooq.RenderContext) error {
	if b.upsert == nil || b.upsert.doNothing {
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

func errTenantRequired(tableName, column string) error {
	return fmt.Errorf("gooq: tenant value is required for %q.%s", tableName, column)
}

func errTenantMustBeCurrent(column string) error {
	return fmt.Errorf("gooq: tenant column %q must be set to the current tenant value", column)
}

func errTenantMustEqual(column string) error {
	return fmt.Errorf("gooq: tenant column %q must equal the current tenant value", column)
}

func errTenantInsertMissing(tableName, column string) error {
	return fmt.Errorf("gooq: insert into tenant table %q must set tenant column %q; call UnTenant() to opt out", tableName, column)
}

func errTenantColumnRepeated(column string) error {
	return fmt.Errorf("gooq: tenant column %q is assigned more than once", column)
}
