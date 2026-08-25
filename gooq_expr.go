package gooq

import (
	"context"
	"fmt"
	"strings"
)

type whenThenClause struct {
	when Expression
	then any
}

type CaseBuilder struct {
	whens   []whenThenClause
	elseVal any
}

func Case() *CaseBuilder {
	return &CaseBuilder{}
}

func (c *CaseBuilder) When(cond Expression) *CaseThen {
	return &CaseThen{parent: c, when: cond}
}

func (c *CaseBuilder) Else(val any) *CaseBuilder {
	c.elseVal = val
	return c
}

func (c *CaseBuilder) End() *caseExpr {
	return &caseExpr{builder: c}
}

type CaseThen struct {
	parent *CaseBuilder
	when   Expression
}

func (t *CaseThen) Then(val any) *CaseBuilder {
	t.parent.whens = append(t.parent.whens, whenThenClause{when: t.when, then: val})
	return t.parent
}

type caseExpr struct {
	builder *CaseBuilder
	alias   string
}

func (e *caseExpr) As(alias string) Expression {
	e.alias = alias
	return e
}

func (e *caseExpr) Condition() (string, []any) {
	return e.render(newRenderContext(context.Background(), DialectMySQL))
}

func (e *caseExpr) render(rc *renderContext) (string, []any) {
	var (
		parts  []string
		allArg []any
	)
	for _, w := range e.builder.whens {
		whenSQL, whenArgs := rc.render(w.when)
		thenSQL, thenArgs := rc.renderValue(w.then)
		parts = append(parts, "WHEN "+whenSQL+" THEN "+thenSQL)
		allArg = append(allArg, whenArgs...)
		allArg = append(allArg, thenArgs...)
	}
	if e.builder.elseVal != nil {
		elseSQL, elseArgs := rc.renderValue(e.builder.elseVal)
		parts = append(parts, "ELSE "+elseSQL)
		allArg = append(allArg, elseArgs...)
	}
	sql := "CASE " + strings.Join(parts, " ") + " END"
	if e.alias != "" {
		sql += " AS " + e.alias
	}
	return sql, allArg
}

func (rc *renderContext) renderValue(v any) (string, []any) {
	if expr, ok := v.(Expression); ok {
		return rc.render(expr)
	}
	return rc.addArg(v), []any{v}
}

type arithOp int

const (
	arithAdd arithOp = iota
	arithSub
	arithMul
	arithDiv
)

var arithOperator = map[arithOp]string{
	arithAdd: "+",
	arithSub: "-",
	arithMul: "*",
	arithDiv: "/",
}

type arithExpr struct {
	op          arithOp
	left, right any
}

func (a *arithExpr) Condition() (string, []any) {
	return a.render(newRenderContext(context.Background(), DialectMySQL))
}

func (a *arithExpr) render(rc *renderContext) (string, []any) {
	leftSQL, leftArgs := rc.renderValue(a.left)
	rightSQL, rightArgs := rc.renderValue(a.right)
	return fmt.Sprintf(
		"(%s %s %s)",
		leftSQL, arithOperator[a.op], rightSQL,
	), append(leftArgs, rightArgs...)
}

func Negate(e Expression) Expression {
	return &negateExpr{expr: e}
}

type negateExpr struct {
	expr Expression
}

func (n *negateExpr) Condition() (string, []any) {
	return n.render(newRenderContext(context.Background(), DialectMySQL))
}

func (n *negateExpr) render(rc *renderContext) (string, []any) {
	sql, args := rc.render(n.expr)
	return "(-" + sql + ")", args
}

func Add(left, right any) Expression {
	return &arithExpr{op: arithAdd, left: left, right: right}
}

func Sub(left, right any) Expression {
	return &arithExpr{op: arithSub, left: left, right: right}
}

func Mul(left, right any) Expression {
	return &arithExpr{op: arithMul, left: left, right: right}
}

func Div(left, right any) Expression {
	return &arithExpr{op: arithDiv, left: left, right: right}
}

func Str(s string) Expression {
	return &rawExpr{sql: "'" + s + "'"}
}

type distinctExpr struct {
	expr Expression
}

func (d *distinctExpr) Condition() (string, []any) {
	return d.render(newRenderContext(context.Background(), DialectMySQL))
}

func (d *distinctExpr) render(rc *renderContext) (string, []any) {
	sql, args := rc.render(d.expr)
	return "DISTINCT " + sql, args
}

// Distinct 包装表达式为 DISTINCT（如 COUNT(DISTINCT field)）。
func Distinct(e Expression) Expression {
	return &distinctExpr{expr: e}
}

type castExpr struct {
	expr      Expression
	localType LocalType
}

func (c *castExpr) Condition() (string, []any) {
	return c.render(newRenderContext(context.Background(), DialectMySQL))
}

func (c *castExpr) render(rc *renderContext) (string, []any) {
	sql, args := rc.render(c.expr)
	return fmt.Sprintf("CAST(%s AS %s)", sql, castTypeOf(rc, c.localType)), args
}

// Cast 将表达式转换为目标类型（类型名按方言映射）。
func Cast(e Expression, t LocalType) Expression {
	return &castExpr{expr: e, localType: t}
}
