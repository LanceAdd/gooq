package dsl

import (
	"fmt"
	"strings"
	"time"

	"github.com/lanceadd/gooq"
)

// ToSql renders the SQL with the given dialect, or the default MySQL dialect
// when the parameter is omitted.
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
	// 列清单：优先子查询字段的列名，否则表全列。
	var columns []string
	if b.selectBuilder != nil && len(b.selectBuilder.fields) > 0 {
		for _, f := range b.selectBuilder.fields {
			if field, ok := f.(interface{ ColumnName() string }); ok {
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

func isInStrings(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

func (b *DMLBuilder) renderUpdate(rc *gooq.RenderContext) (string, []any, error) {
	if len(b.setValues) == 0 {
		return "", nil, fmt.Errorf("gooq: update data is empty")
	}
	spec := rc.Spec()
	if spec == nil {
		return "", nil, fmt.Errorf("gooq: dialect %q is not registered", rc.Dialect())
	}
	target := renderTableName(rc, b.table)
	sets := b.renderSets(rc)
	joins := b.joinRenders(rc)
	where := b.renderWhere(rc)
	sqlStr, err := spec.RenderUpdate(rc, target, sets, where, joins)
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
		return nil, nil, fmt.Errorf("gooq: batch delete/update requires primary key or Keys(...)")
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
			if isInStrings(keys, cv.column) {
				where = append(where, cv)
			} else {
				sets = append(sets, cv)
			}
		}
		if len(where) == 0 || (b.kind == dmlUpdate && len(sets) == 0) {
			continue
		}
		rc := gooq.NewRenderContext(dialect)
		var sqlStr string
		switch b.kind {
		case dmlDelete:
			var softField *gooq.FieldMeta
			if b.table.Meta() != nil {
				softField = b.table.Meta().SoftDeleteField()
			}
			if softField != nil && !b.unscoped {
				sqlStr = fmt.Sprintf(
					"UPDATE %s SET %s = %s",
					tableNameSQL(rc, b.table),
					rc.Quote(softField.ColumnName),
					rc.AddArg(time.Now()),
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
			if cv.null {
				whereParts[i] = rc.Quote(cv.column) + " IS NULL"
				continue
			}
			whereParts[i] = fmt.Sprintf("%s = %s", rc.Quote(cv.column), rc.AddArg(cv.value))
		}
		if extra := b.renderWhere(rc); extra != "" {
			whereParts = append(whereParts, extra)
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
	spec := rc.Spec()
	if spec == nil {
		return "", nil, fmt.Errorf("gooq: dialect %q is not registered", rc.Dialect())
	}
	sets := renderSetValue(rc, columnValue{column: softField.ColumnName, value: time.Now()})
	sqlStr, err := spec.RenderUpdate(rc, renderTableName(rc, b.table), sets, b.renderWhere(rc), b.joinRenders(rc))
	if err != nil {
		return "", nil, err
	}
	returning, err := b.renderReturning(rc)
	if err != nil {
		return "", nil, err
	}
	return sqlStr + returning, rc.Args(), nil
}

func (b *DMLBuilder) renderWhere(rc *gooq.RenderContext) string {
	var parts []string
	for _, c := range b.conditions {
		condSQL := rc.Render(c)
		if condSQL != "" {
			parts = append(parts, condSQL)
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
	if len(j.on) == 0 {
		return ""
	}
	var ons []string
	for _, c := range j.on {
		onSQL := rc.Render(c)
		if onSQL != "" {
			ons = append(ons, onSQL)
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
