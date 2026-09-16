package gooq

import (
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
	return e.Render(NewRenderContext(DialectMySQL))
}

func (e *caseExpr) Render(rc *RenderContext) (string, []any) {
	var (
		parts  []string
		allArg []any
	)
	for _, w := range e.builder.whens {
		whenSQL, whenArgs := rc.Render(w.when)
		thenSQL, thenArgs := rc.RenderValue(w.then)
		parts = append(parts, "WHEN "+whenSQL+" THEN "+thenSQL)
		allArg = append(allArg, whenArgs...)
		allArg = append(allArg, thenArgs...)
	}
	if e.builder.elseVal != nil {
		elseSQL, elseArgs := rc.RenderValue(e.builder.elseVal)
		parts = append(parts, "ELSE "+elseSQL)
		allArg = append(allArg, elseArgs...)
	}
	sql := "CASE " + strings.Join(parts, " ") + " END"
	if e.alias != "" {
		sql += " AS " + e.alias
	}
	return sql, allArg
}

func (e *caseExpr) SubExpressions() []Expression {
	var subs []Expression
	for _, w := range e.builder.whens {
		subs = append(subs, w.when)
		if thenExpr, ok := w.then.(Expression); ok {
			subs = append(subs, thenExpr)
		}
	}
	if elseExpr, ok := e.builder.elseVal.(Expression); ok {
		subs = append(subs, elseExpr)
	}
	return subs
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
	return a.Render(NewRenderContext(DialectMySQL))
}

func (a *arithExpr) Render(rc *RenderContext) (string, []any) {
	leftSQL, leftArgs := rc.RenderValue(a.left)
	rightSQL, rightArgs := rc.RenderValue(a.right)
	return fmt.Sprintf(
		"(%s %s %s)",
		leftSQL, arithOperator[a.op], rightSQL,
	), append(leftArgs, rightArgs...)
}

func (a *arithExpr) SubExpressions() []Expression {
	var subs []Expression
	if left, ok := a.left.(Expression); ok {
		subs = append(subs, left)
	}
	if right, ok := a.right.(Expression); ok {
		subs = append(subs, right)
	}
	return subs
}

func Negate(e Expression) Expression {
	return &negateExpr{expr: e}
}

type negateExpr struct {
	expr Expression
}

func (n *negateExpr) Condition() (string, []any) {
	return n.Render(NewRenderContext(DialectMySQL))
}

func (n *negateExpr) Render(rc *RenderContext) (string, []any) {
	sql, args := rc.Render(n.expr)
	return "(-" + sql + ")", args
}

func (n *negateExpr) SubExpressions() []Expression {
	return []Expression{n.expr}
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
	return d.Render(NewRenderContext(DialectMySQL))
}

func (d *distinctExpr) Render(rc *RenderContext) (string, []any) {
	sql, args := rc.Render(d.expr)
	return "DISTINCT " + sql, args
}

func (d *distinctExpr) SubExpressions() []Expression {
	return []Expression{d.expr}
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
	return c.Render(NewRenderContext(DialectMySQL))
}

func (c *castExpr) Render(rc *RenderContext) (string, []any) {
	sql, args := rc.Render(c.expr)
	return fmt.Sprintf("CAST(%s AS %s)", sql, castTypeOf(rc, c.localType)), args
}

func (c *castExpr) SubExpressions() []Expression {
	return []Expression{c.expr}
}

// Cast 将表达式转换为目标类型（类型名按方言映射）。
func Cast(e Expression, t LocalType) Expression {
	return &castExpr{expr: e, localType: t}
}
