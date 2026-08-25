package gooq

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
)

type lockMode int

const (
	lockNone lockMode = iota
	lockForUpdate
	lockInShareMode
)

type joinType int

const (
	joinLeft joinType = iota
	joinRight
	joinInner
	joinFull
	joinLeftLateral
	joinInnerLateral
	joinCrossLateral
)

var joinKeyword = map[joinType]string{
	joinLeft:         "LEFT JOIN",
	joinRight:        "RIGHT JOIN",
	joinInner:        "INNER JOIN",
	joinFull:         "FULL JOIN",
	joinLeftLateral:  "LEFT JOIN LATERAL",
	joinInnerLateral: "INNER JOIN LATERAL",
	joinCrossLateral: "CROSS JOIN LATERAL",
}

type joinClause struct {
	joinType joinType
	table    Table
	on       []Expression
}

type setOp int

const (
	setUnionAll setOp = iota
	setUnion
	setIntersect
	setExcept
)

var setOpKeyword = map[setOp]string{
	setUnionAll:  "UNION ALL",
	setUnion:     "UNION",
	setIntersect: "INTERSECT",
	setExcept:    "EXCEPT",
}

type setOpClause struct {
	op    setOp
	query *SelectBuilder
}

type cteClause struct {
	name  string
	query *SelectBuilder
}
type groupExtKind int

const (
	groupExtNone groupExtKind = iota
	groupExtRollup
	groupExtCube
	groupExtGroupingSets
)

type SelectBuilder struct {
	ctx          context.Context
	fields       []Expression   // SELECT 字段。
	from         Table          // FROM 表或派生表。
	joins        []*joinClause  // JOIN 子句。
	conditions   []Expression   // WHERE 条件（默认 AND）。
	groupBy      []Expression   // GROUP BY。
	groupExt     groupExtKind   // GROUP BY 扩展（ROLLUP/CUBE/GROUPING SETS）。
	groupExtSets [][]Expression // 扩展分组字段（ROLLUP/CUBE 单组；GROUPING SETS 每组一个列表）。
	having       []Expression   // HAVING 条件。
	orderBy      []OrderClause  // ORDER BY。
	limit        int            // LIMIT（0 表示不限制）。
	offset       int            // OFFSET。
	distinct     bool           // DISTINCT。
	unscoped     bool           // 是否绕过软删除自动条件。
	alias        string         // 子查询别名（派生表或标量子查询的 AS alias）。
	cacheOption  *CacheOption   // 单查询缓存配置（nil 不缓存）。
	pageCacheOpt *CacheOption   // 分页缓存配置（count 与各页 data 同 key 同生命周期，共享一个配置）。
	pageNum      int            // 分页页码（Page 设置，供分页缓存 field 使用）。
	pageSize     int            // 分页条数（Page 设置，供分页缓存 field 使用）。
	lock         lockMode       // 行锁模式（默认无锁）。
	setOps       []setOpClause  // 集合操作（UNION/INTERSECT/EXCEPT）。
	ctes         []cteClause    // CTE（WITH name AS (query)）。
	recursive    bool           // CTE 是否递归（WITH RECURSIVE）。
	executor     executor       // 执行器（UseDB/UseTX 绑定；nil 时仅离线渲染）。
	columns      []string       // 结果列名（字段收集；派生表列访问与 FieldsEx 使用）。
}

func (b *SelectBuilder) TableName() string {
	return ""
}

func (b *SelectBuilder) Alias() string {
	return b.alias
}

func (b *SelectBuilder) Meta() *TableMeta {
	return nil
}

func (b *SelectBuilder) AllColumns() []string {
	return b.columns
}

func (b *SelectBuilder) Field(column string) Field[any] {
	return Field[any]{tableName: b.alias, columnName: column}
}

type JoinBuilder[T any] struct {
	parent T
	clause *joinClause
}

func (j *JoinBuilder[T]) On(conditions ...Expression) T {
	j.clause.on = append(j.clause.on, conditions...)
	return j.parent
}

func cloneJoins(joins []*joinClause) []*joinClause {
	result := make([]*joinClause, len(joins))
	for i, j := range joins {
		newJ := *j
		newJ.on = append([]Expression(nil), j.on...)
		result[i] = &newJ
	}
	return result
}

func (b *SelectBuilder) Clone() *SelectBuilder {
	newB := *b
	newB.fields = append([]Expression(nil), b.fields...)
	newB.conditions = append([]Expression(nil), b.conditions...)
	newB.groupBy = append([]Expression(nil), b.groupBy...)
	newB.having = append([]Expression(nil), b.having...)
	newB.orderBy = append([]OrderClause(nil), b.orderBy...)
	newB.columns = append([]string(nil), b.columns...)
	newB.joins = cloneJoins(b.joins)
	if b.groupExtSets != nil {
		newB.groupExtSets = make([][]Expression, len(b.groupExtSets))
		for i, set := range b.groupExtSets {
			newB.groupExtSets[i] = append([]Expression(nil), set...)
		}
	}
	newB.setOps = append([]setOpClause(nil), b.setOps...)
	for i := range newB.setOps {
		if newB.setOps[i].query != nil {
			newB.setOps[i].query = newB.setOps[i].query.Clone()
		}
	}
	newB.ctes = append([]cteClause(nil), b.ctes...)
	for i := range newB.ctes {
		if newB.ctes[i].query != nil {
			newB.ctes[i].query = newB.ctes[i].query.Clone()
		}
	}
	if sub, ok := b.from.(*SelectBuilder); ok {
		newB.from = sub.Clone()
	}
	return &newB
}

func Select(fields ...any) *SelectBuilder {
	b := &SelectBuilder{ctx: context.Background()}
	return b.Fields(fields...)
}

func SelectFrom(t Table) *SelectBuilder {
	return Select().From(t)
}

func (b *SelectBuilder) Ctx(ctx context.Context) *SelectBuilder {
	b.ctx = ctx
	return b
}

func (b *SelectBuilder) Fields(fields ...any) *SelectBuilder {
	for _, f := range fields {
		switch v := f.(type) {
		case []Field[any]:
			for _, f2 := range v {
				b.fields = append(b.fields, f2)
				b.columns = append(b.columns, f2.ColumnName())
			}
		case Expression:
			b.fields = append(b.fields, v)
			if c, ok := v.(interface{ ColumnName() string }); ok {
				b.columns = append(b.columns, c.ColumnName())
			}
		default:
			b.fields = append(b.fields, Raw(fmt.Sprintf(`%v`, f)))
		}
	}
	return b
}

func (b *SelectBuilder) FieldsEx(fields ...any) *SelectBuilder {
	excluded := make(map[string]bool)
	for _, f := range fields {
		if field, ok := f.(interface{ ColumnName() string }); ok {
			excluded[field.ColumnName()] = true
		}
	}
	var kept []Expression
	if b.from != nil {
		for _, col := range b.from.AllColumns() {
			if !excluded[col] {
				kept = append(kept, b.from.Field(col))
				b.columns = append(b.columns, col)
			}
		}
	}
	b.fields = kept
	return b
}

func (b *SelectBuilder) From(t Table) *SelectBuilder {
	b.from = t
	return b
}

func (b *SelectBuilder) Distinct() *SelectBuilder {
	b.distinct = true
	return b
}

func (b *SelectBuilder) Where(conditions ...Expression) *SelectBuilder {
	b.conditions = append(b.conditions, conditions...)
	return b
}

func (b *SelectBuilder) And(condition Expression) *SelectBuilder {
	b.conditions = append(b.conditions, condition)
	return b
}

func (b *SelectBuilder) Or(condition Expression) *SelectBuilder {
	b.conditions = append(b.conditions, OR(condition))
	return b
}

func (b *SelectBuilder) Order(clauses ...OrderClause) *SelectBuilder {
	b.orderBy = append(b.orderBy, clauses...)
	return b
}

func (b *SelectBuilder) Group(fields ...any) *SelectBuilder {
	b.groupBy = append(b.groupBy, toExpressions(fields)...)
	return b
}

func (b *SelectBuilder) GroupRollup(fields ...any) *SelectBuilder {
	return b.setGroupExt(groupExtRollup, [][]Expression{toExpressions(fields)})
}

func (b *SelectBuilder) GroupCube(fields ...any) *SelectBuilder {
	return b.setGroupExt(groupExtCube, [][]Expression{toExpressions(fields)})
}

func (b *SelectBuilder) GroupingSets(sets ...[]Expression) *SelectBuilder {
	return b.setGroupExt(groupExtGroupingSets, sets)
}

func (b *SelectBuilder) setGroupExt(kind groupExtKind, sets [][]Expression) *SelectBuilder {
	b.groupExt = kind
	b.groupExtSets = sets
	return b
}

func toExpressions(fields []any) []Expression {
	var exprs []Expression
	for _, f := range fields {
		if expr, ok := f.(Expression); ok {
			exprs = append(exprs, expr)
		}
	}
	return exprs
}

func (b *SelectBuilder) Having(conditions ...Expression) *SelectBuilder {
	b.having = append(b.having, conditions...)
	return b
}

func (b *SelectBuilder) Limit(n int) *SelectBuilder {
	b.limit = n
	return b
}

func (b *SelectBuilder) Offset(n int) *SelectBuilder {
	b.offset = n
	return b
}

func (b *SelectBuilder) Page(page, size int) *SelectBuilder {
	b.pageNum = page
	b.pageSize = size
	b.offset = (page - 1) * size
	b.limit = size
	return b
}

func (b *SelectBuilder) Unscoped() *SelectBuilder {
	b.unscoped = true
	return b
}

func (b *SelectBuilder) LockForUpdate() *SelectBuilder {
	b.lock = lockForUpdate
	return b
}

func (b *SelectBuilder) LockInShareMode() *SelectBuilder {
	b.lock = lockInShareMode
	return b
}

func (b *SelectBuilder) Cache(option CacheOption) *SelectBuilder {
	b.cacheOption = &option
	return b
}

func (b *SelectBuilder) PageCache(option CacheOption) *SelectBuilder {
	b.pageCacheOpt = &option
	return b
}

func (b *SelectBuilder) LeftJoin(t Table) *JoinBuilder[*SelectBuilder] {
	return b.addJoin(joinLeft, t)
}

func (b *SelectBuilder) RightJoin(t Table) *JoinBuilder[*SelectBuilder] {
	return b.addJoin(joinRight, t)
}

func (b *SelectBuilder) InnerJoin(t Table) *JoinBuilder[*SelectBuilder] {
	return b.addJoin(joinInner, t)
}

func (b *SelectBuilder) FullJoin(t Table) *JoinBuilder[*SelectBuilder] {
	return b.addJoin(joinFull, t)
}

func (b *SelectBuilder) LeftJoinLateral(t Table) *JoinBuilder[*SelectBuilder] {
	return b.addJoin(joinLeftLateral, t)
}

func (b *SelectBuilder) InnerJoinLateral(t Table) *JoinBuilder[*SelectBuilder] {
	return b.addJoin(joinInnerLateral, t)
}

func (b *SelectBuilder) CrossJoinLateral(t Table) *SelectBuilder {
	return b.addJoin(joinCrossLateral, t).parent
}

func (b *SelectBuilder) addJoin(joinType joinType, t Table) *JoinBuilder[*SelectBuilder] {
	clause := &joinClause{joinType: joinType, table: t}
	b.joins = append(b.joins, clause)
	return &JoinBuilder[*SelectBuilder]{parent: b, clause: clause}
}

func (b *SelectBuilder) As(alias string) *SelectBuilder {
	b.alias = alias
	return b
}

func (b *SelectBuilder) UnionAll(other *SelectBuilder) *SelectBuilder {
	b.setOps = append(b.setOps, setOpClause{op: setUnionAll, query: other})
	return b
}

func (b *SelectBuilder) Union(other *SelectBuilder) *SelectBuilder {
	b.setOps = append(b.setOps, setOpClause{op: setUnion, query: other})
	return b
}

func (b *SelectBuilder) Intersect(other *SelectBuilder) *SelectBuilder {
	b.setOps = append(b.setOps, setOpClause{op: setIntersect, query: other})
	return b
}

func (b *SelectBuilder) Except(other *SelectBuilder) *SelectBuilder {
	b.setOps = append(b.setOps, setOpClause{op: setExcept, query: other})
	return b
}

func (b *SelectBuilder) With(name string, query *SelectBuilder) *SelectBuilder {
	b.ctes = append(b.ctes, cteClause{name: name, query: query})
	return b
}

func (b *SelectBuilder) WithRecursive(name string, query *SelectBuilder) *SelectBuilder {
	b.recursive = true
	return b.With(name, query)
}

func With(name string, query *SelectBuilder) *SelectBuilder {
	return Select().With(name, query)
}

func WithRecursive(name string, query *SelectBuilder) *SelectBuilder {
	return Select().WithRecursive(name, query)
}

func Cte(name string) Table {
	return &cteTable{name: name}
}

type cteTable struct {
	name string
}

func (c *cteTable) TableName() string {
	return c.name
}

func (c *cteTable) Alias() string {
	return ""
}

func (c *cteTable) Meta() *TableMeta {
	return nil
}

func (c *cteTable) AllColumns() []string {
	return nil
}

func (c *cteTable) Field(column string) Field[any] {
	return Field[any]{tableName: c.name, columnName: column}
}

func (b *SelectBuilder) Condition() (string, []any) {
	return b.render(newRenderContext(b.ctx, DialectMySQL))
}

func (b *SelectBuilder) render(rc *renderContext) (string, []any) {
	sql, args := b.renderSelect(rc)
	sql = "(" + sql + ")"
	if b.alias != "" {
		sql += " AS " + b.alias
	}
	return sql, args
}

func (b *SelectBuilder) UseDB(db gdb.DB) *SelectBuilder {
	b.executor = db
	return b
}

func (b *SelectBuilder) UseTX(tx gdb.TX) *SelectBuilder {
	b.executor = &txExecutor{tx: tx}
	return b
}

func (b *SelectBuilder) Scan(ctx context.Context, dest any) error {
	if b.executor == nil {
		return fmt.Errorf("gooq: no database bound, use UseDB/UseTX before Scan")
	}
	dialect := b.dialect()
	sql, args, err := b.ToSql(dialect)
	if err != nil {
		return err
	}
	if b.cacheOption != nil {
		if adapter := GetCacheAdapter(); adapter != nil {
			return b.scanWithCache(ctx, adapter, dialect, sql, args, dest)
		}
	}
	return scanExec(ctx, b.executor, sql, args, dest)
}

func (b *SelectBuilder) scanWithCache(
	ctx context.Context, adapter CacheAdapter, dialect Dialect, sql string, args []any, dest any,
) error {
	key, err := b.cacheKey(dialect)
	if err != nil {
		return err
	}
	if bytes, ok, err := adapter.Get(ctx, key); err == nil && ok {
		return json.Unmarshal(bytes, dest)
	}
	if err := scanExec(ctx, b.executor, sql, args, dest); err != nil {
		return err
	}
	if bytes, err := json.Marshal(dest); err == nil {
		_ = adapter.Set(ctx, key, bytes, b.cacheOption.Duration)
	}
	return nil
}

func (b *SelectBuilder) Row(ctx context.Context) (Record, error) {
	if b.executor == nil {
		return nil, fmt.Errorf("gooq: no database bound, use UseDB/UseTX before Row")
	}
	sql, args, err := b.ToSql(b.dialect())
	if err != nil {
		return nil, err
	}
	record, err := b.executor.GetOne(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, nil
	}
	return Record(record), nil
}

func (b *SelectBuilder) Count(ctx context.Context) (int64, error) {
	if b.executor == nil {
		return 0, fmt.Errorf("gooq: no database bound, use UseDB/UseTX before Count")
	}
	countBuilder := b.Clone()
	countBuilder.fields = []Expression{Raw("COUNT(*)")}
	sql, args, err := countBuilder.ToSql(b.dialect())
	if err != nil {
		return 0, err
	}
	value, err := b.executor.GetValue(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return value.Int64(), nil
}

func (b *SelectBuilder) Exists(ctx context.Context) (bool, error) {
	if b.executor == nil {
		return false, fmt.Errorf("gooq: no database bound, use UseDB/UseTX before Exists")
	}
	dialect := b.dialect()
	subSQL, subArgs := b.renderSelect(newRenderContext(b.ctx, dialect))
	value, err := b.executor.GetValue(ctx, fmt.Sprintf("SELECT EXISTS (%s)", subSQL), subArgs...)
	if err != nil {
		return false, err
	}
	return value.Bool(), nil
}

func (b *SelectBuilder) dialect() Dialect {
	if b.executor != nil {
		return autoDialect(b.executor)
	}
	return DialectMySQL
}

// ToSql renders the SQL with the given dialect, or the default MySQL dialect
// when the parameter is omitted.
func (b *SelectBuilder) ToSql(dialects ...Dialect) (string, []any, error) {
	var dialect = DialectMySQL
	if len(dialects) > 0 && dialects[0] != "" {
		dialect = dialects[0]
	}
	if err := b.validate(dialect); err != nil {
		return "", nil, err
	}
	rc := newRenderContext(b.ctx, dialect)
	sql, args := b.renderSelect(rc)
	return sql, args, nil
}

func (b *SelectBuilder) validate(dialect Dialect) error {
	if dialect == DialectMySQL {
		switch b.groupExt {
		case groupExtCube, groupExtGroupingSets:
			return fmt.Errorf("gooq: GROUP BY %s is not supported by dialect mysql", groupExtKeyword(b.groupExt))
		default:
		}
	}
	visit := func(e Expression) error {
		if g, ok := e.(*groupConcatExpr); ok && g.distinct {
			switch dialect {
			case DialectPgsql:
				return fmt.Errorf("gooq: GROUP_CONCAT DISTINCT is not supported by dialect %s", dialect)
			}
		}
		return nil
	}
	for _, e := range b.fields {
		if err := walkExpression(e, dialect, visit); err != nil {
			return err
		}
	}
	for _, e := range b.conditions {
		if err := walkExpression(e, dialect, visit); err != nil {
			return err
		}
	}
	for _, j := range b.joins {
		for _, on := range j.on {
			if err := walkExpression(on, dialect, visit); err != nil {
				return err
			}
		}
	}
	for _, e := range b.groupBy {
		if err := walkExpression(e, dialect, visit); err != nil {
			return err
		}
	}
	for _, e := range b.having {
		if err := walkExpression(e, dialect, visit); err != nil {
			return err
		}
	}
	for _, o := range b.orderBy {
		if err := walkExpression(o.field, dialect, visit); err != nil {
			return err
		}
	}
	return nil
}

func groupExtKeyword(k groupExtKind) string {
	switch k {
	case groupExtRollup:
		return "ROLLUP"
	case groupExtCube:
		return "CUBE"
	case groupExtGroupingSets:
		return "GROUPING SETS"
	default:
		return ""
	}
}

func (b *SelectBuilder) renderGroupExt(rc *renderContext) string {
	switch b.groupExt {
	case groupExtRollup, groupExtCube:
		var flat []string
		for _, set := range b.groupExtSets {
			for _, f := range set {
				groupSQL, _ := rc.render(f)
				flat = append(flat, groupSQL)
			}
		}
		return groupExtKeyword(b.groupExt) + "(" + strings.Join(flat, ", ") + ")"
	case groupExtGroupingSets:
		var setParts []string
		for _, set := range b.groupExtSets {
			var fields []string
			for _, f := range set {
				groupSQL, _ := rc.render(f)
				fields = append(fields, groupSQL)
			}
			setParts = append(setParts, "("+strings.Join(fields, ", ")+")")
		}
		return "GROUPING SETS(" + strings.Join(setParts, ", ") + ")"
	default:
		return ""
	}
}

func (b *SelectBuilder) renderSelect(rc *renderContext) (string, []any) {
	var (
		sql     strings.Builder
		selects []string
	)
	if len(b.ctes) > 0 {
		var cteParts []string
		for _, cte := range b.ctes {
			cteSQL, _ := cte.query.renderSelect(rc)
			cteParts = append(cteParts, cte.name+" AS ("+cteSQL+")")
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
			selectSQL, _ := rc.render(f)
			if selectSQL != "" {
				selects = append(selects, selectSQL)
			}
		}
		sql.WriteString(strings.Join(selects, ", "))
	}
	sql.WriteString(" FROM ")
	sql.WriteString(b.renderTable(rc, b.from))
	for _, j := range b.joins {
		keyword := joinKeyword[j.joinType]
		if j.joinType == joinInnerLateral && rc.dialect == DialectSQLite {
			keyword = "CROSS JOIN LATERAL"
		}
		sql.WriteString(" ")
		sql.WriteString(keyword)
		sql.WriteString(" ")
		sql.WriteString(b.renderTable(rc, j.table))
		if len(j.on) > 0 {
			var ons []string
			for _, c := range j.on {
				onSQL, _ := rc.render(c)
				if onSQL != "" {
					ons = append(ons, onSQL)
				}
			}
			if len(ons) > 0 {
				sql.WriteString(" ON ")
				sql.WriteString(strings.Join(ons, " AND "))
			}
		}
	}
	if where := b.renderWhere(rc); where != "" {
		sql.WriteString(" WHERE ")
		sql.WriteString(where)
	}
	if len(b.groupBy) > 0 || b.groupExt != groupExtNone {
		var groups []string
		for _, g := range b.groupBy {
			groupSQL, _ := rc.render(g)
			groups = append(groups, groupSQL)
		}
		if b.groupExt != groupExtNone && b.groupExt == groupExtRollup && rc.dialect == DialectMySQL {
			for _, set := range b.groupExtSets {
				for _, f := range set {
					groupSQL, _ := rc.render(f)
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
			havingSQL, _ := rc.render(h)
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
		subSQL, _ := so.query.renderSelect(rc)
		sql.WriteString(" ")
		sql.WriteString(setOpKeyword[so.op])
		sql.WriteString(" ")
		sql.WriteString(subSQL)
	}
	if len(b.orderBy) > 0 {
		var orders []string
		for _, o := range b.orderBy {
			orderSQL, _ := o.render(rc)
			orders = append(orders, orderSQL)
		}
		sql.WriteString(" ORDER BY ")
		sql.WriteString(strings.Join(orders, ", "))
	}
	if b.limit > 0 {
		if rc.dialectInfo != nil && rc.dialectInfo.RenderLimit != nil {
			// 驱动注册的自定义分页渲染（扩展点）。
			sql.WriteString(rc.dialectInfo.RenderLimit(rc, b.limit, b.offset))
		} else {
			sql.WriteString(" LIMIT ")
			_, _ = fmt.Fprintf(&sql, "%d", b.limit)
			if b.offset > 0 {
				sql.WriteString(" OFFSET ")
				_, _ = fmt.Fprintf(&sql, "%d", b.offset)
			}
		}
	}
	if b.lock != lockNone && rc.dialect != DialectSQLite {
		switch b.lock {
		case lockForUpdate:
			sql.WriteString(" FOR UPDATE")
		case lockInShareMode:
			sql.WriteString(" " + rc.shareLockKeyword())
		default:
		}
	}
	return sql.String(), rc.args
}

// renderTable 渲染表或派生表。
func (b *SelectBuilder) renderTable(rc *renderContext, t Table) string {
	return renderTableName(rc, t)
}

// renderTableName 渲染表或派生表（表名 + 别名）；SelectBuilder 与 DMLBuilder 共用。
func renderTableName(rc *renderContext, t Table) string {
	if sub, ok := t.(*SelectBuilder); ok {
		subSQL, _ := sub.renderSelect(rc)
		return "(" + subSQL + ") AS " + sub.alias
	}
	var sql = tableNameSQL(rc, t)
	if t.Alias() != "" {
		sql += " AS " + t.Alias()
	}
	return sql
}

// tableNameSQL 渲染限定表名（schema.table）；表元数据无 schema 时仅表名。
func tableNameSQL(rc *renderContext, t Table) string {
	var name = rc.quote(t.TableName())
	if m := t.Meta(); m != nil && m.Schema != "" {
		name = rc.quote(m.Schema) + "." + name
	}
	return name
}

// renderWhere 渲染 WHERE 条件（含软删除自动条件）。
func (b *SelectBuilder) renderWhere(rc *renderContext) string {
	conditions := b.conditions
	if !b.unscoped && b.from != nil && b.from.Meta() != nil {
		if softField := b.from.Meta().SoftDeleteField(); softField != nil && !containsColumn(b.conditions, softField.ColumnName) {
			conditions = append(conditions, b.from.Field(softField.ColumnName).IsNull())
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

// containsColumn 判断条件树中是否显式引用了指定列（软删除显式接管）。
func containsColumn(conditions []Expression, columnName string) bool {
	for _, c := range conditions {
		if walkContainsColumn(c, columnName) {
			return true
		}
	}
	return false
}

func walkContainsColumn(e Expression, columnName string) bool {
	switch v := e.(type) {
	case *fieldCondition:
		return v.columnName == columnName
	case *groupCondition:
		for _, c := range v.conditions {
			if walkContainsColumn(c, columnName) {
				return true
			}
		}
	}
	return false
}
