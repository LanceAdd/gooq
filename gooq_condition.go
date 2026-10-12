package gooq

import (
	"fmt"
	"strings"
)

// groupOp 是条件组操作符。
type groupOp int

const (
	groupAnd groupOp = iota
	groupOr
	groupNot
)

type groupCondition struct {
	op         groupOp
	conditions []Expression
}

func (g *groupCondition) Render(rc *RenderContext) string {
	parts := make([]string, 0, len(g.conditions))
	for _, condition := range g.conditions {
		if condition == nil {
			continue
		}
		sql := rc.Render(condition)
		if sql == "" {
			continue
		}
		parts = append(parts, sql)
	}
	if len(parts) == 0 {
		return ""
	}
	switch g.op {
	case groupAnd:
		return "(" + strings.Join(parts, " AND ") + ")"
	case groupOr:
		return "(" + strings.Join(parts, " OR ") + ")"
	case groupNot:
		return "(NOT " + parts[0] + ")"
	default:
		return ""
	}
}

func (g *groupCondition) SubExpressions() []Expression {
	return g.conditions
}

func AND(conditions ...Expression) Expression {
	return &groupCondition{op: groupAnd, conditions: conditions}
}

func OR(conditions ...Expression) Expression {
	return &groupCondition{op: groupOr, conditions: conditions}
}

func NOT(condition Expression) Expression {
	return &groupCondition{op: groupNot, conditions: []Expression{condition}}
}

type rawExpr struct {
	sql  string
	args []any
}

func (r *rawExpr) Render(rc *RenderContext) string {
	rc.args = append(rc.args, r.args...)
	rc.argIndex += len(r.args)
	return r.sql
}

// Raw 原样拼出 SQL 并追加参数，不改写占位符。
// MySQL / SQLite 写 ?。PostgreSQL 不会把 ? 改成 $n，片段里要自己写 $n，并和周围已渲染的序号对齐。
func Raw(sql string, args ...any) Expression {
	return &rawExpr{sql: sql, args: args}
}

type existsCondition struct {
	subquery Subquery
	negate   bool
}

func (e *existsCondition) Render(rc *RenderContext) string {
	subSQL := e.subquery.Render(rc)
	if e.negate {
		return "NOT EXISTS " + subSQL
	}
	return "EXISTS " + subSQL
}

func (e *existsCondition) SubExpressions() []Expression {
	return []Expression{e.subquery}
}

func Exists(subquery Subquery) Expression {
	return &existsCondition{subquery: subquery}
}

func NotExists(subquery Subquery) Expression {
	return &existsCondition{subquery: subquery, negate: true}
}

type exprCondition struct {
	left  Expression
	op    opType
	value any
}

func (c *exprCondition) Render(rc *RenderContext) string {
	leftSQL := rc.Render(c.left)
	switch c.op {
	case opIsNull:
		return leftSQL + " IS NULL"
	case opIsNotNull:
		return leftSQL + " IS NOT NULL"
	default:
		operator := opStrings[c.op]
		if expr, ok := c.value.(Expression); ok {
			return fmt.Sprintf(`%s %s %s`, leftSQL, operator, rc.Render(expr))
		}
		return fmt.Sprintf(`%s %s %s`, leftSQL, operator, rc.AddArg(c.value))
	}
}

func (c *exprCondition) SubExpressions() []Expression {
	subs := []Expression{c.left}
	if expr, ok := c.value.(Expression); ok {
		subs = append(subs, expr)
	}
	return subs
}

func Eq(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opEq, value: v}
}

func Ne(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opNe, value: v}
}

func Gt(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opGt, value: v}
}

func Gte(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opGte, value: v}
}

func Lt(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opLt, value: v}
}

func Lte(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opLte, value: v}
}

func Like(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opLike, value: v}
}

func IsNull(e Expression) Expression {
	return &exprCondition{left: e, op: opIsNull}
}

func IsNotNull(e Expression) Expression {
	return &exprCondition{left: e, op: opIsNotNull}
}
