package gooq

import (
	"fmt"
	"strings"
)

type Field[T any] struct {
	table      *TableBase // 所属表实例（As/Clone 时绑定；nil 时回退 tableName 前缀）。
	tableName  string     // 渲染前缀表名（无指针绑定时使用）。
	columnName string     // 列名。
	alias      string     // 列别名（SELECT 位置渲染为 col AS alias）。
}

func NewField[T any](tableName, columnName string) Field[T] {
	return Field[T]{tableName: tableName, columnName: columnName}
}

func NewFieldAt[T any](t *TableBase, column string) Field[T] {
	return Field[T]{table: t, tableName: t.meta.TableName, columnName: column}
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
	return f.render(newRenderContext(DialectMySQL))
}

func (f Field[T]) render(rc *renderContext) (string, []any) {
	var sql string
	if parts := f.prefixParts(); len(parts) > 0 && parts[0] != "" {
		var quoted []string
		for _, p := range parts {
			quoted = append(quoted, rc.quote(p))
		}
		sql = strings.Join(quoted, ".") + "." + rc.quote(f.columnName)
	} else {
		sql = rc.quote(f.columnName)
	}
	if f.alias != "" {
		sql += " AS " + f.alias
	}
	return sql, nil
}

func (f Field[T]) prefixParts() []string {
	if f.table != nil {
		if alias := f.table.Alias(); alias != "" {
			return []string{alias}
		}
		if schema := f.table.Meta().Schema; schema != "" {
			return []string{schema, f.tableName}
		}
	}
	return strings.Split(f.tableName, ".")
}

type OrderClause struct {
	field Field[any]
	desc  bool
	nulls string // "FIRST" / "LAST"（PG 渲染，MySQL/SQLite 忽略）。
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
	value      any
	values     []any // IN/BETWEEN 使用。
	subquery   *SelectBuilder
	columnName string
}

func (c *fieldCondition) Condition() (string, []any) {
	return c.render(newRenderContext(DialectMySQL))
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
		if len(c.values) == 0 {
			return "", nil
		}
		var placeholders = make([]string, len(c.values))
		for i, v := range c.values {
			placeholders[i] = rc.addArg(v)
		}
		keyword := "IN"
		if c.op == opNotIn {
			keyword = "NOT IN"
		}
		return fmt.Sprintf(`%s %s (%s)`, fieldSQL, keyword, strings.Join(placeholders, ", ")), c.values
	case opBetween:
		p1SQL, p1Args := rc.renderValue(c.values[0])
		p2SQL, p2Args := rc.renderValue(c.values[1])
		return fmt.Sprintf(`%s BETWEEN %s AND %s`, fieldSQL, p1SQL, p2SQL), append(p1Args, p2Args...)
	default:
		operator := opStrings[c.op]
		// 值为表达式（字段/函数/子查询）时渲染为 SQL 片段而非参数（如 u.id = o.user_id）。
		if expr, ok := c.value.(Expression); ok {
			valSQL, valArgs := rc.render(expr)
			return fmt.Sprintf(`%s %s %s`, fieldSQL, operator, valSQL), valArgs
		}
		placeholder := rc.addArg(c.value)
		return fmt.Sprintf(`%s %s %s`, fieldSQL, operator, placeholder), []any{c.value}
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

func buildFieldCondition(field Field[any], op opType, value any) *fieldCondition {
	return &fieldCondition{field: field, op: op, value: value, columnName: field.columnName}
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

func (f Field[T]) In(values ...T) Expression {
	c := buildFieldCondition(toAnyField(f), opIn, nil)
	c.values = toAnySlice(values)
	return c
}

func (f Field[T]) InExpr(subquery *SelectBuilder) Expression {
	c := buildFieldCondition(toAnyField(f), opIn, nil)
	c.subquery = subquery
	return c
}

func (f Field[T]) NotIn(values ...T) Expression {
	c := buildFieldCondition(toAnyField(f), opNotIn, nil)
	c.values = toAnySlice(values)
	return c
}

func (f Field[T]) NotInExpr(subquery *SelectBuilder) Expression {
	c := buildFieldCondition(toAnyField(f), opNotIn, nil)
	c.subquery = subquery
	return c
}

func (f Field[T]) Between(a, b T) Expression {
	c := buildFieldCondition(toAnyField(f), opBetween, nil)
	c.values = []any{a, b}
	return c
}

func (f Field[T]) BetweenExpr(a, b Expression) Expression {
	c := buildFieldCondition(toAnyField(f), opBetween, nil)
	c.values = []any{a, b}
	return c
}

func (f Field[T]) IsNull() Expression {
	return buildFieldCondition(toAnyField(f), opIsNull, nil)
}

func (f Field[T]) IsNotNull() Expression {
	return buildFieldCondition(toAnyField(f), opIsNotNull, nil)
}

func (f Field[T]) Cast(localType LocalType) Expression {
	return &castExpr{expr: f, localType: localType}
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
	return Field[any]{table: f.table, tableName: f.tableName, columnName: f.columnName, alias: f.alias}
}

func toAnySlice[T any](values []T) []any {
	result := make([]any, len(values))
	for i, v := range values {
		result[i] = v
	}
	return result
}
