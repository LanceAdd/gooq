// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件实现 CASE WHEN 条件表达式与算术表达式（Add/Sub/Mul/Div/Negate）。
package gooq

import (
	"context"
	"fmt"
	"strings"
)

// whenThenClause 是 CASE WHEN 的单条分支。
type whenThenClause struct {
	when Expression
	then any
}

// CaseBuilder 是 CASE WHEN 表达式构建器。
type CaseBuilder struct {
	whens   []whenThenClause
	elseVal any
}

// Case 创建 CASE WHEN 表达式构建器。
func Case() *CaseBuilder {
	return &CaseBuilder{}
}

// When 设置 WHEN 条件分支（随后调用 Then）。
func (c *CaseBuilder) When(cond Expression) *CaseThen {
	return &CaseThen{parent: c, when: cond}
}

// Else 设置 ELSE 分支值。
func (c *CaseBuilder) Else(val any) *CaseBuilder {
	c.elseVal = val
	return c
}

// End 结束构建并返回 CASE 表达式。
func (c *CaseBuilder) End() *caseExpr {
	return &caseExpr{builder: c}
}

// CaseThen 是 WHEN 分支的 Then 步骤。
type CaseThen struct {
	parent *CaseBuilder
	when   Expression
}

// Then 设置该分支的返回值并回到构建器。
func (t *CaseThen) Then(val any) *CaseBuilder {
	t.parent.whens = append(t.parent.whens, whenThenClause{when: t.when, then: val})
	return t.parent
}

// caseExpr 是 CASE WHEN 表达式节点。
type caseExpr struct {
	builder *CaseBuilder
	alias   string
}

// As 返回带别名的 CASE 表达式（SELECT 位置渲染为 CASE ... END AS alias）。
func (e *caseExpr) As(alias string) Expression {
	e.alias = alias
	return e
}

// Condition 实现 Expression 接口。
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

// renderValue 渲染值位置：Expression 渲染为表达式，其余参数化。
func (rc *renderContext) renderValue(v any) (string, []any) {
	if expr, ok := v.(Expression); ok {
		return rc.render(expr)
	}
	return rc.addArg(v), []any{v}
}

// arithOp 是算术操作符。
type arithOp int

const (
	arithAdd arithOp = iota
	arithSub
	arithMul
	arithDiv
)

// arithOperator 是算术操作符的 SQL 表示。
var arithOperator = map[arithOp]string{
	arithAdd: "+",
	arithSub: "-",
	arithMul: "*",
	arithDiv: "/",
}

// arithExpr 是算术表达式节点（左操作数 op 右操作数）。
type arithExpr struct {
	op          arithOp
	left, right any
}

// Condition 实现 Expression 接口。
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

// Negate 返回一元取负表达式 (-expr)。
func Negate(e Expression) Expression {
	return &negateExpr{expr: e}
}

// negateExpr 是一元取负表达式节点。
type negateExpr struct {
	expr Expression
}

// Condition 实现 Expression 接口。
func (n *negateExpr) Condition() (string, []any) {
	return n.render(newRenderContext(context.Background(), DialectMySQL))
}

func (n *negateExpr) render(rc *renderContext) (string, []any) {
	sql, args := rc.render(n.expr)
	return "(-" + sql + ")", args
}

// Add 返回两值相加表达式（值可为字段/表达式/字面量）。
func Add(left, right any) Expression {
	return &arithExpr{op: arithAdd, left: left, right: right}
}

// Sub 返回两值相减表达式。
func Sub(left, right any) Expression {
	return &arithExpr{op: arithSub, left: left, right: right}
}

// Mul 返回两值相乘表达式。
func Mul(left, right any) Expression {
	return &arithExpr{op: arithMul, left: left, right: right}
}

// Div 返回两值相除表达式。
func Div(left, right any) Expression {
	return &arithExpr{op: arithDiv, left: left, right: right}
}
