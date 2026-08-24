package gooq

import (
	"context"
	"fmt"
	"strings"
)

type Field[T any] struct {
	tableName  string // 所属表名（无表归属时为空串，用于自定义表达式）。
	columnName string // 列名。
	alias      string // 列别名（SELECT 位置渲染为 col AS alias）。
}

func NewField[T any](tableName, columnName string) Field[T] {
	return Field[T]{tableName: tableName, columnName: columnName}
}

func (f Field[T]) TableName() string {
	return f.tableName
}

func (f Field[T]) ColumnName() string {
	return f.columnName
}

func (f Field[T]) As(alias string) Field[T] {
	f.alias = alias
	return f
}

func (f Field[T]) Alias() string {
	return f.alias
}

func (f Field[T]) Desc() OrderClause {
	return OrderClause{field: toAnyField(f), desc: true}
}

func (f Field[T]) Asc() OrderClause {
	return OrderClause{field: toAnyField(f), desc: false}
}

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

type OrderClause struct {
	field Field[any]
	desc  bool
	nulls string // "FIRST" / "LAST"（PG/Oracle 渲染，MySQL 忽略）。
}

func (o OrderClause) NullsFirst() OrderClause {
	o.nulls = "FIRST"
	return o
}

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

type fieldCondition struct {
	field      Field[any]
	op         opType
	val        any
	vals       []any // IN/BETWEEN 使用。
	subquery   *SelectBuilder
	columnName string
}

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
		p1SQL, p1Args := rc.renderValue(c.vals[0])
		p2SQL, p2Args := rc.renderValue(c.vals[1])
		return fmt.Sprintf(`%s BETWEEN %s AND %s`, fieldSQL, p1SQL, p2SQL), append(p1Args, p2Args...)
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

func buildFieldCondition(field Field[any], op opType, val any) *fieldCondition {
	return &fieldCondition{field: field, op: op, val: val, columnName: field.columnName}
}

func (f Field[T]) Eq(v T) Expression {
	return buildFieldCondition(toAnyField(f), opEq, v)
}

func (f Field[T]) EqExpr(e Expression) Expression {
	return buildFieldCondition(toAnyField(f), opEq, e)
}

func (f Field[T]) Ne(v T) Expression {
	return buildFieldCondition(toAnyField(f), opNe, v)
}

func (f Field[T]) NeExpr(e Expression) Expression {
	return buildFieldCondition(toAnyField(f), opNe, e)
}

func (f Field[T]) Gt(v T) Expression {
	return buildFieldCondition(toAnyField(f), opGt, v)
}

func (f Field[T]) GtExpr(e Expression) Expression {
	return buildFieldCondition(toAnyField(f), opGt, e)
}

func (f Field[T]) Gte(v T) Expression {
	return buildFieldCondition(toAnyField(f), opGte, v)
}

func (f Field[T]) GteExpr(e Expression) Expression {
	return buildFieldCondition(toAnyField(f), opGte, e)
}

func (f Field[T]) Lt(v T) Expression {
	return buildFieldCondition(toAnyField(f), opLt, v)
}

func (f Field[T]) LtExpr(e Expression) Expression {
	return buildFieldCondition(toAnyField(f), opLt, e)
}

func (f Field[T]) Lte(v T) Expression {
	return buildFieldCondition(toAnyField(f), opLte, v)
}

func (f Field[T]) LteExpr(e Expression) Expression {
	return buildFieldCondition(toAnyField(f), opLte, e)
}

func (f Field[T]) Like(v T) Expression {
	return buildFieldCondition(toAnyField(f), opLike, v)
}

func (f Field[T]) LikeExpr(e Expression) Expression {
	return buildFieldCondition(toAnyField(f), opLike, e)
}

func (f Field[T]) NotLike(v T) Expression {
	return buildFieldCondition(toAnyField(f), opNotLike, v)
}

func (f Field[T]) NotLikeExpr(e Expression) Expression {
	return buildFieldCondition(toAnyField(f), opNotLike, e)
}

func (f Field[T]) In(vals ...T) Expression {
	c := buildFieldCondition(toAnyField(f), opIn, nil)
	c.vals = toAnySlice(vals)
	return c
}

func (f Field[T]) InExpr(sub *SelectBuilder) Expression {
	c := buildFieldCondition(toAnyField(f), opIn, nil)
	c.subquery = sub
	return c
}

func (f Field[T]) NotIn(vals ...T) Expression {
	c := buildFieldCondition(toAnyField(f), opNotIn, nil)
	c.vals = toAnySlice(vals)
	return c
}

func (f Field[T]) NotInExpr(sub *SelectBuilder) Expression {
	c := buildFieldCondition(toAnyField(f), opNotIn, nil)
	c.subquery = sub
	return c
}

func (f Field[T]) Between(a, b T) Expression {
	c := buildFieldCondition(toAnyField(f), opBetween, nil)
	c.vals = []any{a, b}
	return c
}

func (f Field[T]) BetweenExpr(a, b Expression) Expression {
	c := buildFieldCondition(toAnyField(f), opBetween, nil)
	c.vals = []any{a, b}
	return c
}

func (f Field[T]) IsNull() Expression {
	return buildFieldCondition(toAnyField(f), opIsNull, nil)
}

func (f Field[T]) IsNotNull() Expression {
	return buildFieldCondition(toAnyField(f), opIsNotNull, nil)
}

func (f Field[T]) Add(v T) Expression {
	return &arithExpr{op: arithAdd, left: f, right: v}
}

func (f Field[T]) Sub(v T) Expression {
	return &arithExpr{op: arithSub, left: f, right: v}
}

func (f Field[T]) Mul(v T) Expression {
	return &arithExpr{op: arithMul, left: f, right: v}
}

func (f Field[T]) Div(v T) Expression {
	return &arithExpr{op: arithDiv, left: f, right: v}
}

func toAnyField[T any](f Field[T]) Field[any] {
	return Field[any]{tableName: f.tableName, columnName: f.columnName, alias: f.alias}
}

func toAnySlice[T any](vals []T) []any {
	result := make([]any, len(vals))
	for i, v := range vals {
		result[i] = v
	}
	return result
}
