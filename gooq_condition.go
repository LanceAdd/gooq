package gooq

import (
	"fmt"
	"strings"
)

// Condition 是可以继续 AND / OR / NOT 的布尔表达式。它仍是 Expression，可直接传入 Where、On、Having。
// 链式调用左结合：a.Or(b).And(c) 得到 ((a OR b) AND c)。要先组合右边时，把右边自己写成一组再传入。
type Condition interface {
	Expression
	And(parts ...Expression) Condition
	Or(parts ...Expression) Condition
	Not() Condition
}

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

func (g *groupCondition) And(parts ...Expression) Condition { return chain(groupAnd, g, parts...) }
func (g *groupCondition) Or(parts ...Expression) Condition  { return chain(groupOr, g, parts...) }
func (g *groupCondition) Not() Condition                    { return negate(g) }

func chain(op groupOp, head Expression, rest ...Expression) Condition {
	parts := make([]Expression, 0, 1+len(rest))
	parts = append(parts, head)
	parts = append(parts, rest...)
	return &groupCondition{op: op, conditions: parts}
}

func negate(condition Expression) Condition {
	return &groupCondition{op: groupNot, conditions: []Expression{condition}}
}

func AND(conditions ...Expression) Condition {
	return &groupCondition{op: groupAnd, conditions: conditions}
}

func OR(conditions ...Expression) Condition {
	return &groupCondition{op: groupOr, conditions: conditions}
}

func NOT(condition Expression) Condition {
	return negate(condition)
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

func (e *existsCondition) And(parts ...Expression) Condition { return chain(groupAnd, e, parts...) }
func (e *existsCondition) Or(parts ...Expression) Condition  { return chain(groupOr, e, parts...) }
func (e *existsCondition) Not() Condition                    { return negate(e) }

func Exists(subquery Subquery) Condition {
	return &existsCondition{subquery: subquery}
}

func NotExists(subquery Subquery) Condition {
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

func (c *exprCondition) And(parts ...Expression) Condition { return chain(groupAnd, c, parts...) }
func (c *exprCondition) Or(parts ...Expression) Condition  { return chain(groupOr, c, parts...) }
func (c *exprCondition) Not() Condition                    { return negate(c) }

func Eq(e Expression, v any) Condition {
	return &exprCondition{left: e, op: opEq, value: v}
}

func Ne(e Expression, v any) Condition {
	return &exprCondition{left: e, op: opNe, value: v}
}

func Gt(e Expression, v any) Condition {
	return &exprCondition{left: e, op: opGt, value: v}
}

func Gte(e Expression, v any) Condition {
	return &exprCondition{left: e, op: opGte, value: v}
}

func Lt(e Expression, v any) Condition {
	return &exprCondition{left: e, op: opLt, value: v}
}

func Lte(e Expression, v any) Condition {
	return &exprCondition{left: e, op: opLte, value: v}
}

func Like(e Expression, v any) Condition {
	return &exprCondition{left: e, op: opLike, value: v}
}

func IsNull(e Expression) Condition {
	return &exprCondition{left: e, op: opIsNull}
}

func IsNotNull(e Expression) Condition {
	return &exprCondition{left: e, op: opIsNotNull}
}

// wrappedCondition 让 Raw 等普通表达式也能继续 And / Or / Not。
type wrappedCondition struct {
	expr Expression
}

func (w *wrappedCondition) Render(rc *RenderContext) string {
	if w == nil || w.expr == nil {
		return ""
	}
	return rc.Render(w.expr)
}

func (w *wrappedCondition) SubExpressions() []Expression {
	if w == nil || w.expr == nil {
		return nil
	}
	return []Expression{w.expr}
}

func (w *wrappedCondition) And(parts ...Expression) Condition { return chain(groupAnd, w, parts...) }
func (w *wrappedCondition) Or(parts ...Expression) Condition  { return chain(groupOr, w, parts...) }
func (w *wrappedCondition) Not() Condition                    { return negate(w) }

// Cond 把表达式变成可链式组合的条件。已经是 Condition 时原样返回。nil 返回 nil。
func Cond(e Expression) Condition {
	if e == nil {
		return nil
	}
	if c, ok := e.(Condition); ok {
		return c
	}
	return &wrappedCondition{expr: e}
}

var (
	_ Condition = (*fieldCondition)(nil)
	_ Condition = (*exprCondition)(nil)
	_ Condition = (*groupCondition)(nil)
	_ Condition = (*existsCondition)(nil)
	_ Condition = (*wrappedCondition)(nil)
)
