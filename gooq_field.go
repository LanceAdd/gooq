// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件实现类型化字段 Field[T] 与字段条件（Eq/Gt/Like 等操作）。
package gooq

import (
	"context"
	"fmt"
	"strings"
)

// Field[T] 是表列的类型化字段，携带所属表名与列名。
// T 仅用于编译期类型标注，运行时不影响渲染。
type Field[T any] struct {
	tableName  string // 所属表名（无表归属时为空串，用于自定义表达式）。
	columnName string // 列名。
	alias      string // 列别名（SELECT 位置渲染为 col AS alias）。
}

// NewField 创建指定表与列的类型化字段。
func NewField[T any](tableName, columnName string) Field[T] {
	return Field[T]{tableName: tableName, columnName: columnName}
}

// TableName 返回字段所属表名。
func (f Field[T]) TableName() string {
	return f.tableName
}

// ColumnName 返回字段列名。
func (f Field[T]) ColumnName() string {
	return f.columnName
}

// As 返回带列别名的字段副本（SELECT 位置渲染为 col AS alias）。
func (f Field[T]) As(alias string) Field[T] {
	f.alias = alias
	return f
}

// Alias 返回字段的列别名。
func (f Field[T]) Alias() string {
	return f.alias
}

// Desc 返回降序排序子句（配合 Order 使用）。
func (f Field[T]) Desc() OrderClause {
	return OrderClause{field: toAnyField(f), desc: true}
}

// Asc 返回升序排序子句（配合 Order 使用）。
func (f Field[T]) Asc() OrderClause {
	return OrderClause{field: toAnyField(f), desc: false}
}

// Condition 实现 Expression 接口：渲染为列引用。
func (f Field[T]) Condition() (string, []any) {
	return f.render(newRenderContext(context.Background(), DialectMySQL))
}

func (f Field[T]) render(rc *renderContext) (string, []any) {
	var sql string
	if prefix := rc.aliasFor(f.tableName); prefix != "" {
		sql = prefix + "." + f.columnName
	} else {
		sql = f.columnName
	}
	if f.alias != "" {
		sql += " AS " + f.alias
	}
	return sql, nil
}

// OrderClause 是排序子句（字段 + 方向 + NULLS 位置）。
type OrderClause struct {
	field Field[any]
	desc  bool
	nulls string // "FIRST" / "LAST"（PG/Oracle 渲染，MySQL 忽略）。
}

// NullsFirst 返回 NULLS FIRST 排序子句（PG/Oracle）。
func (o OrderClause) NullsFirst() OrderClause {
	o.nulls = "FIRST"
	return o
}

// NullsLast 返回 NULLS LAST 排序子句（PG/Oracle）。
func (o OrderClause) NullsLast() OrderClause {
	o.nulls = "LAST"
	return o
}

func (o OrderClause) render(rc *renderContext) (string, []any) {
	sql, _ := o.field.render(rc)
	if o.desc {
		sql += " DESC"
	} else {
		sql += " ASC"
	}
	if o.nulls != "" && rc.dialect != DialectMySQL && rc.dialect != DialectSQLite {
		sql += " NULLS " + o.nulls
	}
	return sql, nil
}

// opType 是字段条件操作符类型。
type opType int

const (
	opEq opType = iota
	opNe
	opGt
	opGte
	opLt
	opLte
	opLike
	opNotLike
	opIn
	opNotIn
	opBetween
	opIsNull
	opIsNotNull
)

// fieldCondition 是字段条件：字段 + 操作符 + 值。
type fieldCondition struct {
	field      Field[any]
	op         opType
	val        any
	vals       []any // IN/BETWEEN 使用。
	subquery   *SelectBuilder
	columnName string
}

// Condition 实现 Expression 接口。
func (c *fieldCondition) Condition() (string, []any) {
	return c.render(newRenderContext(context.Background(), DialectMySQL))
}

func (c *fieldCondition) render(rc *renderContext) (string, []any) {
	fieldSQL, _ := c.field.render(rc)
	switch c.op {
	case opIsNull:
		return fieldSQL + " IS NULL", nil
	case opIsNotNull:
		return fieldSQL + " IS NOT NULL", nil
	case opIn, opNotIn:
		if c.subquery != nil {
			// 子查询渲染自带括号，此处不再包裹。
			subSQL, subArgs := c.subquery.render(rc)
			keyword := "IN"
			if c.op == opNotIn {
				keyword = "NOT IN"
			}
			return fmt.Sprintf(`%s %s %s`, fieldSQL, keyword, subSQL), subArgs
		}
		if len(c.vals) == 0 {
			return "", nil
		}
		var placeholders = make([]string, len(c.vals))
		for i, v := range c.vals {
			placeholders[i] = rc.addArg(v)
		}
		keyword := "IN"
		if c.op == opNotIn {
			keyword = "NOT IN"
		}
		return fmt.Sprintf(`%s %s (%s)`, fieldSQL, keyword, strings.Join(placeholders, ", ")), c.vals
	case opBetween:
		p1 := rc.addArg(c.vals[0])
		p2 := rc.addArg(c.vals[1])
		return fmt.Sprintf(`%s BETWEEN %s AND %s`, fieldSQL, p1, p2), []any{c.vals[0], c.vals[1]}
	default:
		operator := opStrings[c.op]
		// 值为表达式（字段/函数/子查询）时渲染为 SQL 片段而非参数（如 u.id = o.user_id）。
		if expr, ok := c.val.(Expression); ok {
			valSQL, valArgs := rc.render(expr)
			return fmt.Sprintf(`%s %s %s`, fieldSQL, operator, valSQL), valArgs
		}
		placeholder := rc.addArg(c.val)
		return fmt.Sprintf(`%s %s %s`, fieldSQL, operator, placeholder), []any{c.val}
	}
}

// opStrings 是操作符的 SQL 表示。
var opStrings = map[opType]string{
	opEq:      "=",
	opNe:      "!=",
	opGt:      ">",
	opGte:     ">=",
	opLt:      "<",
	opLte:     "<=",
	opLike:    "LIKE",
	opNotLike: "NOT LIKE",
}

// buildFieldCondition 构建字段条件。
func buildFieldCondition(field Field[any], op opType, val any) *fieldCondition {
	return &fieldCondition{field: field, op: op, val: val, columnName: field.columnName}
}

// Eq 返回字段等于值的条件。
func (f Field[T]) Eq(v any) Expression {
	return buildFieldCondition(toAnyField(f), opEq, v)
}

// Ne 返回字段不等于值的条件。
func (f Field[T]) Ne(v any) Expression {
	return buildFieldCondition(toAnyField(f), opNe, v)
}

// Gt 返回字段大于值的条件。
func (f Field[T]) Gt(v any) Expression {
	return buildFieldCondition(toAnyField(f), opGt, v)
}

// Gte 返回字段大于等于值的条件。
func (f Field[T]) Gte(v any) Expression {
	return buildFieldCondition(toAnyField(f), opGte, v)
}

// Lt 返回字段小于值的条件。
func (f Field[T]) Lt(v any) Expression {
	return buildFieldCondition(toAnyField(f), opLt, v)
}

// Lte 返回字段小于等于值的条件。
func (f Field[T]) Lte(v any) Expression {
	return buildFieldCondition(toAnyField(f), opLte, v)
}

// Like 返回字段 LIKE 模式匹配条件。
func (f Field[T]) Like(v any) Expression {
	return buildFieldCondition(toAnyField(f), opLike, v)
}

// NotLike 返回字段 NOT LIKE 模式匹配条件。
func (f Field[T]) NotLike(v any) Expression {
	return buildFieldCondition(toAnyField(f), opNotLike, v)
}

// In 返回字段 IN 值列表条件；参数为子查询时渲染为 IN (SELECT ...)。
func (f Field[T]) In(vals ...any) Expression {
	c := buildFieldCondition(toAnyField(f), opIn, nil)
	if len(vals) == 1 {
		if sub, ok := vals[0].(*SelectBuilder); ok {
			c.subquery = sub
			return c
		}
	}
	c.vals = vals
	return c
}

// NotIn 返回字段 NOT IN 值列表条件。
func (f Field[T]) NotIn(vals ...any) Expression {
	c := buildFieldCondition(toAnyField(f), opNotIn, nil)
	c.vals = vals
	return c
}

// Between 返回字段 BETWEEN 值区间条件。
func (f Field[T]) Between(a, b any) Expression {
	c := buildFieldCondition(toAnyField(f), opBetween, nil)
	c.vals = []any{a, b}
	return c
}

// IsNull 返回字段 IS NULL 条件。
func (f Field[T]) IsNull() Expression {
	return buildFieldCondition(toAnyField(f), opIsNull, nil)
}

// IsNotNull 返回字段 IS NOT NULL 条件。
func (f Field[T]) IsNotNull() Expression {
	return buildFieldCondition(toAnyField(f), opIsNotNull, nil)
}

// Add 返回字段与值的相加表达式（值可为表达式/字面量）。
func (f Field[T]) Add(v any) Expression {
	return &arithExpr{op: arithAdd, left: f, right: v}
}

// Sub 返回字段与值的相减表达式。
func (f Field[T]) Sub(v any) Expression {
	return &arithExpr{op: arithSub, left: f, right: v}
}

// Mul 返回字段与值的相乘表达式。
func (f Field[T]) Mul(v any) Expression {
	return &arithExpr{op: arithMul, left: f, right: v}
}

// Div 返回字段与值的相除表达式。
func (f Field[T]) Div(v any) Expression {
	return &arithExpr{op: arithDiv, left: f, right: v}
}

// toAnyField 将 Field[T] 转换为内部使用的 Field[any]。
func toAnyField[T any](f Field[T]) Field[any] {
	return Field[any]{tableName: f.tableName, columnName: f.columnName, alias: f.alias}
}
