package dsl

import (
	"fmt"
	"sort"

	"github.com/lanceadd/gooq"
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
	doNothing    bool
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
	shardingValue   any
	hasShardValue   bool
	err             error // 构造期错误（如 Values 数量不匹配），ToSql / Exec 返回它。
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
	if b.err != nil {
		return b
	}
	if b.kind != dmlInsert {
		b.err = fmt.Errorf("gooq: Values is only supported on Insert")
		return b
	}
	if len(b.insertColumns) == 0 {
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

// ValuesOf 写入由 model.NewXxxRecord 的 setter 标记的列。
// Insert 接受一行或多行；一行 Update 渲染成 SET；多行 Update/Delete
// 按主键拆成逐行语句，由 Exec 执行。
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

// Master 把这条语句强制路由到主库。
func (b *DMLBuilder) Master() *DMLBuilder {
	b.route = db.RouteMaster
	return b
}

// Slave 把这条语句强制路由到从库；组里没有从库时回落到主库。
func (b *DMLBuilder) Slave() *DMLBuilder {
	b.route = db.RouteSlave
	return b
}

// ShardBy 给这条语句的主表设置分片值。nil 也算设置过。
// Insert、Update、Delete 的主表是被写入或删除的那一张。
func (b *DMLBuilder) ShardBy(value any) *DMLBuilder {
	b.shardingValue = value
	b.hasShardValue = true
	return b
}

// DB 选择执行时使用的配置组。
// 名字为空时使用默认组，等同于不调用 DB。
func (b *DMLBuilder) DB(name string) *DMLBuilder {
	b.group = name
	b.groupSet = name != ""
	return b
}
