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
	"github.com/gogf/gf/v2/text/gstr"
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

type columnValue struct {
	column string
	value  any
}

type DMLBuilder struct {
	ctx             context.Context
	table           Table
	kind            dmlKind
	insertColumns   []string        // Columns 设置的列（当前组）。
	insertRows      [][]columnValue // INSERT 行数据（保序列值对）。
	batch           int             // INSERT 分批大小（0 不分批）。
	setValues       []columnValue   // UPDATE SET 列值对（保序）。
	batchUpdateRows [][]columnValue // 批量 UPDATE 数据（每条：列值对）。
	updateKeys      []string        // 批量 UPDATE 条件列（默认主键）。
	selectBuilder   *SelectBuilder  // INSERT ... SELECT 数据源。
	conditions      []Expression
	unscoped        bool
	upsert          *upsertClause
	joins           []*joinClause // UPDATE 多表 JOIN 子句。
	returning       []Expression  // RETURNING/OUTPUT 返回列。
	executor        executor      // 执行器（UseDB/UseTX 绑定；nil 时仅离线渲染）。
}

func Insert(t Table) *DMLBuilder {
	return &DMLBuilder{
		ctx:   context.Background(),
		table: t,
		kind:  dmlInsert,
	}
}

func InsertFrom(t Table, sub *SelectBuilder) *DMLBuilder {
	return &DMLBuilder{
		ctx:           context.Background(),
		table:         t,
		kind:          dmlInsertFrom,
		selectBuilder: sub,
	}
}

func (b *DMLBuilder) Columns(fields ...any) *DMLBuilder {
	for _, f := range fields {
		if field, ok := f.(interface{ ColumnName() string }); ok {
			b.insertColumns = append(b.insertColumns, field.ColumnName())
		}
	}
	return b
}

func (b *DMLBuilder) Values(values ...any) *DMLBuilder {
	if len(b.insertColumns) == 0 {
		return b
	}
	row := make([]columnValue, len(b.insertColumns))
	for i, col := range b.insertColumns {
		row[i] = columnValue{column: col, value: values[i]}
	}
	b.insertRows = append(b.insertRows, row)
	return b
}

func (b *DMLBuilder) Record(data any) *DMLBuilder {
	row := recordToRow(b.table, data, b.kind == dmlInsert)
	if b.kind == dmlUpdate {
		b.setValues = append(b.setValues, row...)
		return b
	}
	b.insertRows = append(b.insertRows, row)
	return b
}

func (b *DMLBuilder) Records(data any) *DMLBuilder {
	rv := reflect.ValueOf(data)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return b
	}
	for i := 0; i < rv.Len(); i++ {
		row := recordToRow(b.table, rv.Index(i).Interface(), b.kind == dmlInsert)
		if len(row) == 0 {
			continue
		}
		if b.kind == dmlUpdate || b.kind == dmlDelete {
			b.batchUpdateRows = append(b.batchUpdateRows, row)
		} else {
			b.insertRows = append(b.insertRows, row)
		}
	}
	return b
}

func (b *DMLBuilder) Batch(size int) *DMLBuilder {
	b.batch = size
	return b
}

func (b *DMLBuilder) Keys(fields ...interface{ ColumnName() string }) *DMLBuilder {
	for _, f := range fields {
		b.updateKeys = append(b.updateKeys, f.ColumnName())
	}
	return b
}

func recordToRow(t Table, data any, skipAutoIncr bool) []columnValue {
	rv := reflect.ValueOf(data)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}
	if t.Meta() == nil {
		return nil
	}
	autoIncr := autoIncrementColumn(t)
	rt := rv.Type()
	var row []columnValue
	for _, fm := range t.Meta().Fields {
		if skipAutoIncr && fm.ColumnName == autoIncr {
			continue
		}
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			if f.PkgPath != "" || !fieldMatchesColumn(f, fm.ColumnName) {
				continue
			}
			fv := rv.Field(i)
			if fv.IsZero() {
				break
			}
			row = append(row, columnValue{column: fm.ColumnName, value: fv.Interface()})
			break
		}
	}
	return row
}

func fieldMatchesColumn(f reflect.StructField, column string) bool {
	for _, tag := range []string{"orm", "json"} {
		if v := f.Tag.Get(tag); v != "" {
			if strings.Split(v, ",")[0] == column {
				return true
			}
		}
	}
	return gstr.CaseSnake(f.Name) == column
}

func (b *DMLBuilder) Clone() *DMLBuilder {
	newB := *b
	newB.conditions = append([]Expression(nil), b.conditions...)
	newB.joins = cloneJoins(b.joins)
	newB.returning = append([]Expression(nil), b.returning...)
	newB.insertColumns = append([]string(nil), b.insertColumns...)
	newB.setValues = append([]columnValue(nil), b.setValues...)
	newB.updateKeys = append([]string(nil), b.updateKeys...)
	if b.insertRows != nil {
		newB.insertRows = make([][]columnValue, len(b.insertRows))
		for i, row := range b.insertRows {
			newB.insertRows[i] = append([]columnValue(nil), row...)
		}
	}
	if b.batchUpdateRows != nil {
		newB.batchUpdateRows = make([][]columnValue, len(b.batchUpdateRows))
		for i, row := range b.batchUpdateRows {
			newB.batchUpdateRows[i] = append([]columnValue(nil), row...)
		}
	}
	if b.upsert != nil {
		newUpsert := *b.upsert
		newUpsert.updateMap = make(map[string]any, len(b.upsert.updateMap))
		for k, v := range b.upsert.updateMap {
			newUpsert.updateMap[k] = v
		}
		newB.upsert = &newUpsert
	}
	if b.selectBuilder != nil {
		newB.selectBuilder = b.selectBuilder.Clone()
	}
	return &newB
}

func Update(t Table) *DMLBuilder {
	return &DMLBuilder{
		ctx:   context.Background(),
		table: t,
		kind:  dmlUpdate,
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

func (b *DMLBuilder) Data(data map[string]any) *DMLBuilder {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.setValues = append(b.setValues, columnValue{column: k, value: data[k]})
	}
	return b
}

func (b *DMLBuilder) Set(field interface{ ColumnName() string }, v any) *DMLBuilder {
	b.setValues = append(b.setValues, columnValue{column: field.ColumnName(), value: v})
	return b
}

func (b *DMLBuilder) Where(conditions ...Expression) *DMLBuilder {
	b.conditions = append(b.conditions, conditions...)
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
	dialect := b.dmlDialect()
	if (b.kind == dmlUpdate || b.kind == dmlDelete) && len(b.batchUpdateRows) > 0 {
		sqls, argss, err := b.renderBatchDML(dialect)
		if err != nil {
			return nil, err
		}
		if len(sqls) == 0 {
			return nil, fmt.Errorf("gooq: batch delete/update has no valid records")
		}
		var (
			total      int64
			lastResult sql.Result
		)
		for i, sqlStr := range sqls {
			result, err := b.executor.Exec(ctx, sqlStr, argss[i]...)
			if err != nil {
				return nil, err
			}
			if n, err := result.RowsAffected(); err == nil {
				total += n
			}
			lastResult = result
		}
		return &batchResult{result: lastResult, affected: total}, nil
	}
	if b.kind == dmlInsert && b.batch > 0 && len(b.insertRows) > b.batch {
		var (
			total      int64
			lastResult sql.Result
		)
		for start := 0; start < len(b.insertRows); start += b.batch {
			end := start + b.batch
			if end > len(b.insertRows) {
				end = len(b.insertRows)
			}
			chunk := *b
			chunk.insertRows = b.insertRows[start:end]
			sqlStr, args, err := chunk.ToSql(dialect)
			if err != nil {
				return nil, err
			}
			result, err := b.executor.Exec(ctx, sqlStr, args...)
			if err != nil {
				return nil, err
			}
			if n, err := result.RowsAffected(); err == nil {
				total += n
			}
			lastResult = result
		}
		return &batchResult{result: lastResult, affected: total}, nil
	}
	sqlStr, args, err := b.ToSql(dialect)
	if err != nil {
		return nil, err
	}
	return b.executor.Exec(ctx, sqlStr, args...)
}

type batchResult struct {
	result   sql.Result
	affected int64
}

func (r *batchResult) LastInsertId() (int64, error) {
	return r.result.LastInsertId()
}

func (r *batchResult) RowsAffected() (int64, error) {
	return r.affected, nil
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

// ToSql renders the SQL with the given dialect, or the default MySQL dialect
// when the parameter is omitted.
func (b *DMLBuilder) ToSql(dialects ...Dialect) (string, []any, error) {
	var dialect = DialectMySQL
	if len(dialects) > 0 && dialects[0] != "" {
		dialect = dialects[0]
	}
	if (b.kind == dmlUpdate || b.kind == dmlDelete) && len(b.batchUpdateRows) > 0 {
		return "", nil, fmt.Errorf("gooq: batch delete/update renders multiple SQL, use Exec")
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
		tableNameSQL(rc, b.table),
		strings.Join(quoteColumns(rc, columns), ", "),
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
	if len(b.insertRows) == 0 {
		return "", nil, fmt.Errorf("gooq: insert data is empty")
	}
	columns := make([]string, len(b.insertRows[0]))
	for i, cv := range b.insertRows[0] {
		columns[i] = cv.column
	}
	if len(columns) == 0 {
		return "", nil, fmt.Errorf("gooq: insert columns is empty")
	}
	rows := make([]string, len(b.insertRows))
	for i, row := range b.insertRows {
		if len(row) != len(columns) {
			return "", nil, fmt.Errorf(
				"gooq: insert values count %d mismatch columns count %d", len(row), len(columns),
			)
		}
		placeholders := make([]string, len(columns))
		for j, col := range columns {
			placeholders[j] = rc.addArg(valueOf(row, col))
		}
		rows[i] = "(" + strings.Join(placeholders, ", ") + ")"
	}
	insertKeyword := "INSERT"
	if b.upsert != nil && b.upsert.doNothing && rc.dialect == DialectMySQL {
		insertKeyword = "INSERT IGNORE"
	}
	var sqlStr = fmt.Sprintf(
		"%s INTO %s (%s) VALUES %s",
		insertKeyword,
		tableNameSQL(rc, b.table),
		strings.Join(quoteColumns(rc, columns), ", "),
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

func quoteColumns(rc *renderContext, columns []string) []string {
	result := make([]string, len(columns))
	for i, c := range columns {
		result[i] = rc.quote(c)
	}
	return result
}

func valueOf(row []columnValue, column string) any {
	for _, cv := range row {
		if cv.column == column {
			return cv.value
		}
	}
	return nil
}

func isInStrings(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

func (b *DMLBuilder) renderUpdate(rc *renderContext) (string, []any, error) {
	if len(b.setValues) == 0 {
		return "", nil, fmt.Errorf("gooq: update data is empty")
	}
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

func (b *DMLBuilder) renderBatchDML(dialect Dialect) ([]string, [][]any, error) {
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
			setCols []string
			setArgs []any
			where   []string
			whereA  []any
		)
		for _, cv := range row {
			if isInStrings(keys, cv.column) {
				where = append(where, cv.column)
				whereA = append(whereA, cv.value)
			} else {
				setCols = append(setCols, cv.column)
				setArgs = append(setArgs, cv.value)
			}
		}
		if len(where) == 0 || (b.kind == dmlUpdate && len(setCols) == 0) {
			continue
		}
		rc := newRenderContext(b.ctx, dialect)
		var sqlStr string
		switch b.kind {
		case dmlDelete:
			softField := b.table.Meta().SoftDeleteField()
			if softField != nil && !b.unscoped {
				sqlStr = fmt.Sprintf(
					"UPDATE %s SET %s = %s",
					tableNameSQL(rc, b.table),
					rc.quote(softField.ColumnName),
					rc.addArg(time.Now()),
				)
			} else {
				sqlStr = "DELETE FROM " + tableNameSQL(rc, b.table)
			}
		default:
			placeholders := make([]string, len(setCols))
			for i := range setCols {
				placeholders[i] = fmt.Sprintf("%s = %s", rc.quote(setCols[i]), rc.addArg(setArgs[i]))
			}
			sqlStr = fmt.Sprintf("UPDATE %s SET %s", tableNameSQL(rc, b.table), strings.Join(placeholders, ", "))
		}
		whereParts := make([]string, len(where))
		for i := range where {
			whereParts[i] = fmt.Sprintf("%s = %s", rc.quote(where[i]), rc.addArg(whereA[i]))
		}
		sqls = append(sqls, sqlStr+" WHERE "+strings.Join(whereParts, " AND "))
		argss = append(argss, rc.args)
	}
	return sqls, argss, nil
}

func (b *DMLBuilder) renderDelete(rc *renderContext) (string, []any, error) {
	if !b.unscoped && b.table.Meta() != nil {
		if softField := b.table.Meta().SoftDeleteField(); softField != nil {
			sqlStr := fmt.Sprintf(
				"UPDATE %s SET %s = %s",
				tableNameSQL(rc, b.table),
				rc.quote(softField.ColumnName),
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
	var sqlStr = "DELETE FROM " + tableNameSQL(rc, b.table)
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
	conditions := b.conditions
	if len(b.joins) > 0 && (rc.dialect == DialectPgsql || rc.dialect == DialectSQLite) {
		for _, j := range b.joins {
			conditions = append(conditions, j.on...)
		}
	}
	var parts []string
	for _, c := range conditions {
		condSQL, _ := rc.render(c)
		if condSQL != "" {
			parts = append(parts, condSQL)
		}
	}
	return strings.Join(parts, " AND ")
}

func (b *DMLBuilder) renderSets(rc *renderContext) string {
	var sets []string
	for _, cv := range b.setValues {
		sets = append(sets, fmt.Sprintf(`%s = %s`, rc.quote(cv.column), rc.addArg(cv.value)))
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
				strings.Join(quoteColumns(rc, clause.conflictCols), ", "),
			)
		}
		var sets []string
		for col, val := range clause.updateMap {
			sets = append(sets, fmt.Sprintf(`%s = %s`, rc.quote(col), rc.addArg(val)))
		}
		return fmt.Sprintf(
			" ON CONFLICT (%s) DO UPDATE SET %s",
			strings.Join(quoteColumns(rc, clause.conflictCols), ", "),
			strings.Join(sets, ", "),
		)
	default:
		var sets []string
		for col := range clause.updateMap {
			sets = append(sets, fmt.Sprintf(`%s = VALUES(%s)`, rc.quote(col), rc.quote(col)))
		}
		return " ON DUPLICATE KEY UPDATE " + strings.Join(sets, ", ")
	}
}
