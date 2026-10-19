package dsl

import (
	"fmt"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
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
	table    gooq.Table
	on       []gooq.Expression
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
	fields        []gooq.Expression
	from          gooq.Table
	joins         []*joinClause
	conditions    []gooq.Expression // 默认 AND。
	groupBy       []gooq.Expression
	groupExt      groupExtKind
	groupExtSets  [][]gooq.Expression // 扩展分组字段（ROLLUP/CUBE 单组；GROUPING SETS 每组一个列表）。
	having        []gooq.Expression
	orderBy       []gooq.Expression
	limit         int // 0 表示不限制。
	offset        int
	distinct      bool
	unscoped      bool // 绕过软删除自动条件。
	untenant      bool // 绕过本层主表和 JOIN 的租户条件。
	tenantIDs     []any
	hasTenantIDs  bool
	ctxTenant     any
	hasCtxTenant  bool
	alias         string // 子查询别名（派生表或标量子查询的 AS alias）。
	cacheOption   *CacheOption
	pageCacheOpt  *CacheOption // count 与各页 data 同 key 同生命周期。
	buildErr      error        // 构造期错误，ToSql 和执行时返回它。
	lock          lockMode
	lockWait      gooq.RowLockWait
	setOps        []setOpClause
	ctes          []cteClause
	recursive     bool     // CTE 递归（WITH RECURSIVE）。
	executor      executor // 测试注入的执行器；正式执行由 DB(name)、显式事务和 ctx 中的事务决定。
	explicitTx    *db.Tx   // Tx(tx) 指定的连接。nil 表示仍从 ctx 查找。
	group         string   // DB(name) 指定的配置组；空字符串表示默认组。
	groupSet      bool     // 是否显式指定了非默认配置组。
	route         db.Route
	shardingValue any
	hasShardValue bool
	columns       []string // 结果列名（字段收集；派生表列访问与 FieldsEx 使用）。
}

func (b *SelectBuilder) TableName() string {
	return ""
}

func (b *SelectBuilder) AliasName() string {
	return b.alias
}

func (b *SelectBuilder) Meta() *gooq.TableMeta {
	return nil
}

func (b *SelectBuilder) AllColumns() []string {
	return b.columns
}

func (b *SelectBuilder) Field(column string) gooq.Field[any] {
	return gooq.NewField[any](b.alias, column)
}

type JoinBuilder[T any] struct {
	parent T
	clause *joinClause
}

func (j *JoinBuilder[T]) On(conditions ...gooq.Expression) T {
	j.clause.on = append(j.clause.on, conditions...)
	return j.parent
}

func cloneJoins(joins []*joinClause) []*joinClause {
	result := make([]*joinClause, len(joins))
	for i, j := range joins {
		newJ := *j
		newJ.on = append([]gooq.Expression(nil), j.on...)
		result[i] = &newJ
	}
	return result
}

func (b *SelectBuilder) Clone() *SelectBuilder {
	newB := *b
	newB.fields = append([]gooq.Expression(nil), b.fields...)
	newB.conditions = append([]gooq.Expression(nil), b.conditions...)
	newB.groupBy = append([]gooq.Expression(nil), b.groupBy...)
	newB.having = append([]gooq.Expression(nil), b.having...)
	newB.orderBy = append([]gooq.Expression(nil), b.orderBy...)
	newB.columns = append([]string(nil), b.columns...)
	newB.joins = cloneJoins(b.joins)
	if b.groupExtSets != nil {
		newB.groupExtSets = make([][]gooq.Expression, len(b.groupExtSets))
		for i, set := range b.groupExtSets {
			newB.groupExtSets[i] = append([]gooq.Expression(nil), set...)
		}
	}
	newB.setOps = append([]setOpClause(nil), b.setOps...)
	for i := range newB.setOps {
		if newB.setOps[i].query != nil {
			newB.setOps[i].query = newB.setOps[i].query.Clone()
		}
	}
	newB.tenantIDs = copyTenantIDs(b.tenantIDs)
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
	b := &SelectBuilder{}
	return b.Fields(fields...)
}

func SelectFrom(t gooq.Table) *SelectBuilder {
	return Select().From(t)
}

func (b *SelectBuilder) Fields(fields ...any) *SelectBuilder {
	for _, f := range fields {
		switch v := f.(type) {
		case []gooq.Field[any]:
			for _, f2 := range v {
				b.fields = append(b.fields, f2)
				b.columns = append(b.columns, f2.ColumnName())
			}
		case gooq.Expression:
			b.fields = append(b.fields, v)
			if c, ok := v.(gooq.ColumnNamer); ok {
				b.columns = append(b.columns, c.ColumnName())
			}
		default:
			b.fields = append(b.fields, gooq.Raw(fmt.Sprintf(`%v`, f)))
		}
	}
	return b
}

func (b *SelectBuilder) FieldsEx(fields ...any) *SelectBuilder {
	excluded := make(map[string]bool)
	for _, f := range fields {
		if field, ok := f.(gooq.ColumnNamer); ok {
			excluded[field.ColumnName()] = true
		}
	}
	b.columns = nil
	var kept []gooq.Expression
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

func (b *SelectBuilder) From(t gooq.Table) *SelectBuilder {
	b.from = t
	return b
}

func (b *SelectBuilder) Distinct() *SelectBuilder {
	b.distinct = true
	return b
}

func (b *SelectBuilder) Where(conditions ...gooq.Expression) *SelectBuilder {
	b.conditions = append(b.conditions, conditions...)
	return b
}

// Order 追加排序表达式，原样渲染（方向由表达式自带：Field.Asc()/Desc()、gooq.OrderAscExpr/DescExpr；任意片段用 gooq.Raw）。
func (b *SelectBuilder) Order(exprs ...gooq.Expression) *SelectBuilder {
	b.orderBy = append(b.orderBy, exprs...)
	return b
}

// OrderAsc 追加排序表达式，逐条包装为升序。
func (b *SelectBuilder) OrderAsc(exprs ...gooq.Expression) *SelectBuilder {
	for _, e := range exprs {
		b.orderBy = append(b.orderBy, gooq.OrderAscExpr(e))
	}
	return b
}

// OrderDesc 追加排序表达式，逐条包装为降序。
func (b *SelectBuilder) OrderDesc(exprs ...gooq.Expression) *SelectBuilder {
	for _, e := range exprs {
		b.orderBy = append(b.orderBy, gooq.OrderDescExpr(e))
	}
	return b
}

func (b *SelectBuilder) Group(fields ...any) *SelectBuilder {
	b.groupBy = append(b.groupBy, toExpressions(fields)...)
	return b
}

func (b *SelectBuilder) GroupRollup(fields ...any) *SelectBuilder {
	return b.setGroupExt(groupExtRollup, [][]gooq.Expression{toExpressions(fields)})
}

func (b *SelectBuilder) GroupCube(fields ...any) *SelectBuilder {
	return b.setGroupExt(groupExtCube, [][]gooq.Expression{toExpressions(fields)})
}

func (b *SelectBuilder) GroupingSets(sets ...[]gooq.Expression) *SelectBuilder {
	return b.setGroupExt(groupExtGroupingSets, sets)
}

func (b *SelectBuilder) setGroupExt(kind groupExtKind, sets [][]gooq.Expression) *SelectBuilder {
	if b.groupExt != groupExtNone {
		b.buildErr = fmt.Errorf("gooq: GROUP BY extension is already set")
		return b
	}
	b.groupExt = kind
	b.groupExtSets = sets
	return b
}

func toExpressions(fields []any) []gooq.Expression {
	var exprs []gooq.Expression
	for _, f := range fields {
		if expr, ok := f.(gooq.Expression); ok {
			exprs = append(exprs, expr)
		}
	}
	return exprs
}

func (b *SelectBuilder) Having(conditions ...gooq.Expression) *SelectBuilder {
	b.having = append(b.having, conditions...)
	return b
}

// Limit 设置返回行数。0 表示不限制。负数写入构造错误，渲染和执行都会失败。
func (b *SelectBuilder) Limit(n int) *SelectBuilder {
	if n < 0 {
		b.buildErr = fmt.Errorf("gooq: LIMIT must be greater than or equal to 0")
		return b
	}
	b.limit = n
	return b
}

func (b *SelectBuilder) Offset(n int) *SelectBuilder {
	b.offset = n
	return b
}

// Page 按页码和页大小写入 OFFSET 与 LIMIT。页大小为负数时写入构造错误。
func (b *SelectBuilder) Page(page, size int) *SelectBuilder {
	if size < 0 {
		b.buildErr = fmt.Errorf("gooq: page size must be greater than or equal to 0")
		return b
	}
	b.offset = (page - 1) * size
	b.limit = size
	return b
}

func (b *SelectBuilder) Unscoped() *SelectBuilder {
	b.unscoped = true
	return b
}

// Master 把这条查询强制路由到主库。
func (b *SelectBuilder) Master() *SelectBuilder {
	b.route = db.RouteMaster
	return b
}

// Slave 把这条查询强制路由到从库；组里没有从库时回落到主库。
func (b *SelectBuilder) Slave() *SelectBuilder {
	b.route = db.RouteSlave
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

// SkipLocked 让行锁跳过已被锁住的行。必须先调用 LockForUpdate 或 LockInShareMode。
// 后调用的等待策略覆盖先调用的。方言没有 RenderRowLockWait 时，ToSql 失败。
func (b *SelectBuilder) SkipLocked() *SelectBuilder {
	b.lockWait = gooq.RowLockSkipLocked
	return b
}

// NoWait 让行锁遇到其他事务持有的锁时立刻失败。必须先调用 LockForUpdate 或 LockInShareMode。
// 后调用的等待策略覆盖先调用的。方言没有 RenderRowLockWait 时，ToSql 失败。
func (b *SelectBuilder) NoWait() *SelectBuilder {
	b.lockWait = gooq.RowLockNoWait
	return b
}

// Cache 开启单查询缓存；无参或零值 option 不开启。
func (b *SelectBuilder) Cache(option ...CacheOption) *SelectBuilder {
	if len(option) > 0 && option[0].valid() {
		b.cacheOption = &option[0]
	} else {
		b.cacheOption = nil
	}
	return b
}

// PageCache 开启复合查询（RowsAndCount）缓存；无参或零值 option 不开启。
func (b *SelectBuilder) PageCache(option ...CacheOption) *SelectBuilder {
	if len(option) > 0 && option[0].valid() {
		b.pageCacheOpt = &option[0]
	} else {
		b.pageCacheOpt = nil
	}
	return b
}

// ShardBy 给这条查询的主表设置分片值。nil 也算设置过。
// JOIN 和其他分片表不使用这个值，要在表上调用 ShardBy。
func (b *SelectBuilder) ShardBy(value any) *SelectBuilder {
	b.shardingValue = value
	b.hasShardValue = true
	return b
}

func (b *SelectBuilder) LeftJoin(t gooq.Table) *JoinBuilder[*SelectBuilder] {
	return b.addJoin(joinLeft, t)
}

func (b *SelectBuilder) RightJoin(t gooq.Table) *JoinBuilder[*SelectBuilder] {
	return b.addJoin(joinRight, t)
}

func (b *SelectBuilder) InnerJoin(t gooq.Table) *JoinBuilder[*SelectBuilder] {
	return b.addJoin(joinInner, t)
}

func (b *SelectBuilder) FullJoin(t gooq.Table) *JoinBuilder[*SelectBuilder] {
	return b.addJoin(joinFull, t)
}

func (b *SelectBuilder) LeftJoinLateral(t gooq.Table) *JoinBuilder[*SelectBuilder] {
	return b.addJoin(joinLeftLateral, t)
}

func (b *SelectBuilder) InnerJoinLateral(t gooq.Table) *JoinBuilder[*SelectBuilder] {
	return b.addJoin(joinInnerLateral, t)
}

func (b *SelectBuilder) CrossJoinLateral(t gooq.Table) *SelectBuilder {
	return b.addJoin(joinCrossLateral, t).parent
}

func (b *SelectBuilder) addJoin(joinType joinType, t gooq.Table) *JoinBuilder[*SelectBuilder] {
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

func Cte(name string) gooq.Table {
	return &cteTable{name: name}
}

type cteTable struct {
	name string
}

func (c *cteTable) TableName() string {
	return c.name
}

func (c *cteTable) AliasName() string {
	return ""
}

func (c *cteTable) Meta() *gooq.TableMeta {
	return nil
}

func (c *cteTable) AllColumns() []string {
	return nil
}

func (c *cteTable) Field(column string) gooq.Field[any] {
	return gooq.NewField[any](c.name, column)
}

// DB 选择执行时使用的配置组。
// 名字为空时使用默认组，等同于不调用 DB。
func (b *SelectBuilder) DB(name string) *SelectBuilder {
	b.group = name
	b.groupSet = name != ""
	b.noteExplicitTxGroup()
	return b
}

// Tx 指定这条语句使用的事务连接。nil 不生效，也不清掉已经设置的事务。
func (b *SelectBuilder) Tx(tx *db.Tx) *SelectBuilder {
	if tx == nil {
		return b
	}
	if tx.DB() == nil {
		b.buildErr = fmt.Errorf("gooq: Tx has no database")
		return b
	}
	b.explicitTx = tx
	b.noteExplicitTxGroup()
	return b
}

func (b *SelectBuilder) noteExplicitTxGroup() {
	if b.explicitTx == nil || b.explicitTx.DB() == nil || !b.groupSet {
		return
	}
	if b.group != b.explicitTx.DB().Group() {
		b.buildErr = fmt.Errorf("gooq: Tx group %q does not match DB %q", b.explicitTx.DB().Group(), b.group)
	}
}
