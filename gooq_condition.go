// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件实现条件组合（AND/OR/NOT）与结构化 Raw 表达式。
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

// groupCondition 是条件组合节点：AND/OR 组合多个条件，NOT 取反。
type groupCondition struct {
	op    groupOp
	conds []Expression
}

// Condition 实现 Expression 接口。
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

// AND 组合多个条件（默认 AND 连接）；无参数或全空时返回空条件。
func AND(conds ...Expression) Expression {
	return &groupCondition{op: groupAnd, conds: conds}
}

// OR 组合多个条件（OR 连接）。
func OR(conds ...Expression) Expression {
	return &groupCondition{op: groupOr, conds: conds}
}

// NOT 取反单个条件。
func NOT(cond Expression) Expression {
	return &groupCondition{op: groupNot, conds: []Expression{cond}}
}

// rawExpr 是结构化 Raw 表达式：原样 SQL + 参数绑定（防注入）。
type rawExpr struct {
	sql  string
	args []any
}

// Condition 实现 Expression 接口。
func (r *rawExpr) Condition() (string, []any) {
	return r.sql, r.args
}

func (r *rawExpr) render(rc *renderContext) (string, []any) {
	// Raw 的占位符形式（? / $n）由使用者按方言书写；参数并入上下文保证收集顺序。
	rc.args = append(rc.args, r.args...)
	rc.argIndex += len(r.args)
	return r.sql, r.args
}

// Raw 创建结构化 Raw 表达式：SQL 原样输出，args 作为参数绑定。
// 用于 OperatorFunc 覆盖不到的当前库特殊语法。
func Raw(sql string, args ...any) Expression {
	return &rawExpr{sql: sql, args: args}
}

// existsCondition 是 EXISTS/NOT EXISTS 子查询条件。
type existsCondition struct {
	subquery *SelectBuilder
	negate   bool
}

// Condition 实现 Expression 接口。
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

// Exists 构造 EXISTS (SELECT ...) 子查询条件。
func Exists(subquery *SelectBuilder) Expression {
	return &existsCondition{subquery: subquery}
}

// NotExists 构造 NOT EXISTS (SELECT ...) 子查询条件。
func NotExists(subquery *SelectBuilder) Expression {
	return &existsCondition{subquery: subquery, negate: true}
}

// exprCondition 是任意表达式上的比较条件（如 (-age) > ?、函数结果 = ?）。
type exprCondition struct {
	left Expression
	op   opType
	val  any
}

// Condition 实现 Expression 接口。
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

// 包级比较函数：作用于任意表达式（字段/算术/CASE/函数结果），与 Field 方法并存。

// Eq 返回表达式等于值的条件。
func Eq(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opEq, val: v}
}

// Ne 返回表达式不等于值的条件。
func Ne(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opNe, val: v}
}

// Gt 返回表达式大于值的条件。
func Gt(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opGt, val: v}
}

// Gte 返回表达式大于等于值的条件。
func Gte(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opGte, val: v}
}

// Lt 返回表达式小于值的条件。
func Lt(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opLt, val: v}
}

// Lte 返回表达式小于等于值的条件。
func Lte(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opLte, val: v}
}

// Like 返回表达式 LIKE 模式匹配条件。
func Like(e Expression, v any) Expression {
	return &exprCondition{left: e, op: opLike, val: v}
}

// IsNull 返回表达式 IS NULL 条件。
func IsNull(e Expression) Expression {
	return &exprCondition{left: e, op: opIsNull}
}

// IsNotNull 返回表达式 IS NOT NULL 条件。
func IsNotNull(e Expression) Expression {
	return &exprCondition{left: e, op: opIsNotNull}
}
