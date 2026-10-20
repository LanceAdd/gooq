package dsl

import (
	"fmt"
	"slices"
	"strings"

	"github.com/lanceadd/gooq"
)

func (b *DMLBuilder) installShard(rc *gooq.RenderContext) {
	rc.SwapQueryShard(b.hasShardValue, b.shardingValue)
	rc.SetMainTable(gooq.TableBaseOf(b.table))
}

func (b *DMLBuilder) installTenant(rc *gooq.RenderContext) {
	seedTenant(rc, b.hasCtxTenant, b.ctxTenant)
	if b.untenant {
		rc.PushTenantDisabled()
		return
	}
	if !b.hasTenantIDs {
		return
	}
	rc.PushTenant(b.tenantIDs)
}

// ToSql 用指定方言渲染 SQL，不传方言时默认 MySQL。
func (b *DMLBuilder) ToSql(dialects ...gooq.Dialect) (string, []any, error) {
	if b.err != nil {
		return "", nil, b.err
	}
	var dialect = gooq.DialectMySQL
	if len(dialects) > 0 && dialects[0] != "" {
		dialect = dialects[0]
	}
	if _, err := gooq.DialectSpecOf(dialect); err != nil {
		return "", nil, err
	}
	if (b.kind == dmlUpdate || b.kind == dmlDelete) && len(b.batchUpdateRows) > 0 {
		return "", nil, fmt.Errorf("gooq: batch delete/update renders multiple SQL, use Exec")
	}
	rc := gooq.NewRenderContext(dialect)
	b.installShard(rc)
	b.installTenant(rc)
	var (
		sql  string
		args []any
		err  error
	)
	switch b.kind {
	case dmlInsert:
		sql, args, err = b.renderInsert(rc)
	case dmlInsertFrom:
		sql, args, err = b.renderInsertFrom(rc)
	case dmlUpdate:
		sql, args, err = b.renderUpdate(rc)
	case dmlDelete:
		sql, args, err = b.renderDelete(rc)
	default:
		return "", nil, fmt.Errorf("gooq: unknown dml kind %d", b.kind)
	}
	if err != nil {
		return "", nil, err
	}
	if err = rc.Err(); err != nil {
		return "", nil, err
	}
	return sql, args, nil
}

func (b *DMLBuilder) renderInsertFrom(rc *gooq.RenderContext) (string, []any, error) {
	if err := b.rejectTenantInsertFrom(rc); err != nil {
		return "", nil, err
	}
	// 列清单：优先子查询字段的列名，否则表全列。
	var columns []string
	if b.selectBuilder != nil && len(b.selectBuilder.fields) > 0 {
		for _, f := range b.selectBuilder.fields {
			if field, ok := f.(gooq.ColumnNamer); ok {
				columns = append(columns, field.ColumnName())
			}
		}
	}
	if len(columns) == 0 {
		columns = b.table.AllColumns()
	}
	if len(columns) == 0 {
		return "", nil, fmt.Errorf("gooq: insert-from columns is empty")
	}
	subSQL := b.selectBuilder.renderSelect(rc)
	var sqlStr = fmt.Sprintf(
		"INSERT INTO %s (%s) %s",
		tableNameSQL(rc, b.table),
		strings.Join(quoteColumns(rc, columns), ", "),
		subSQL,
	)
	if returning, err := b.renderReturning(rc); err != nil {
		return "", nil, err
	} else if returning != "" {
		sqlStr += returning
	}
	return sqlStr, rc.Args(), nil
}

func (b *DMLBuilder) renderInsert(rc *gooq.RenderContext) (string, []any, error) {
	if err := b.rejectTenantInsert(rc); err != nil {
		return "", nil, err
	}
	if err := b.rejectTenantUpsert(rc); err != nil {
		return "", nil, err
	}
	if len(b.insertRows) == 0 {
		return "", nil, fmt.Errorf("gooq: insert data is empty")
	}
	// 一条 SQL 只能表达同一列集合。缺列由 Exec 拆成多条 INSERT，不在这里补 NULL。
	if len(groupInsertRows(b.insertRows)) > 1 {
		return "", nil, fmt.Errorf("gooq: mixed insert columns render multiple SQL, use Exec")
	}
	// 列并集：按首行顺序，后续行的额外列追加。
	var columns []string
	seen := make(map[string]bool)
	for _, row := range b.insertRows {
		for _, cv := range row {
			if !seen[cv.column] {
				seen[cv.column] = true
				columns = append(columns, cv.column)
			}
		}
	}
	if len(columns) == 0 {
		return "", nil, fmt.Errorf("gooq: insert columns is empty")
	}
	rows := make([][]string, len(b.insertRows))
	for i, row := range b.insertRows {
		placeholders := make([]string, len(columns))
		for j, col := range columns {
			placeholders[j] = placeValue(rc, row, col)
		}
		rows[i] = placeholders
	}
	spec := rc.Spec()
	if b.upsert != nil && spec != nil {
		if renderer, ok := spec.(gooq.InsertRenderer); ok {
			sqlStr, err := renderer.RenderInsert(rc, gooq.InsertStatement{
				Table:   tableNameSQL(rc, b.table),
				Columns: columns,
				Rows:    rows,
				Upsert: &gooq.UpsertClause{
					DoNothing:    b.upsert.doNothing,
					ConflictCols: b.upsert.conflictCols,
					Updates:      b.upsert.updateMap,
				},
			})
			if err != nil {
				return "", nil, err
			}
			if returning, retErr := b.renderReturning(rc); retErr != nil {
				return "", nil, retErr
			} else if returning != "" {
				sqlStr += returning
			}
			return sqlStr, rc.Args(), nil
		}
	}
	joined := make([]string, len(rows))
	for i, row := range rows {
		joined[i] = "(" + strings.Join(row, ", ") + ")"
	}
	doNothing := b.upsert != nil && b.upsert.doNothing
	insertKeyword := "INSERT"
	if spec != nil {
		insertKeyword = spec.InsertKeyword(doNothing)
	}
	var sqlStr = fmt.Sprintf(
		"%s INTO %s (%s) VALUES %s",
		insertKeyword,
		tableNameSQL(rc, b.table),
		strings.Join(quoteColumns(rc, columns), ", "),
		strings.Join(joined, ", "),
	)
	if b.upsert != nil && spec != nil {
		upsertSQL, err := spec.RenderUpsert(rc, gooq.UpsertClause{
			DoNothing:    b.upsert.doNothing,
			ConflictCols: b.upsert.conflictCols,
			Updates:      b.upsert.updateMap,
		})
		if err != nil {
			return "", nil, err
		}
		sqlStr += upsertSQL
	}
	if returning, err := b.renderReturning(rc); err != nil {
		return "", nil, err
	} else if returning != "" {
		sqlStr += returning
	}
	return sqlStr, rc.Args(), nil
}

func quoteColumns(rc *gooq.RenderContext, columns []string) []string {
	result := make([]string, len(columns))
	for i, c := range columns {
		result[i] = rc.Quote(c)
	}
	return result
}

func placeValue(rc *gooq.RenderContext, row []columnValue, column string) string {
	for _, cv := range row {
		if cv.column != column {
			continue
		}
		if cv.null {
			return "NULL"
		}
		if expr, ok := cv.value.(gooq.Expression); ok {
			valSQL := rc.Render(expr)
			return valSQL
		}
		return rc.AddArg(cv.value)
	}
	return "NULL"
}

func (b *DMLBuilder) renderUpdate(rc *gooq.RenderContext) (string, []any, error) {
	if len(b.setValues) == 0 {
		return "", nil, fmt.Errorf("gooq: update data is empty")
	}
	if err := b.rejectTenantSets(rc, b.setValues); err != nil {
		return "", nil, err
	}
	return b.renderUpdateStatement(rc, func() string { return b.renderSets(rc) })
}

// updateJoinsBeforeSet 表示带 JOIN 的 UPDATE 把连接写在 SET 前面。
// MySQL 家族实现它。位置参数要排成 JOIN、SET、WHERE。
type updateJoinsBeforeSet interface {
	UpdateJoinsBeforeSet() bool
}

func joinsBeforeSet(spec gooq.DialectSpec) bool {
	order, ok := spec.(updateJoinsBeforeSet)
	return ok && order.UpdateJoinsBeforeSet()
}

// renderUpdateStatement 按 SET、JOIN、WHERE 收集参数。
// MySQL 家族的 SQL 把 JOIN 写在 SET 前面，参数在 RETURNING 之前改成同样的顺序。
// 每段参数单独拷贝。它们来自同一条参数切片，直接拼接会被后续 append 盖掉。
func (b *DMLBuilder) renderUpdateStatement(rc *gooq.RenderContext, renderSets func() string) (string, []any, error) {
	spec := rc.Spec()
	if spec == nil {
		return "", nil, fmt.Errorf("gooq: dialect %q is not registered", rc.Dialect())
	}
	target := renderTableName(rc, b.table)
	setMark := len(rc.Args())
	sets := renderSets()
	setArgs := slices.Clone(rc.Args()[setMark:])
	joinMark := len(rc.Args())
	joins := b.joinRenders(rc)
	joinArgs := slices.Clone(rc.Args()[joinMark:])
	whereMark := len(rc.Args())
	where := b.renderWhere(rc)
	whereArgs := slices.Clone(rc.Args()[whereMark:])
	sqlStr, err := spec.RenderUpdate(rc, target, sets, where, joins)
	if err != nil {
		return "", nil, err
	}
	if len(joins) > 0 && joinsBeforeSet(spec) {
		ordered := make([]any, 0, len(rc.Args()))
		ordered = append(ordered, rc.Args()[:setMark]...)
		ordered = append(ordered, joinArgs...)
		ordered = append(ordered, setArgs...)
		ordered = append(ordered, whereArgs...)
		rc.SetArgs(ordered)
	}
	returning, err := b.renderReturning(rc)
	if err != nil {
		return "", nil, err
	}
	return sqlStr + returning, rc.Args(), nil
}

func (b *DMLBuilder) validateBatchUpdateKeys(keys []string) error {
	for index, row := range b.batchUpdateRows {
		present := make(map[string]columnValue, len(keys))
		for _, cv := range row {
			if slices.Contains(keys, cv.column) {
				present[cv.column] = cv
			}
		}
		record := index + 1
		if len(present) == 0 {
			return fmt.Errorf("gooq: batch update record %d has no key columns", record)
		}
		var missing []string
		for _, key := range keys {
			cv, ok := present[key]
			if !ok {
				missing = append(missing, key)
				continue
			}
			if cv.null {
				return fmt.Errorf("gooq: batch update record %d key %s is NULL", record, key)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("gooq: batch update record %d missing key columns: %s", record, strings.Join(missing, ", "))
		}
	}
	return nil
}

func (b *DMLBuilder) renderBatchDML(dialect gooq.Dialect) ([]string, [][]any, error) {
	if len(b.joins) > 0 {
		return nil, nil, fmt.Errorf("gooq: batch delete/update does not support JOIN")
	}
	keys := b.updateKeys
	if len(keys) == 0 && b.table.Meta() != nil {
		for _, fm := range b.table.Meta().Fields {
			if fm.Primary {
				keys = append(keys, fm.ColumnName)
			}
		}
	}
	if len(keys) == 0 {
		return nil, nil, fmt.Errorf("gooq: batch update requires primary key or Keys(...)")
	}
	if err := b.validateBatchUpdateKeys(keys); err != nil {
		return nil, nil, err
	}
	var (
		sqls  []string
		argss [][]any
	)
	for _, row := range b.batchUpdateRows {
		var (
			sets  []columnValue
			where []columnValue
		)
		for _, cv := range row {
			if slices.Contains(keys, cv.column) {
				where = append(where, cv)
			} else {
				sets = append(sets, cv)
			}
		}
		if b.kind == dmlUpdate && len(sets) == 0 {
			continue
		}
		rc := gooq.NewRenderContext(dialect)
		b.installShard(rc)
		b.installTenant(rc)
		if err := b.rejectTenantSets(rc, sets); err != nil {
			return nil, nil, err
		}
		var sqlStr string
		switch b.kind {
		case dmlDelete:
			var softField *gooq.FieldMeta
			if b.table.Meta() != nil {
				softField = b.table.Meta().SoftDeleteField()
			}
			if softField != nil && !b.unscoped {
				value, ok := softDeleteValue(softField.LocalType)
				if !ok {
					return nil, nil, fmt.Errorf("gooq: soft delete column %q has unsupported type %q", softField.ColumnName, softField.LocalType)
				}
				sqlStr = fmt.Sprintf(
					"UPDATE %s SET %s = %s",
					tableNameSQL(rc, b.table),
					rc.Quote(softField.ColumnName),
					rc.AddArg(value),
				)
			} else {
				sqlStr = "DELETE FROM " + tableNameSQL(rc, b.table)
			}
		default:
			placeholders := make([]string, len(sets))
			for i := range sets {
				placeholders[i] = renderSetValue(rc, sets[i])
			}
			sqlStr = fmt.Sprintf("UPDATE %s SET %s", tableNameSQL(rc, b.table), strings.Join(placeholders, ", "))
		}
		whereParts := make([]string, len(where))
		for i, cv := range where {
			whereParts[i] = fmt.Sprintf("%s = %s", rc.Quote(cv.column), rc.AddArg(cv.value))
		}
		if extra := b.renderWhere(rc); extra != "" {
			whereParts = append(whereParts, extra)
		}
		if err := rc.Err(); err != nil {
			return nil, nil, err
		}
		sqls = append(sqls, sqlStr+" WHERE "+strings.Join(whereParts, " AND "))
		argss = append(argss, rc.Args())
	}
	return sqls, argss, nil
}

func (b *DMLBuilder) renderDelete(rc *gooq.RenderContext) (string, []any, error) {
	if !b.unscoped && b.table.Meta() != nil {
		if softField := b.table.Meta().SoftDeleteField(); softField != nil {
			return b.renderSoftDelete(rc, softField)
		}
	}
	spec := rc.Spec()
	if spec == nil {
		return "", nil, fmt.Errorf("gooq: dialect %q is not registered", rc.Dialect())
	}
	sqlStr, err := spec.RenderDelete(rc, renderTableName(rc, b.table), b.joinRenders(rc), b.renderWhere(rc))
	if err != nil {
		return "", nil, err
	}
	if returning, err := b.renderReturning(rc); err != nil {
		return "", nil, err
	} else if returning != "" {
		sqlStr += returning
	}
	return sqlStr, rc.Args(), nil
}

// renderSoftDelete 把软删 DELETE 改写成 UPDATE deleted_at = now()。
// 赋值在局部构造，渲染期间不写 setValues，也不追加 IS NULL 守卫。
func (b *DMLBuilder) renderSoftDelete(rc *gooq.RenderContext, softField *gooq.FieldMeta) (string, []any, error) {
	value, ok := softDeleteValue(softField.LocalType)
	if !ok {
		return "", nil, fmt.Errorf("gooq: soft delete column %q has unsupported type %q", softField.ColumnName, softField.LocalType)
	}
	return b.renderUpdateStatement(rc, func() string {
		return renderSetValue(rc, columnValue{column: softField.ColumnName, value: value})
	})
}

func (b *DMLBuilder) renderWhere(rc *gooq.RenderContext) string {
	checkTenantJoins(rc, b.table, b.joins)
	var parts []string
	for _, c := range b.conditions {
		condSQL := rc.Render(c)
		if condSQL != "" {
			parts = append(parts, parenthesizeSQL(condSQL))
		}
	}
	if tenant := tenantPredicate(rc, b.table); tenant != "" {
		parts = append(parts, tenant)
	}
	for _, j := range b.joins {
		if !joinSoftDeleteInWhere(j.joinType) {
			continue
		}
		if tenant := tenantPredicate(rc, j.table); tenant != "" {
			parts = append(parts, tenant)
		}
	}
	return strings.Join(parts, " AND ")
}

func (b *DMLBuilder) renderSets(rc *gooq.RenderContext) string {
	var sets []string
	for _, cv := range b.setValues {
		sets = append(sets, renderSetValue(rc, cv))
	}
	return strings.Join(sets, ", ")
}

// renderSetValue 渲染 SET 值：NULL、表达式片段，或绑定参数。
func renderSetValue(rc *gooq.RenderContext, cv columnValue) string {
	if cv.null {
		return rc.Quote(cv.column) + " = NULL"
	}
	if expr, ok := cv.value.(gooq.Expression); ok {
		valSQL := rc.Render(expr)
		return fmt.Sprintf("%s = %s", rc.Quote(cv.column), valSQL)
	}
	return fmt.Sprintf("%s = %s", rc.Quote(cv.column), rc.AddArg(cv.value))
}

func (b *DMLBuilder) joinRenders(rc *gooq.RenderContext) []gooq.JoinRender {
	out := make([]gooq.JoinRender, 0, len(b.joins))
	for _, j := range b.joins {
		keyword := joinKeywordSQL(rc, joinKeyword[j.joinType])
		out = append(out, gooq.JoinRender{
			Keyword: keyword,
			Table:   renderTableName(rc, j.table),
			On:      b.renderJoinCond(rc, j),
		})
	}
	return out
}

func (b *DMLBuilder) renderJoinCond(rc *gooq.RenderContext, j *joinClause) string {
	var ons []string
	for _, c := range j.on {
		onSQL := rc.Render(c)
		if onSQL != "" {
			ons = append(ons, parenthesizeSQL(onSQL))
		}
	}
	if !joinSoftDeleteInWhere(j.joinType) {
		if tenant := tenantPredicate(rc, j.table); tenant != "" {
			ons = append(ons, tenant)
		}
	}
	return strings.Join(ons, " AND ")
}

func (b *DMLBuilder) renderReturning(rc *gooq.RenderContext) (string, error) {
	if len(b.returning) == 0 {
		return "", nil
	}
	var keyword string
	if rc.Spec() != nil {
		keyword = rc.Spec().Returning()
	}
	if keyword == "" {
		return "", fmt.Errorf("gooq: RETURNING is not supported by dialect %s", rc.Dialect())
	}
	var parts []string
	for _, f := range b.returning {
		fieldSQL := rc.Render(f)
		parts = append(parts, fieldSQL)
	}
	return " " + keyword + " " + strings.Join(parts, ", "), nil
}
