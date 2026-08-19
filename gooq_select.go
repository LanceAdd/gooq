// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件实现 V3 类型化查询 DSL 的核心：SelectBuilder 链式构建、渲染与执行。
package gooq

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
)

// lockMode 是行锁模式。
type lockMode int

const (
	lockNone lockMode = iota
	lockForUpdate
	lockInShareMode
)

// joinType 是连接类型。
type joinType int

const (
	joinLeft joinType = iota
	joinRight
	joinInner
	joinFull
)

// joinKeyword 是连接类型对应的 SQL 关键字。
var joinKeyword = map[joinType]string{
	joinLeft:  "LEFT JOIN",
	joinRight: "RIGHT JOIN",
	joinInner: "INNER JOIN",
	joinFull:  "FULL JOIN",
}

// joinClause 是连接子句（表 + ON 条件）。
type joinClause struct {
	joinType joinType
	table    Table
	on       []Expression
}

// SelectBuilder 是类型化查询构建器：Select(字段).From(表) 链式 DSL。
// 它同时实现 Table 接口（As 后作为派生表）与 Expression 接口（子查询/标量子查询）。
type SelectBuilder struct {
	db           gdb.DB // 绑定实例（nil 时执行取默认实例）。
	ctx          context.Context
	fields       []Expression  // SELECT 字段。
	from         Table         // FROM 表或派生表。
	joins        []*joinClause // JOIN 子句。
	conditions   []Expression  // WHERE 条件（默认 AND）。
	groupBy      []Expression  // GROUP BY。
	having       []Expression  // HAVING 条件。
	orderBy      []OrderClause // ORDER BY。
	limit        int           // LIMIT（0 表示不限制）。
	offset       int           // OFFSET。
	distinct     bool          // DISTINCT。
	unscoped     bool          // 是否绕过软删除自动条件。
	alias        string        // 子查询别名（派生表或标量子查询的 AS alias）。
	cacheOption  *CacheOption  // 单查询缓存配置（nil 不缓存）。
	pageCacheOpt *CacheOption  // 分页缓存配置（count 与各页 data 同 key 同生命周期，共享一个配置）。
	pageNum      int           // 分页页码（Page 设置，供分页缓存 field 使用）。
	pageSize     int           // 分页条数（Page 设置，供分页缓存 field 使用）。
	lock         lockMode      // 行锁模式（默认无锁）。
}

// TableName 实现 Table 接口（子查询无表名，From 渲染走专门逻辑）。
func (b *SelectBuilder) TableName() string {
	return ""
}

// Alias 实现 Table 接口。
func (b *SelectBuilder) Alias() string {
	return b.alias
}

// Meta 实现 Table 接口（子查询无元数据）。
func (b *SelectBuilder) Meta() *TableMeta {
	return nil
}

// AllColumns 实现 Table 接口（子查询列未知）。
func (b *SelectBuilder) AllColumns() []string {
	return nil
}

// JoinBuilder 是连接子句构建器：LeftJoin(t).On(conds...)。
type JoinBuilder struct {
	parent *SelectBuilder
	clause *joinClause
}

// Select 创建查询构建器（未绑定实例，执行时取默认实例；多库场景用 DB 绑定）。
func Select(fields ...any) *SelectBuilder {
	b := &SelectBuilder{ctx: context.Background()}
	return b.Fields(fields...)
}

// SelectFrom 创建 SELECT * 查询构建器（jOOQ selectFrom 同款便捷入口）。
func SelectFrom(t Table) *SelectBuilder {
	return Select().From(t)
}

// DB 绑定显式实例（多库场景）。
func (b *SelectBuilder) DB(db gdb.DB) *SelectBuilder {
	b.db = db
	return b
}

// Ctx 设置查询上下文。
func (b *SelectBuilder) Ctx(ctx context.Context) *SelectBuilder {
	b.ctx = ctx
	return b
}

// Fields 设置 SELECT 字段（空参表示全字段）。
func (b *SelectBuilder) Fields(fields ...any) *SelectBuilder {
	for _, f := range fields {
		if expr, ok := f.(Expression); ok {
			b.fields = append(b.fields, expr)
		} else {
			b.fields = append(b.fields, Raw(fmt.Sprintf(`%v`, f)))
		}
	}
	return b
}

// FieldsEx 设置全字段减去排除字段（集合差集）。
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
				kept = append(kept, NewField[any](b.from.TableName(), col))
			}
		}
	}
	b.fields = kept
	return b
}

// From 设置查询表（可为派生表）。
func (b *SelectBuilder) From(t Table) *SelectBuilder {
	b.from = t
	return b
}

// Distinct 设置 DISTINCT 查询。
func (b *SelectBuilder) Distinct() *SelectBuilder {
	b.distinct = true
	return b
}

// Where 追加条件（多参数默认 AND 连接）。
func (b *SelectBuilder) Where(conds ...Expression) *SelectBuilder {
	b.conditions = append(b.conditions, conds...)
	return b
}

// And 追加 AND 条件。
func (b *SelectBuilder) And(cond Expression) *SelectBuilder {
	b.conditions = append(b.conditions, cond)
	return b
}

// Or 追加 OR 条件。
func (b *SelectBuilder) Or(cond Expression) *SelectBuilder {
	b.conditions = append(b.conditions, OR(cond))
	return b
}

// Order 设置排序子句。
func (b *SelectBuilder) Order(clauses ...OrderClause) *SelectBuilder {
	b.orderBy = append(b.orderBy, clauses...)
	return b
}

// Group 设置分组字段。
func (b *SelectBuilder) Group(fields ...any) *SelectBuilder {
	for _, f := range fields {
		if expr, ok := f.(Expression); ok {
			b.groupBy = append(b.groupBy, expr)
		}
	}
	return b
}

// Having 设置分组过滤条件。
func (b *SelectBuilder) Having(conds ...Expression) *SelectBuilder {
	b.having = append(b.having, conds...)
	return b
}

// Limit 设置查询条数限制。
func (b *SelectBuilder) Limit(n int) *SelectBuilder {
	b.limit = n
	return b
}

// Offset 设置查询偏移。
func (b *SelectBuilder) Offset(n int) *SelectBuilder {
	b.offset = n
	return b
}

// Page 设置分页（page 从 1 开始）。
func (b *SelectBuilder) Page(page, size int) *SelectBuilder {
	b.pageNum = page
	b.pageSize = size
	b.offset = (page - 1) * size
	b.limit = size
	return b
}

// Unscoped 绕过软删除自动条件。
func (b *SelectBuilder) Unscoped() *SelectBuilder {
	b.unscoped = true
	return b
}

// LockForUpdate 设置行锁 FOR UPDATE（MySQL/PG；SQLite 忽略）。
func (b *SelectBuilder) LockForUpdate() *SelectBuilder {
	b.lock = lockForUpdate
	return b
}

// LockInShareMode 设置共享锁（MySQL: LOCK IN SHARE MODE；PG: FOR SHARE；SQLite 忽略）。
func (b *SelectBuilder) LockInShareMode() *SelectBuilder {
	b.lock = lockInShareMode
	return b
}

// Cache 启用单查询缓存（键值适配器；未注入适配器时自动跳过）。
func (b *SelectBuilder) Cache(option CacheOption) *SelectBuilder {
	b.cacheOption = &option
	return b
}

// PageCache 启用分页缓存：count 与各页 data 同 key 同生命周期（哈希适配器）。
// 单个 CacheOption 即可：hash key 的 TTL 语义为刷新整个 key，count 与 data 不存在独立过期。
func (b *SelectBuilder) PageCache(option CacheOption) *SelectBuilder {
	b.pageCacheOpt = &option
	return b
}

// LeftJoin 创建 LEFT JOIN 子句（随后调用 On）。
func (b *SelectBuilder) LeftJoin(t Table) *JoinBuilder {
	return b.addJoin(joinLeft, t)
}

// RightJoin 创建 RIGHT JOIN 子句（随后调用 On）。
func (b *SelectBuilder) RightJoin(t Table) *JoinBuilder {
	return b.addJoin(joinRight, t)
}

// InnerJoin 创建 INNER JOIN 子句（随后调用 On）。
func (b *SelectBuilder) InnerJoin(t Table) *JoinBuilder {
	return b.addJoin(joinInner, t)
}

// FullJoin 创建 FULL JOIN 子句（随后调用 On）。
func (b *SelectBuilder) FullJoin(t Table) *JoinBuilder {
	return b.addJoin(joinFull, t)
}

func (b *SelectBuilder) addJoin(joinType joinType, t Table) *JoinBuilder {
	clause := &joinClause{joinType: joinType, table: t}
	b.joins = append(b.joins, clause)
	return &JoinBuilder{parent: b, clause: clause}
}

// On 设置连接条件（多条件默认 AND）。
func (j *JoinBuilder) On(conds ...Expression) *SelectBuilder {
	j.clause.on = append(j.clause.on, conds...)
	return j.parent
}

// As 设置子查询别名；返回值同时可用作派生表（From 位置）与标量子查询（SELECT 位置）。
func (b *SelectBuilder) As(alias string) *SelectBuilder {
	b.alias = alias
	return b
}

// Condition 实现 Expression 接口：渲染为 (SELECT ...) 子查询。
func (b *SelectBuilder) Condition() (string, []any) {
	return b.render(newRenderContext(b.ctx, b.db, DialectMySQL))
}

func (b *SelectBuilder) render(rc *renderContext) (string, []any) {
	// 表达式位置（子查询/标量）总是带括号包装；顶层 ToSql 不走本方法。
	sql, args := b.renderSelect(rc)
	sql = "(" + sql + ")"
	if b.alias != "" {
		sql += " AS " + b.alias
	}
	return sql, args
}

// ToSql 离线渲染 SQL（不连库）；dialect 为空时使用 MySQL 方言。
func (b *SelectBuilder) ToSql(dialect Dialect) (string, []any, error) {
	if dialect == "" {
		dialect = DialectMySQL
	}
	rc := newRenderContext(b.ctx, b.db, dialect)
	sql, args := b.renderSelect(rc)
	return sql, args, nil
}

// renderSelect 渲染完整 SELECT 语句。
func (b *SelectBuilder) renderSelect(rc *renderContext) (string, []any) {
	b.registerAliases(rc)
	var (
		sql     strings.Builder
		selects []string
	)
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
		sql.WriteString(" ")
		sql.WriteString(joinKeyword[j.joinType])
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
	if len(b.groupBy) > 0 {
		var groups []string
		for _, g := range b.groupBy {
			groupSQL, _ := rc.render(g)
			groups = append(groups, groupSQL)
		}
		sql.WriteString(" GROUP BY ")
		sql.WriteString(strings.Join(groups, ", "))
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
		sql.WriteString(" LIMIT ")
		fmt.Fprintf(&sql, "%d", b.limit)
		if b.offset > 0 {
			sql.WriteString(" OFFSET ")
			fmt.Fprintf(&sql, "%d", b.offset)
		}
	}
	if b.lock != lockNone && rc.dialect != DialectSQLite {
		switch b.lock {
		case lockForUpdate:
			sql.WriteString(" FOR UPDATE")
		case lockInShareMode:
			if rc.dialect == DialectPgsql {
				sql.WriteString(" FOR SHARE")
			} else {
				sql.WriteString(" LOCK IN SHARE MODE")
			}
		}
	}
	return sql.String(), rc.args
}

// registerAliases 收集全部表的别名映射。
func (b *SelectBuilder) registerAliases(rc *renderContext) {
	if b.from != nil && b.from.Alias() != "" {
		rc.registerAlias(b.from.TableName(), b.from.Alias())
	}
	for _, j := range b.joins {
		if j.table.Alias() != "" {
			rc.registerAlias(j.table.TableName(), j.table.Alias())
		}
	}
}

// renderTable 渲染表或派生表。
func (b *SelectBuilder) renderTable(rc *renderContext, t Table) string {
	if sub, ok := t.(*SelectBuilder); ok {
		subSQL, _ := sub.renderSelect(rc)
		return "(" + subSQL + ") AS " + sub.alias
	}
	var sql = t.TableName()
	if t.Alias() != "" {
		sql += " AS " + t.Alias()
	}
	return sql
}

// renderWhere 渲染 WHERE 条件（含软删除自动条件）。
func (b *SelectBuilder) renderWhere(rc *renderContext) string {
	conds := b.conditions
	if !b.unscoped && b.from != nil && b.from.Meta() != nil {
		if softField := b.from.Meta().SoftDeleteField(); softField != nil && !containsColumn(b.conditions, softField.ColumnName) {
			conds = append(conds, NewField[any](b.from.TableName(), softField.ColumnName).IsNull())
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

// containsColumn 判断条件树中是否显式引用了指定列（软删除显式接管）。
func containsColumn(conds []Expression, columnName string) bool {
	for _, c := range conds {
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
		for _, c := range v.conds {
			if walkContainsColumn(c, columnName) {
				return true
			}
		}
	}
	return false
}

// resolveDB 返回执行实例（显式绑定优先，否则默认实例）。
func (b *SelectBuilder) resolveDB() (gdb.DB, error) {
	if b.db != nil {
		return b.db, nil
	}
	db, err := gdb.Instance()
	if err != nil {
		return nil, err
	}
	return db, nil
}

// All 执行查询并返回结果集（动态消费，map 形态）；启用缓存时先查缓存。
func (b *SelectBuilder) All() (Result, error) {
	// 分页缓存命中（count 与各页 data 同 key 同生命周期）。
	if b.pageCacheOpt != nil && b.pageNum > 0 {
		if adapter := GetHashCacheAdapter(); adapter != nil {
			key, err := b.pageCacheKey()
			if err != nil {
				return nil, err
			}
			field := fmt.Sprintf("data:%d:%d", b.pageNum, b.pageSize)
			if value, ok, err := adapter.HGet(b.ctx, key, field); err == nil && ok {
				if result, err := unmarshalResult(value); err == nil {
					return result, nil
				}
			}
		}
	}
	// 单查询缓存命中。
	if b.cacheOption != nil {
		if adapter := GetCacheAdapter(); adapter != nil {
			key, err := b.cacheKey()
			if err != nil {
				return nil, err
			}
			if value, ok, err := adapter.Get(b.ctx, key); err == nil && ok {
				if result, err := unmarshalResult(value); err == nil {
					return result, nil
				}
			}
		}
	}
	db, err := b.resolveDB()
	if err != nil {
		return nil, err
	}
	sql, args, err := b.ToSql(dialectOf(db))
	if err != nil {
		return nil, err
	}
	result, err := db.GetAll(b.ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	converted := convertGdbResult(result)
	// 写回缓存（分页优先，其次单查询）。
	if b.pageCacheOpt != nil && b.pageNum > 0 {
		if adapter := GetHashCacheAdapter(); adapter != nil {
			if key, err := b.pageCacheKey(); err == nil {
				if value, err := marshalResult(converted); err == nil {
					_ = adapter.HSet(b.ctx, key, fmt.Sprintf("data:%d:%d", b.pageNum, b.pageSize), value, b.pageCacheOpt.Duration)
				}
			}
		}
	} else if b.cacheOption != nil {
		if adapter := GetCacheAdapter(); adapter != nil {
			if key, err := b.cacheKey(); err == nil {
				if value, err := marshalResult(converted); err == nil {
					_ = adapter.Set(b.ctx, key, value, b.cacheOption.Duration)
				}
			}
		}
	}
	return converted, nil
}

// Scan 执行查询并映射到目标 struct。
func (b *SelectBuilder) Scan(dst any) error {
	db, err := b.resolveDB()
	if err != nil {
		return err
	}
	sql, args, err := b.ToSql(dialectOf(db))
	if err != nil {
		return err
	}
	return db.GetScan(b.ctx, dst, sql, args...)
}

// OneOrNil 执行查询返回单条；空结果返回 (nil, nil)。
func (b *SelectBuilder) OneOrNil() (Record, error) {
	db, err := b.resolveDB()
	if err != nil {
		return nil, err
	}
	sql, args, err := b.ToSql(dialectOf(db))
	if err != nil {
		return nil, err
	}
	all, err := db.GetAll(b.ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, nil
	}
	return convertGdbRecord(all[0]), nil
}

// MustOne 执行查询返回单条；空结果或多条返回明确错误。
func (b *SelectBuilder) MustOne() (Record, error) {
	db, err := b.resolveDB()
	if err != nil {
		return nil, err
	}
	sql, args, err := b.ToSql(dialectOf(db))
	if err != nil {
		return nil, err
	}
	all, err := db.GetAll(b.ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, errors.New("gooq: MustOne got empty result")
	}
	if len(all) > 1 {
		return nil, fmt.Errorf("gooq: MustOne got %d rows, expected exactly 1", len(all))
	}
	return convertGdbRecord(all[0]), nil
}

// Count 执行 COUNT 查询；启用分页缓存时先查缓存。
func (b *SelectBuilder) Count() (int, error) {
	// 分页缓存命中。
	if b.pageCacheOpt != nil {
		if adapter := GetHashCacheAdapter(); adapter != nil {
			key, err := b.pageCacheKey()
			if err != nil {
				return 0, err
			}
			if value, ok, err := adapter.HGet(b.ctx, key, "count"); err == nil && ok {
				return strconv.Atoi(string(value))
			}
		}
	}
	db, err := b.resolveDB()
	if err != nil {
		return 0, err
	}
	rc := newRenderContext(b.ctx, db, dialectOf(db))
	b.registerAliases(rc)
	sql := "SELECT COUNT(*) FROM " + b.renderTable(rc, b.from)
	if where := b.renderWhere(rc); where != "" {
		sql += " WHERE " + where
	}
	count, err := db.GetCount(b.ctx, sql, rc.args...)
	if err != nil {
		return 0, err
	}
	// 写回分页缓存。
	if b.pageCacheOpt != nil {
		if adapter := GetHashCacheAdapter(); adapter != nil {
			if key, err := b.pageCacheKey(); err == nil {
				_ = adapter.HSet(b.ctx, key, "count", []byte(strconv.Itoa(count)), b.pageCacheOpt.Duration)
			}
		}
	}
	return count, nil
}

// convertGdbResult 将 gdb.Result 转换为 gooq.Result。
func convertGdbResult(result gdb.Result) Result {
	converted := make(Result, len(result))
	for i, record := range result {
		converted[i] = convertGdbRecord(record)
	}
	return converted
}

// convertGdbRecord 将 gdb.Record 转换为 gooq.Record。
func convertGdbRecord(record gdb.Record) Record {
	return Record(record)
}

// dialectOf 返回实例的方言（默认 MySQL）。
func dialectOf(db gdb.DB) Dialect {
	if core, ok := db.(interface{ GetConfig() *gdb.ConfigNode }); ok {
		switch core.GetConfig().Type {
		case "pgsql", "postgresql":
			return DialectPgsql
		case "sqlite", "sqlitecgo":
			return DialectSQLite
		}
	}
	return DialectMySQL
}
