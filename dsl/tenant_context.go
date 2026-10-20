package dsl

import (
	"context"
	"fmt"
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
	if len(ids) > 0 {
		b.hasTenantIDs, b.tenantIDs = true, slices.Clone(ids)
	}
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
	if len(ids) > 0 {
		b.hasTenantIDs, b.tenantIDs = true, slices.Clone(ids)
	}
	return b
}

func (b *DMLBuilder) UnTenant() *DMLBuilder {
	b.untenant = true
	return b
}

func (b *DMLBuilder) stampTenant(ctx context.Context) {
	b.ctxTenant, b.hasCtxTenant = TenantFromContext(ctx)
}

func copyTenantIDs(ids []any) []any {
	if ids == nil {
		return nil
	}
	return slices.Clone(ids)
}

// seedTenant 把执行路径记在构建器上的 ctx 值放进新建的渲染上下文。
// 只记“有值”。这一层的 UnTenant 随后把整棵查询树标成禁用。
func seedTenant(rc *gooq.RenderContext, hasCtx bool, id any) {
	if hasCtx {
		rc.SetTenant([]any{id})
	}
}

// bindTenantLayer 按这一层的开关盖住外层。
// UnTenant 禁用整棵子树；有显式值就压入并重新打开；都没有则继承。
// ctx 只由顶层入口放进渲染上下文，子查询不读构建器上记下的值。
func bindTenantLayer(rc *gooq.RenderContext, untenant, hasIDs bool, ids []any) func() {
	if untenant {
		prev := rc.PushTenantDisabled()
		return func() { rc.PopTenant(prev) }
	}
	if !hasIDs {
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

func checkTenantJoins(rc *gooq.RenderContext, main gooq.Table, joins []*joinClause) {
	if rc.TenantDisabled() {
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

func tenantPredicate(rc *gooq.RenderContext, table gooq.Table) string {
	if rc.TenantDisabled() {
		return ""
	}
	if table == nil || table.Meta() == nil {
		return ""
	}
	meta := table.Meta().TenantField()
	if meta == nil {
		return ""
	}
	ids, ok := rc.TenantIDs()
	if !ok || len(ids) == 0 {
		rc.Fail(fmt.Errorf("gooq: tenant value is required for %q.%s", table.TableName(), meta.ColumnName))
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
