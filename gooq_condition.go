package gooq

import (
	"context"
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
	op    groupOp
	conds []Expression
}

func (g *groupCondition) Condition() (string, []any) {
	return g.render(newRenderContext(context.Background(), DialectMySQL))
}

func (g *groupCondition) render(rc *renderContext) (string, []any) {
	var (
		parts   = make([]string, 0, len(g.conds))
		allArgs = make([]any, 0)
	)
	for _, cond := range g.conds {
		if cond == nil {
			continue
		}
		sql, args := rc.render(cond)
		if sql == "" {
			continue
		}
		parts = append(parts, sql)
		allArgs = append(allArgs, args...)
	}
	if len(parts) == 0 {
		return "", nil
	}
	switch g.op {
	case groupAnd:
		return "(" + strings.Join(parts, " AND ") + ")", allArgs
	case groupOr:
		return "(" + strings.Join(parts, " OR ") + ")", allArgs
	case groupNot:
		return "(NOT " + parts[0] + ")", allArgs
	default:
		return "", nil
	}
}

func AND(conds ...Expression) Expression {
	return &groupCondition{op: groupAnd, conds: conds}
}

func OR(conds ...Expression) Expression {
	return &groupCondition{op: groupOr, conds: conds}
}

func NOT(cond Expression) Expression {
	return &groupCondition{op: groupNot, conds: []Expression{cond}}
}

type rawExpr struct {
	sql  string
	args []any
}

func (r *rawExpr) Condition() (string, []any) {
	return r.sql, r.args
}

func (r *rawExpr) render(rc *renderContext) (string, []any) {
	rc.args = append(rc.args, r.args...)
	rc.argIndex += len(r.args)
	return r.sql, r.args
}

func Raw(sql string, args ...any) Expression {
	return &rawExpr{sql: sql, args: args}
}

type existsCondition struct {
	subquery *SelectBuilder
	negate   bool
}

func (e *existsCondition) Condition() (string, []any) {
	return e.render(newRenderContext(context.Background(), DialectMySQL))
}

func (e *existsCondition) render(rc *renderContext) (string, []any) {
	subSQL, subArgs := e.subquery.render(rc)
	if e.negate {
		return "NOT EXISTS " + subSQL, subArgs
	}
	return "EXISTS " + subSQL, subArgs
}

func Exists(subquery *SelectBuilder) Expression {
	return &existsCondition{subquery: subquery}
}

func NotExists(subquery *SelectBuilder) Expression {
	return &existsCondition{subquery: subquery, negate: true}
}

type exprCondition struct {
	left Expression
	op   opType
	val  any
}

func (c *exprCondition) Condition() (string, []any) {
	return c.render(newRenderContext(context.Background(), DialectMySQL))
}

func (c *exprCondition) render(rc *renderContext) (string, []any) {
	leftSQL, leftArgs := rc.render(c.left)
	switch c.op {
	case opIsNull:
		return leftSQL + " IS NULL", leftArgs
	case opIsNotNull:
		return leftSQL + " IS NOT NULL", leftArgs
	default:
		operator := opStrings[c.op]
		if expr, ok := c.val.(Expression); ok {
			valSQL, valArgs := rc.render(expr)
			return fmt.Sprintf(`%s %s %s`, leftSQL, operator, valSQL), append(leftArgs, valArgs...)
		}
		placeholder := rc.addArg(c.val)
		return fmt.Sprintf(`%s %s %s`, leftSQL, operator, placeholder), append(leftArgs, c.val)
	}
}

func Eq(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opEq, val: v}
}

func Ne(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opNe, val: v}
}

func Gt(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opGt, val: v}
}

func Gte(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opGte, val: v}
}

func Lt(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opLt, val: v}
}

func Lte(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opLte, val: v}
}

func Like(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opLike, val: v}
}

func IsNull(e Expression) Expression {
	return &exprCondition{left: e, op: opIsNull}
}

func IsNotNull(e Expression) Expression {
	return &exprCondition{left: e, op: opIsNotNull}
}
