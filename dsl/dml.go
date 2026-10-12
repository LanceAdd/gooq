package dsl

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/lanceadd/gooq"
	"sort"
	"strings"
	"time"

	"github.com/lanceadd/gooq/db"
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
	null   bool
}

type DMLBuilder struct {
	table           gooq.Table
	kind            dmlKind
	insertColumns   []string        // Columns 设置的列（当前组）。
	insertRows      [][]columnValue // INSERT 行数据（保序列值对）。
	batch           int             // INSERT 分批大小（0 不分批）。
	setValues       []columnValue   // UPDATE SET 列值对（保序）。
	batchUpdateRows [][]columnValue // 批量 UPDATE 数据（每条：列值对）。
	updateKeys      []string        // 批量 UPDATE 条件列（默认主键）。
	selectBuilder   *SelectBuilder  // INSERT ... SELECT 数据源。
	conditions      []gooq.Expression
	unscoped        bool
	upsert          *upsertClause
	joins           []*joinClause     // UPDATE 多表 JOIN 子句。
	returning       []gooq.Expression // RETURNING/OUTPUT 返回列。
	executor        executor          // 测试注入的执行器；正式执行由 DB(name) 和 ctx 中的事务决定。
	group           string            // DB(name) 指定的配置组；空字符串表示默认组。
	groupSet        bool              // 是否显式指定了非默认配置组。
	route           db.Route          // 主从路由（默认写主库）。
	err             error             // 构造期错误（如 Values 数量不匹配），ToSql / Exec 返回它。
}

func Insert(t gooq.Table) *DMLBuilder {
	return &DMLBuilder{
		table: t,
		kind:  dmlInsert,
	}
}

func InsertFrom(t gooq.Table, sub *SelectBuilder) *DMLBuilder {
	return &DMLBuilder{
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
	if b.err != nil || len(b.insertColumns) == 0 {
		return b
	}
	if len(values) != len(b.insertColumns) {
		b.err = fmt.Errorf("gooq: insert values count %d does not match columns count %d", len(values), len(b.insertColumns))
		return b
	}
	row := make([]columnValue, len(b.insertColumns))
	for i, col := range b.insertColumns {
		row[i] = columnValue{column: col, value: values[i]}
	}
	b.insertRows = append(b.insertRows, row)
	return b
}

// ValuesOf writes columns marked by model.NewXxxRecord setters.
// Insert accepts one or more rows. One update row becomes SET. Several update or delete
// rows become per-row statements keyed by the primary key, executed by Exec.
func (b *DMLBuilder) ValuesOf(rows ...gooq.ValueSource) *DMLBuilder {
	parsed := assignmentsToRows(rows)
	if len(parsed) == 0 {
		return b
	}
	if b.kind != dmlUpdate && b.kind != dmlDelete {
		b.insertRows = append(b.insertRows, parsed...)
		return b
	}
	if b.kind == dmlUpdate && len(b.batchUpdateRows) == 0 && len(parsed) == 1 {
		b.setValues = append(b.setValues, parsed[0]...)
		return b
	}
	if b.kind == dmlUpdate && len(b.setValues) > 0 {
		b.batchUpdateRows = append(b.batchUpdateRows, b.setValues)
		b.setValues = nil
	}
	b.batchUpdateRows = append(b.batchUpdateRows, parsed...)
	return b
}

func assignmentsToRows(rows []gooq.ValueSource) [][]columnValue {
	parsed := make([][]columnValue, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		assignments := row.Assignments()
		cvs := make([]columnValue, 0, len(assignments))
		for _, item := range assignments {
			if item.Column == "" {
				continue
			}
			cvs = append(cvs, columnValue{column: item.Column, value: item.Value, null: item.Null})
		}
		if len(cvs) == 0 {
			continue
		}
		parsed = append(parsed, cvs)
	}
	return parsed
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

func (b *DMLBuilder) Clone() *DMLBuilder {
	newB := *b
	newB.conditions = append([]gooq.Expression(nil), b.conditions...)
	newB.joins = cloneJoins(b.joins)
	newB.returning = append([]gooq.Expression(nil), b.returning...)
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

func Update(t gooq.Table) *DMLBuilder {
	return &DMLBuilder{
		table: t,
		kind:  dmlUpdate,
	}
}

func Delete(t gooq.Table) *DMLBuilder {
	return &DMLBuilder{
		table: t,
		kind:  dmlDelete,
	}
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

func (b *DMLBuilder) Where(conditions ...gooq.Expression) *DMLBuilder {
	b.conditions = append(b.conditions, conditions...)
	return b
}

func (b *DMLBuilder) LeftJoin(t gooq.Table) *JoinBuilder[*DMLBuilder] {
	return b.addJoin(joinLeft, t)
}

func (b *DMLBuilder) RightJoin(t gooq.Table) *JoinBuilder[*DMLBuilder] {
	return b.addJoin(joinRight, t)
}

func (b *DMLBuilder) InnerJoin(t gooq.Table) *JoinBuilder[*DMLBuilder] {
	return b.addJoin(joinInner, t)
}

func (b *DMLBuilder) FullJoin(t gooq.Table) *JoinBuilder[*DMLBuilder] {
	return b.addJoin(joinFull, t)
}

func (b *DMLBuilder) addJoin(joinType joinType, t gooq.Table) *JoinBuilder[*DMLBuilder] {
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

// Master forces this statement onto a master node.
func (b *DMLBuilder) Master() *DMLBuilder {
	b.route = db.RouteMaster
	return b
}

// Slave forces this statement onto a slave node when the group has one.
func (b *DMLBuilder) Slave() *DMLBuilder {
	b.route = db.RouteSlave
	return b
}

// DB selects the configuration group used at execution.
// With no name, it uses the default group, the same as omitting DB.
func (b *DMLBuilder) DB(name ...string) *DMLBuilder {
	b.group = ""
	b.groupSet = false
	if len(name) > 0 && name[0] != "" {
		b.group = name[0]
		b.groupSet = true
	}
	return b
}

func (b *DMLBuilder) Exec(ctx context.Context) (sql.Result, error) {
	if b.err != nil {
		return nil, b.err
	}
	restore, err := b.bind(ctx)
	if err != nil {
		return nil, err
	}
	defer restore()
	dialect := b.dmlDialect()
	if _, err = gooq.DialectSpecOf(dialect); err != nil {
		return nil, err
	}
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
	// 缺列不补 NULL，留给表默认值。列集合不同拆成多条 INSERT；Batch 在每个列集合内再切分。
	if b.kind == dmlInsert && len(b.insertRows) > 0 {
		chunks := splitInsertChunks(b.insertRows, b.batch)
		if len(chunks) > 1 {
			return b.execInsertChunks(ctx, dialect, chunks)
		}
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
	restore, err := b.bind(ctx)
	if err != nil {
		return err
	}
	defer restore()
	if len(b.returning) == 0 {
		return fmt.Errorf("gooq: Scan on DML requires Returning fields")
	}
	sqlStr, args, err := b.ToSql(b.dmlDialect())
	if err != nil {
		return err
	}
	return scanExec(ctx, b.executor, sqlStr, args, dest)
}

// Value writes the first RETURNING column into dest. dest is one cell.
func (b *DMLBuilder) Value(ctx context.Context, dest any) error {
	if err := gooq.CheckValue(dest); err != nil {
		return err
	}
	return b.readReturning(ctx, func(sqlStr string, args []any) error {
		return execValue(ctx, b.executor, sqlStr, args, dest)
	})
}

// Column writes the first RETURNING column of every row into dest.
func (b *DMLBuilder) Column(ctx context.Context, dest any) error {
	if err := gooq.CheckColumn(dest); err != nil {
		return err
	}
	return b.readReturning(ctx, func(sqlStr string, args []any) error {
		return execColumn(ctx, b.executor, sqlStr, args, dest)
	})
}

func (b *DMLBuilder) readReturning(ctx context.Context, run func(string, []any) error) error {
	restore, err := b.bind(ctx)
	if err != nil {
		return err
	}
	defer restore()
	if len(b.returning) == 0 {
		return fmt.Errorf("gooq: Value and Column on DML require Returning fields")
	}
	sqlStr, args, err := b.ToSql(b.dmlDialect())
	if err != nil {
		return err
	}
	return run(sqlStr, args)
}

func (b *DMLBuilder) dmlDialect() gooq.Dialect {
	if b.executor != nil {
		return autoDialect(b.executor)
	}
	return gooq.DialectMySQL
}

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
	rows := make([]string, len(b.insertRows))
	for i, row := range b.insertRows {
		placeholders := make([]string, len(columns))
		for j, col := range columns {
			placeholders[j] = placeValue(rc, row, col)
		}
		rows[i] = "(" + strings.Join(placeholders, ", ") + ")"
	}
	spec := rc.Spec()
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
		strings.Join(rows, ", "),
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

// groupInsertRows 按列集合分组：同组内每行列集一致（可直接渲染，无 NULL 补位）；
// 组间列集不同 → 拆分多条 INSERT（缺失列走表默认值）。
func groupInsertRows(rows [][]columnValue) [][][]columnValue {
	groupMap := make(map[string][][]columnValue)
	var order []string
	for _, row := range rows {
		cols := make([]string, len(row))
		for i, cv := range row {
			cols[i] = cv.column
		}
		sort.Strings(cols)
		key := strings.Join(cols, ",")
		if _, ok := groupMap[key]; !ok {
			order = append(order, key)
		}
		groupMap[key] = append(groupMap[key], row)
	}
	groups := make([][][]columnValue, len(order))
	for i, key := range order {
		groups[i] = groupMap[key]
	}
	return groups
}

// splitInsertChunks 先按列集合分组，再按 batch 切开。batch <= 0 表示不切。
// 每个分片列集合一致，渲染时不会用 NULL 填没写的列。
func splitInsertChunks(rows [][]columnValue, batch int) [][][]columnValue {
	groups := groupInsertRows(rows)
	if batch <= 0 {
		return groups
	}
	chunks := make([][][]columnValue, 0, len(groups))
	for _, group := range groups {
		if len(group) <= batch {
			chunks = append(chunks, group)
			continue
		}
		for start := 0; start < len(group); start += batch {
			end := start + batch
			if end > len(group) {
				end = len(group)
			}
			chunks = append(chunks, group[start:end])
		}
	}
	return chunks
}

func (b *DMLBuilder) execInsertChunks(ctx context.Context, dialect gooq.Dialect, chunks [][][]columnValue) (sql.Result, error) {
	var (
		total      int64
		lastResult sql.Result
	)
	for _, rows := range chunks {
		chunk := *b
		chunk.insertRows = rows
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
			valSQL, _ := rc.Render(expr)
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
		sqls = append(sqls, sqlStr+" WHERE "+strings.Join(whereParts, " AND "))
		argss = append(argss, rc.Args())
	}
	return sqls, argss, nil
}

func (b *DMLBuilder) renderDelete(rc *gooq.RenderContext) (string, []any, error) {
	if !b.unscoped && b.table.Meta() != nil {
		if softField := b.table.Meta().SoftDeleteField(); softField != nil {
			saved := b.setValues
			b.setValues = []columnValue{{column: softField.ColumnName, value: time.Now()}}
			defer func() { b.setValues = saved }()
			return b.renderUpdate(rc)
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

func (b *DMLBuilder) renderWhere(rc *gooq.RenderContext) string {
	var parts []string
	for _, c := range b.conditions {
		condSQL, _ := rc.Render(c)
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
		valSQL, _ := rc.Render(expr)
		return fmt.Sprintf("%s = %s", rc.Quote(cv.column), valSQL)
	}
	return fmt.Sprintf("%s = %s", rc.Quote(cv.column), rc.AddArg(cv.value))
}

func (b *DMLBuilder) joinRenders(rc *gooq.RenderContext) []gooq.JoinRender {
	out := make([]gooq.JoinRender, 0, len(b.joins))
	for _, j := range b.joins {
		keyword := joinKeyword[j.joinType]
		if spec := rc.Spec(); spec != nil {
			keyword = spec.JoinKeyword(keyword)
		}
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
		onSQL, _ := rc.Render(c)
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
		fieldSQL, _ := rc.Render(f)
		parts = append(parts, fieldSQL)
	}
	return " " + keyword + " " + strings.Join(parts, ", "), nil
}
