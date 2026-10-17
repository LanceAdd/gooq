package dsl

import (
	"fmt"
	"strings"

	"github.com/lanceadd/gooq"
)

// Render 渲染带括号的子查询。
// spec == nil 时，标识符用双引号，占位符用 ?，共享锁用 FOR SHARE。
// LIMIT 与 FOR UPDATE 保持现在的通用写法。调用方要得到方言错误，使用 ToSql。
func (b *SelectBuilder) Render(rc *gooq.RenderContext) string {
	sql := b.renderSelect(rc)
	sql = "(" + sql + ")"
	if b.alias != "" {
		sql += " AS " + rc.Quote(b.alias)
	}
	return sql
}

func (b *SelectBuilder) dialect() gooq.Dialect {
	if b.executor != nil {
		return autoDialect(b.executor)
	}
	return gooq.DialectMySQL
}

// ToSql 用指定方言渲染 SQL，不传方言时默认 MySQL。
func (b *SelectBuilder) ToSql(dialects ...gooq.Dialect) (string, []any, error) {
	if b.buildErr != nil {
		return "", nil, b.buildErr
	}
	var dialect = gooq.DialectMySQL
	if len(dialects) > 0 && dialects[0] != "" {
		dialect = dialects[0]
	}
	if _, err := gooq.DialectSpecOf(dialect); err != nil {
		return "", nil, err
	}
	if err := b.ValidateDialect(dialect); err != nil {
		return "", nil, err
	}
	rc := gooq.NewRenderContext(dialect)
	seedTenant(rc, b.hasCtxTenant, b.ctxTenant)
	sql := b.renderSelect(rc)
	if err := rc.Err(); err != nil {
		return "", nil, err
	}
	return sql, rc.Args(), nil
}

// ValidateDialect 遍历各位置表达式触发节点方言校验（含嵌套子查询）。
func (b *SelectBuilder) ValidateDialect(dialect gooq.Dialect) error {
	if b.groupExt != groupExtNone {
		spec, err := gooq.DialectSpecOf(dialect)
		if err != nil {
			return err
		}
		if err = spec.SupportsGroup(groupExtKeyword(b.groupExt)); err != nil {
			return err
		}
	}
	exprs := make([]gooq.Expression, 0, len(b.fields)+len(b.conditions)+len(b.groupBy)+len(b.having)+len(b.orderBy))
	exprs = append(exprs, b.fields...)
	exprs = append(exprs, b.conditions...)
	for _, j := range b.joins {
		exprs = append(exprs, j.on...)
	}
	exprs = append(exprs, b.groupBy...)
	exprs = append(exprs, b.having...)
	exprs = append(exprs, b.orderBy...)
	for _, e := range exprs {
		if err := gooq.WalkExpression(e, dialect); err != nil {
			return err
		}
	}
	return nil
}

func groupExtKeyword(k groupExtKind) string {
	switch k {
	case groupExtRollup:
		return gooq.GroupRollup
	case groupExtCube:
		return gooq.GroupCube
	case groupExtGroupingSets:
		return gooq.GroupGroupingSets
	default:
		return ""
	}
}

func (b *SelectBuilder) renderGroupExt(rc *gooq.RenderContext) string {
	switch b.groupExt {
	case groupExtRollup, groupExtCube:
		var flat []string
		for _, set := range b.groupExtSets {
			for _, f := range set {
				groupSQL := rc.Render(f)
				flat = append(flat, groupSQL)
			}
		}
		return groupExtKeyword(b.groupExt) + "(" + strings.Join(flat, ", ") + ")"
	case groupExtGroupingSets:
		var setParts []string
		for _, set := range b.groupExtSets {
			var fields []string
			for _, f := range set {
				groupSQL := rc.Render(f)
				fields = append(fields, groupSQL)
			}
			setParts = append(setParts, "("+strings.Join(fields, ", ")+")")
		}
		return "GROUPING SETS(" + strings.Join(setParts, ", ") + ")"
	default:
		return ""
	}
}

func joinKeywordSQL(rc *gooq.RenderContext, keyword string) string {
	if spec := rc.Spec(); spec != nil {
		keyword = spec.JoinKeyword(keyword)
	}
	if rc.Dialect() == gooq.DialectSQLite && strings.Contains(keyword, "LATERAL") {
		rc.Fail(fmt.Errorf("gooq: LATERAL join is not supported by dialect %s", gooq.DialectSQLite))
	}
	return keyword
}

func (b *SelectBuilder) renderSelect(rc *gooq.RenderContext) string {
	prev := rc.SwapQueryShard(b.hasShardValue, b.shardingValue)
	defer rc.RestoreQueryShard(prev)
	defer bindTenantLayer(rc, b.untenant, b.hasTenantIDs, b.tenantIDs)()
	checkTenantJoins(rc, b.untenant, b.from, b.joins)
	if base := gooq.TableBaseOf(b.from); base != nil {
		rc.SetMainTable(base)
	}
	var (
		sql     strings.Builder
		selects []string
	)
	if len(b.ctes) > 0 {
		var cteParts []string
		for _, cte := range b.ctes {
			cteSQL := cte.query.renderSelect(rc)
			cteParts = append(cteParts, rc.Quote(cte.name)+" AS ("+cteSQL+")")
		}
		sql.WriteString("WITH ")
		if b.recursive {
			sql.WriteString("RECURSIVE ")
		}
		sql.WriteString(strings.Join(cteParts, ", "))
		sql.WriteString(" ")
	}
	sql.WriteString("SELECT ")
	if b.distinct {
		sql.WriteString("DISTINCT ")
	}
	if len(b.fields) == 0 {
		sql.WriteString("*")
	} else {
		for _, f := range b.fields {
			selectSQL := rc.Render(f)
			if selectSQL != "" {
				selects = append(selects, selectSQL)
			}
		}
		sql.WriteString(strings.Join(selects, ", "))
	}
	sql.WriteString(" FROM ")
	sql.WriteString(b.renderTable(rc, b.from))
	for _, j := range b.joins {
		keyword := joinKeywordSQL(rc, joinKeyword[j.joinType])
		sql.WriteString(" ")
		sql.WriteString(keyword)
		sql.WriteString(" ")
		sql.WriteString(b.renderTable(rc, j.table))
		var ons []string
		for _, c := range j.on {
			onSQL := rc.Render(c)
			if onSQL != "" {
				ons = append(ons, parenthesizeSQL(onSQL))
			}
		}
		if !joinSoftDeleteInWhere(j.joinType) {
			if soft := b.softDeleteSQL(rc, j.table); soft != "" {
				ons = append(ons, soft)
			}
			if tenant := b.tenantSQL(rc, j.table); tenant != "" {
				ons = append(ons, tenant)
			}
		}
		if len(ons) > 0 {
			sql.WriteString(" ON ")
			sql.WriteString(strings.Join(ons, " AND "))
		}
	}
	if where := b.renderWhere(rc); where != "" {
		sql.WriteString(" WHERE ")
		sql.WriteString(where)
	}
	if len(b.groupBy) > 0 || b.groupExt != groupExtNone {
		var groups []string
		for _, g := range b.groupBy {
			groupSQL := rc.Render(g)
			groups = append(groups, groupSQL)
		}
		if b.groupExt == groupExtRollup && rc.Spec() != nil && rc.Spec().RollupWith() {
			for _, set := range b.groupExtSets {
				for _, f := range set {
					groupSQL := rc.Render(f)
					groups = append(groups, groupSQL)
				}
			}
			sql.WriteString(" GROUP BY ")
			sql.WriteString(strings.Join(groups, ", "))
			sql.WriteString(" WITH ROLLUP")
		} else {
			// 其余方言括号语法：ROLLUP(a,b) / CUBE(a,b) / GROUPING SETS((a),(b))。
			if b.groupExt != groupExtNone {
				groups = append(groups, b.renderGroupExt(rc))
			}
			sql.WriteString(" GROUP BY ")
			sql.WriteString(strings.Join(groups, ", "))
		}
	}
	if len(b.having) > 0 {
		var haves []string
		for _, h := range b.having {
			havingSQL := rc.Render(h)
			if havingSQL != "" {
				haves = append(haves, havingSQL)
			}
		}
		if len(haves) > 0 {
			sql.WriteString(" HAVING ")
			sql.WriteString(strings.Join(haves, " AND "))
		}
	}
	// 集合操作：整体 ORDER BY / LIMIT 作用于组合结果。
	for _, so := range b.setOps {
		subSQL := so.query.renderSelect(rc)
		sql.WriteString(" ")
		sql.WriteString(setOpKeyword[so.op])
		sql.WriteString(" ")
		sql.WriteString(subSQL)
	}
	if len(b.orderBy) > 0 {
		var orders []string
		for _, o := range b.orderBy {
			orderSQL := rc.Render(o)
			orders = append(orders, orderSQL)
		}
		sql.WriteString(" ORDER BY ")
		sql.WriteString(strings.Join(orders, ", "))
	}
	if b.limit > 0 || b.offset > 0 {
		if spec := rc.Spec(); spec != nil {
			sql.WriteString(spec.RenderLimit(rc, b.limit, b.offset))
		} else if b.limit > 0 {
			sql.WriteString(fmt.Sprintf(" LIMIT %d", b.limit))
			if b.offset > 0 {
				sql.WriteString(fmt.Sprintf(" OFFSET %d", b.offset))
			}
		} else {
			sql.WriteString(fmt.Sprintf(" OFFSET %d", b.offset))
		}
	}
	if b.lock != lockNone && (rc.Spec() == nil || !rc.Spec().OmitLock()) {
		switch b.lock {
		case lockForUpdate:
			sql.WriteString(" FOR UPDATE")
		case lockInShareMode:
			sql.WriteString(" " + rc.ShareLockKeyword())
		default:
		}
	}
	return sql.String()
}

func (b *SelectBuilder) renderTable(rc *gooq.RenderContext, t gooq.Table) string {
	return renderTableName(rc, t)
}

// renderTableName 渲染表或派生表（表名 + 别名）；SelectBuilder 与 DMLBuilder 共用。
func renderTableName(rc *gooq.RenderContext, t gooq.Table) string {
	if sub, ok := t.(*SelectBuilder); ok {
		subSQL := sub.renderSelect(rc)
		sql := "(" + subSQL + ")"
		if sub.alias != "" {
			sql += " AS " + rc.Quote(sub.alias)
		}
		return sql
	}
	var sql = tableNameSQL(rc, t)
	if t.AliasName() != "" {
		sql += " AS " + rc.Quote(t.AliasName())
	}
	return sql
}

// tableNameSQL 渲染限定表名（schema.table）；分片表先按分片值解析物理名。
func tableNameSQL(rc *gooq.RenderContext, t gooq.Table) string {
	var (
		tableName = t.TableName()
		schema    string
	)
	if m := t.Meta(); m != nil {
		tableName = gooq.ResolveTableName(rc, t, tableName)
		schema = m.Schema
	}
	name := rc.Quote(tableName)
	if schema != "" {
		name = rc.Quote(schema) + "." + name
	}
	return name
}

// renderWhere 渲染 WHERE 条件（含 FROM 与内连接的软删除自动条件）。
func (b *SelectBuilder) renderWhere(rc *gooq.RenderContext) string {
	var parts []string
	for _, c := range b.conditions {
		condSQL := rc.Render(c)
		if condSQL != "" {
			parts = append(parts, parenthesizeSQL(condSQL))
		}
	}
	if soft := b.softDeleteSQL(rc, b.from); soft != "" {
		parts = append(parts, soft)
	}
	if tenant := b.tenantSQL(rc, b.from); tenant != "" {
		parts = append(parts, tenant)
	}
	for _, j := range b.joins {
		if !joinSoftDeleteInWhere(j.joinType) {
			continue
		}
		if soft := b.softDeleteSQL(rc, j.table); soft != "" {
			parts = append(parts, soft)
		}
		if tenant := b.tenantSQL(rc, j.table); tenant != "" {
			parts = append(parts, tenant)
		}
	}
	return strings.Join(parts, " AND ")
}
