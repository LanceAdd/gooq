package dsl

import (
	"context"
	"fmt"
	"reflect"
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
	return true, append([]any(nil), ids...)
}

func copyTenantIDs(ids []any) []any {
	if ids == nil {
		return nil
	}
	return append([]any(nil), ids...)
}

// seedTenant 把执行路径记在构建器上的 ctx 值放进新建的渲染上下文。
// UnTenant 也放，子查询仍能看见这个值。
func seedTenant(rc *gooq.RenderContext, hasCtx bool, id any) {
	if hasCtx {
		rc.SetTenant([]any{id})
	}
}

// bindTenantLayer 在这一层有显式租户值时盖住外层。UnTenant 不改上下文。
func bindTenantLayer(rc *gooq.RenderContext, untenant, hasIDs bool, ids []any, hasCtx bool, ctxID any) func() {
	if untenant {
		return func() {}
	}
	if hasIDs {
		prev := rc.PushTenant(ids)
		return func() { rc.PopTenant(prev) }
	}
	if rc.HasTenant() || !hasCtx {
		return func() {}
	}
	prev := rc.PushTenant([]any{ctxID})
	return func() { rc.PopTenant(prev) }
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

func (b *DMLBuilder) rejectTenantWrite(rc *gooq.RenderContext, column string, value any, isNull bool) error {
	if b.untenant || b.table == nil || b.table.Meta() == nil {
		return nil
	}
	field := b.table.Meta().TenantField()
	if field == nil || field.ColumnName != column {
		return nil
	}
	ids, ok := rc.TenantIDs()
	if !ok || len(ids) == 0 {
		return fmt.Errorf("gooq: tenant value is required for %q.%s", b.table.TableName(), field.ColumnName)
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
	if err := gooq.CheckTenantValue(field.LocalType, field.ColumnName, ids[0]); err != nil {
		return err
	}
	if err := gooq.CheckTenantValue(field.LocalType, field.ColumnName, value); err != nil {
		return err
	}
	if !reflect.DeepEqual(value, ids[0]) {
		return fmt.Errorf("gooq: tenant column %q must equal the current tenant value", field.ColumnName)
	}
	return nil
}

func (b *DMLBuilder) rejectTenantSets(rc *gooq.RenderContext, sets []columnValue) error {
	for _, cv := range sets {
		if err := b.rejectTenantWrite(rc, cv.column, cv.value, cv.null); err != nil {
			return err
		}
	}
	return nil
}

func (b *DMLBuilder) rejectTenantUpsert(rc *gooq.RenderContext) error {
	if b.upsert == nil || b.upsert.doNothing {
		return nil
	}
	for column, value := range b.upsert.updateMap {
		if err := b.rejectTenantWrite(rc, column, value, false); err != nil {
			return err
		}
	}
	return nil
}
