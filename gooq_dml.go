package gooq

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/util/gconv"
)

type dmlKind int

const (
	dmlInsert dmlKind = iota
	dmlInsertFrom
	dmlUpdate
	dmlDelete
)

type upsertClause struct {
	conflictCols []string
	updateMap    map[string]any
	doNothing    bool // 冲突时不做任何操作。
}

type DMLBuilder struct {
	ctx           context.Context
	table         Table
	kind          dmlKind
	data          map[string]any
	dataList      []map[string]any // 批量 INSERT 数据。
	selectBuilder *SelectBuilder   // INSERT ... SELECT 数据源。
	conditions    []Expression
	unscoped      bool
	upsert        *upsertClause
	joins         []*joinClause // UPDATE 多表 JOIN 子句。
	returning     []Expression  // RETURNING/OUTPUT 返回列。
	executor      executor      // 执行器（UseDB/UseTX 绑定；nil 时仅离线渲染）。
}

func Insert(t Table, data any) *DMLBuilder {
	b := &DMLBuilder{
		ctx:   context.Background(),
		table: t,
		kind:  dmlInsert,
		data:  make(map[string]any),
	}
	switch v := data.(type) {
	case []map[string]any:
		for _, row := range v {
			b.dataList = append(b.dataList, row)
		}
	case []any:
		for _, row := range v {
			b.dataList = append(b.dataList, gconv.Map(row))
		}
	default:
		if isSliceValue(v) {
			for _, row := range toSlice(v) {
				b.dataList = append(b.dataList, gconv.Map(row))
			}
		} else {
			b.data = gconv.Map(v)
		}
	}
	return b
}

func InsertFrom(t Table, sub *SelectBuilder) *DMLBuilder {
	return &DMLBuilder{
		ctx:           context.Background(),
		table:         t,
		kind:          dmlInsertFrom,
		selectBuilder: sub,
	}
}

func isSliceValue(v any) bool {
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array
}

func toSlice(v any) []any {
	rv := reflect.ValueOf(v)
	result := make([]any, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		result[i] = rv.Index(i).Interface()
	}
	return result
}

func Update(t Table) *DMLBuilder {
	return &DMLBuilder{
		ctx:   context.Background(),
		table: t,
		kind:  dmlUpdate,
		data:  make(map[string]any),
	}
}

func Delete(t Table) *DMLBuilder {
	return &DMLBuilder{
		ctx:   context.Background(),
		table: t,
		kind:  dmlDelete,
	}
}

func (b *DMLBuilder) Ctx(ctx context.Context) *DMLBuilder {
	b.ctx = ctx
	return b
}

func (b *DMLBuilder) Data(data any) *DMLBuilder {
	for k, v := range gconv.Map(data) {
		b.data[k] = v
	}
	return b
}

func (b *DMLBuilder) Set(field interface{ ColumnName() string }, v any) *DMLBuilder {
	b.data[field.ColumnName()] = v
	return b
}

func (b *DMLBuilder) Where(conds ...Expression) *DMLBuilder {
	b.conditions = append(b.conditions, conds...)
	return b
}

func (b *DMLBuilder) LeftJoin(t Table) *JoinBuilder[*DMLBuilder] {
	return b.addJoin(joinLeft, t)
}

func (b *DMLBuilder) RightJoin(t Table) *JoinBuilder[*DMLBuilder] {
	return b.addJoin(joinRight, t)
}

func (b *DMLBuilder) InnerJoin(t Table) *JoinBuilder[*DMLBuilder] {
	return b.addJoin(joinInner, t)
}

func (b *DMLBuilder) FullJoin(t Table) *JoinBuilder[*DMLBuilder] {
	return b.addJoin(joinFull, t)
}

func (b *DMLBuilder) addJoin(joinType joinType, t Table) *JoinBuilder[*DMLBuilder] {
	clause := &joinClause{joinType: joinType, table: t}
	b.joins = append(b.joins, clause)
	return &JoinBuilder[*DMLBuilder]{parent: b, clause: clause}
}

func (b *DMLBuilder) Returning(fields ...any) *DMLBuilder {
	b.returning = append(b.returning, toExpressions(fields)...)
	return b
}

func (b *DMLBuilder) Unscoped() *DMLBuilder {
	b.unscoped = true
	return b
}

func (b *DMLBuilder) OnConflictKey(fields ...interface{ ColumnName() string }) *DMLBuilder {
	if b.upsert == nil {
		b.upsert = &upsertClause{updateMap: make(map[string]any)}
	}
	for _, f := range fields {
		b.upsert.conflictCols = append(b.upsert.conflictCols, f.ColumnName())
	}
	return b
}

func (b *DMLBuilder) DoNothing() *DMLBuilder {
	if b.upsert == nil {
		b.upsert = &upsertClause{updateMap: make(map[string]any)}
	}
	b.upsert.doNothing = true
	return b
}

func (b *DMLBuilder) DoUpdate(field interface{ ColumnName() string }, v any) *DMLBuilder {
	if b.upsert == nil {
		b.upsert = &upsertClause{updateMap: make(map[string]any)}
	}
	b.upsert.updateMap[field.ColumnName()] = v
	return b
}

func (b *DMLBuilder) UseDB(db gdb.DB) *DMLBuilder {
	b.executor = db
	return b
}

func (b *DMLBuilder) UseTX(tx gdb.TX) *DMLBuilder {
	b.executor = &txExecutor{tx: tx}
	return b
}

func (b *DMLBuilder) Exec(ctx context.Context) (sql.Result, error) {
	if b.executor == nil {
		return nil, fmt.Errorf("gooq: no database bound, use UseDB/UseTX before Exec")
	}
	sqlStr, args, err := b.ToSql(b.dmlDialect())
	if err != nil {
		return nil, err
	}
	return b.executor.Exec(ctx, sqlStr, args...)
}

func (b *DMLBuilder) Scan(ctx context.Context, dest any) error {
	if b.executor == nil {
		return fmt.Errorf("gooq: no database bound, use UseDB/UseTX before Scan")
	}
	if len(b.returning) == 0 {
		return fmt.Errorf("gooq: Scan on DML requires Returning fields")
	}
	sqlStr, args, err := b.ToSql(b.dmlDialect())
	if err != nil {
		return err
	}
	return scanExec(ctx, b.executor, sqlStr, args, dest)
}

func (b *DMLBuilder) dmlDialect() Dialect {
	if b.executor != nil {
		return autoDialect(b.executor)
	}
	return DialectMySQL
}

func (b *DMLBuilder) ToSql(dialect Dialect) (string, []any, error) {
	if dialect == "" {
		dialect = DialectMySQL
	}
	rc := newRenderContext(b.ctx, dialect)
	switch b.kind {
	case dmlInsert:
		return b.renderInsert(rc)
	case dmlInsertFrom:
		return b.renderInsertFrom(rc)
	case dmlUpdate:
		return b.renderUpdate(rc)
	case dmlDelete:
		return b.renderDelete(rc)
	default:
		return "", nil, fmt.Errorf("gooq: unknown dml kind %d", b.kind)
	}
}

func (b *DMLBuilder) renderInsertFrom(rc *renderContext) (string, []any, error) {
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
	subSQL, _ := b.selectBuilder.renderSelect(rc)
	var sqlStr = fmt.Sprintf(
		"INSERT INTO %s (%s) %s",
		b.table.TableName(),
		strings.Join(columns, ", "),
		subSQL,
	)
	if returning, err := b.renderReturning(rc); err != nil {
		return "", nil, err
	} else if returning != "" {
		sqlStr += returning
	}
	return sqlStr, rc.args, nil
}

func (b *DMLBuilder) renderInsert(rc *renderContext) (string, []any, error) {
	if len(b.dataList) > 0 {
		return b.renderInsertBatch(rc)
	}
	var (
		columns  []string
		autoIncr = autoIncrementColumn(b.table)
	)
	for col := range b.data {
		if col == autoIncr {
			continue
		}
		columns = append(columns, col)
	}
	if len(columns) == 0 {
		return "", nil, fmt.Errorf("gooq: insert data is empty")
	}
	sort.Strings(columns)
	values := make([]string, len(columns))
	for i, col := range columns {
		values[i] = rc.addArg(b.data[col])
	}
	insertKeyword := "INSERT"
	if b.upsert != nil && b.upsert.doNothing && rc.dialect == DialectMySQL {
		insertKeyword = "INSERT IGNORE"
	}
	var sqlStr = fmt.Sprintf(
		"%s INTO %s (%s) VALUES (%s)",
		insertKeyword,
		b.table.TableName(),
		strings.Join(columns, ", "),
		strings.Join(values, ", "),
	)
	if b.upsert != nil && !(b.upsert.doNothing && rc.dialect == DialectMySQL) {
		sqlStr += renderUpsertClause(rc, b.upsert, rc.dialect)
	}
	if returning, err := b.renderReturning(rc); err != nil {
		return "", nil, err
	} else if returning != "" {
		sqlStr += returning
	}
	return sqlStr, rc.args, nil
}

func (b *DMLBuilder) renderInsertBatch(rc *renderContext) (string, []any, error) {
	var (
		columnSet = make(map[string]bool)
		autoIncr  = autoIncrementColumn(b.table)
	)
	for _, row := range b.dataList {
		for col := range row {
			if col != autoIncr {
				columnSet[col] = true
			}
		}
	}
	columns := make([]string, 0, len(columnSet))
	for col := range columnSet {
		columns = append(columns, col)
	}
	sort.Strings(columns)
	if len(columns) == 0 {
		return "", nil, fmt.Errorf("gooq: insert batch data is empty")
	}
	sort.Strings(columns)
	var rows []string
	for _, row := range b.dataList {
		placeholders := make([]string, len(columns))
		for i, col := range columns {
			placeholders[i] = rc.addArg(row[col])
		}
		rows = append(rows, "("+strings.Join(placeholders, ", ")+")")
	}
	insertKeyword := "INSERT"
	if b.upsert != nil && b.upsert.doNothing && rc.dialect == DialectMySQL {
		insertKeyword = "INSERT IGNORE"
	}
	var sqlStr = fmt.Sprintf(
		"%s INTO %s (%s) VALUES %s",
		insertKeyword,
		b.table.TableName(),
		strings.Join(columns, ", "),
		strings.Join(rows, ", "),
	)
	if b.upsert != nil && !(b.upsert.doNothing && rc.dialect == DialectMySQL) {
		sqlStr += renderUpsertClause(rc, b.upsert, rc.dialect)
	}
	if returning, err := b.renderReturning(rc); err != nil {
		return "", nil, err
	} else if returning != "" {
		sqlStr += returning
	}
	return sqlStr, rc.args, nil
}

func (b *DMLBuilder) renderUpdate(rc *renderContext) (string, []any, error) {
	if len(b.data) == 0 {
		return "", nil, fmt.Errorf("gooq: update data is empty")
	}
	b.registerAliases(rc)
	var sqlStr string
	switch {
	case len(b.joins) == 0:
		sqlStr = fmt.Sprintf("UPDATE %s SET %s", renderTableName(rc, b.table), b.renderSets(rc))
	case rc.dialect == DialectPgsql || rc.dialect == DialectSQLite:
		sqlStr = fmt.Sprintf("UPDATE %s SET %s FROM %s", renderTableName(rc, b.table), b.renderSets(rc), b.renderJoinTables(rc))
	default:
		sqlStr = fmt.Sprintf("UPDATE %s %s SET %s", renderTableName(rc, b.table), b.renderJoinClauses(rc), b.renderSets(rc))
	}
	if where := b.renderWhere(rc); where != "" {
		sqlStr += " WHERE " + where
	}
	if returning, err := b.renderReturning(rc); err != nil {
		return "", nil, err
	} else if returning != "" {
		sqlStr += returning
	}
	return sqlStr, rc.args, nil
}

func (b *DMLBuilder) renderDelete(rc *renderContext) (string, []any, error) {
	if !b.unscoped && b.table.Meta() != nil {
		if softField := b.table.Meta().SoftDeleteField(); softField != nil {
			sqlStr := fmt.Sprintf(
				"UPDATE %s SET %s = %s",
				b.table.TableName(),
				softField.ColumnName,
				rc.addArg(time.Now()),
			)
			if where := b.renderWhere(rc); where != "" {
				sqlStr += " WHERE " + where
			}
			if returning, err := b.renderReturning(rc); err != nil {
				return "", nil, err
			} else if returning != "" {
				sqlStr += returning
			}
			return sqlStr, rc.args, nil
		}
	}
	var sqlStr = "DELETE FROM " + b.table.TableName()
	if where := b.renderWhere(rc); where != "" {
		sqlStr += " WHERE " + where
	}
	if returning, err := b.renderReturning(rc); err != nil {
		return "", nil, err
	} else if returning != "" {
		sqlStr += returning
	}
	return sqlStr, rc.args, nil
}

func (b *DMLBuilder) renderWhere(rc *renderContext) string {
	conds := b.conditions
	if len(b.joins) > 0 && (rc.dialect == DialectPgsql || rc.dialect == DialectSQLite) {
		for _, j := range b.joins {
			conds = append(conds, j.on...)
		}
	}
	var parts []string
	for _, c := range conds {
		condSQL, _ := rc.render(c)
		if condSQL != "" {
			parts = append(parts, condSQL)
		}
	}
	return strings.Join(parts, " AND ")
}

func (b *DMLBuilder) renderSets(rc *renderContext) string {
	var sets []string
	for col, val := range b.data {
		sets = append(sets, fmt.Sprintf(`%s = %s`, col, rc.addArg(val)))
	}
	return strings.Join(sets, ", ")
}

func (b *DMLBuilder) renderJoinClauses(rc *renderContext) string {
	var joins []string
	for _, j := range b.joins {
		joins = append(joins, joinKeyword[j.joinType]+" "+renderTableName(rc, j.table)+b.renderJoinOn(rc, j))
	}
	return strings.Join(joins, " ")
}

// renderJoinTables 渲染 FROM 表列表（PG/SQLite 的 UPDATE...FROM 语法，无 ON 条件）。
func (b *DMLBuilder) renderJoinTables(rc *renderContext) string {
	var tables []string
	for _, j := range b.joins {
		tables = append(tables, renderTableName(rc, j.table))
	}
	return strings.Join(tables, ", ")
}

func (b *DMLBuilder) renderJoinOn(rc *renderContext, j *joinClause) string {
	if len(j.on) == 0 {
		return ""
	}
	var ons []string
	for _, c := range j.on {
		onSQL, _ := rc.render(c)
		if onSQL != "" {
			ons = append(ons, onSQL)
		}
	}
	return " ON " + strings.Join(ons, " AND ")
}

func (b *DMLBuilder) registerAliases(rc *renderContext) {
	if b.table.Alias() != "" {
		rc.registerAlias(b.table.TableName(), b.table.Alias())
	}
	for _, j := range b.joins {
		if j.table.Alias() != "" {
			rc.registerAlias(j.table.TableName(), j.table.Alias())
		}
	}
}

func (b *DMLBuilder) renderReturning(rc *renderContext) (string, error) {
	if len(b.returning) == 0 {
		return "", nil
	}
	var keyword string
	if rc.dialectInfo != nil {
		keyword = rc.dialectInfo.Returning
	}
	if keyword == "" {
		return "", fmt.Errorf("gooq: RETURNING is not supported by dialect %s", rc.dialect)
	}
	var parts []string
	for _, f := range b.returning {
		fieldSQL, _ := rc.render(f)
		parts = append(parts, fieldSQL)
	}
	return " " + keyword + " " + strings.Join(parts, ", "), nil
}

func autoIncrementColumn(t Table) string {
	if t.Meta() == nil {
		return ""
	}
	for i := range t.Meta().Fields {
		if t.Meta().Fields[i].AutoIncrement {
			return t.Meta().Fields[i].ColumnName
		}
	}
	return ""
}

func renderUpsertClause(rc *renderContext, clause *upsertClause, dialect Dialect) string {
	switch dialect {
	case DialectPgsql:
		if clause.doNothing {
			return fmt.Sprintf(
				" ON CONFLICT (%s) DO NOTHING",
				strings.Join(clause.conflictCols, ", "),
			)
		}
		var sets []string
		for col, val := range clause.updateMap {
			sets = append(sets, fmt.Sprintf(`%s = %s`, col, rc.addArg(val)))
		}
		return fmt.Sprintf(
			" ON CONFLICT (%s) DO UPDATE SET %s",
			strings.Join(clause.conflictCols, ", "),
			strings.Join(sets, ", "),
		)
	default:
		var sets []string
		for col := range clause.updateMap {
			sets = append(sets, fmt.Sprintf(`%s = VALUES(%s)`, col, col))
		}
		return " ON DUPLICATE KEY UPDATE " + strings.Join(sets, ", ")
	}
}
