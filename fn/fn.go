// Package fn 是 gooq 的数据库函数库：统一函数节点、标准函数构造器与方言渲染策略。
package fn

import (
	"strings"

	"github.com/lanceadd/gooq"
)

// RenderFn 是函数的方言渲染策略：接收已渲染的参数 SQL 片段，返回函数 SQL 与附加参数。
type RenderFn func(rc *gooq.RenderContext, argsSQL []string) (sql string, args []any)

// FuncExpr 是所有函数表达式的统一节点：NAME(args...)。
// 方言差异通过 RenderWith 挂实例级渲染策略；结构选项（分隔符/去重等）由构造器捕获。
type FuncExpr struct {
	name     string
	args     []any
	alias    string
	over     *overClause
	filter   gooq.Expression
	render   RenderFn
	validate func(gooq.Dialect) error
}

// New 构造通用函数节点（无方言感知，按 NAME(args...) 渲染）。
func New(name string, args ...any) *FuncExpr {
	return &FuncExpr{name: name, args: args}
}

// RenderWith 挂载方言渲染策略（实例级，非全局注册表）并返回自身便于链式。
func (f *FuncExpr) RenderWith(render RenderFn) *FuncExpr {
	f.render = render
	return f
}

func (f *FuncExpr) Name() string {
	return f.name
}

func (f *FuncExpr) As(alias string) *FuncExpr {
	f.alias = alias
	return f
}

func (f *FuncExpr) Over(partitionBy []gooq.Expression, orderBy []gooq.Expression) *FuncExpr {
	f.over = &overClause{partitionBy: partitionBy, orderBy: orderBy}
	return f
}

func (f *FuncExpr) OverFrame(
	partitionBy []gooq.Expression, orderBy []gooq.Expression, frame WindowFrame,
) *FuncExpr {
	f.over = &overClause{partitionBy: partitionBy, orderBy: orderBy, frame: &frame}
	return f
}

func (f *FuncExpr) Filter(cond gooq.Expression) *FuncExpr {
	f.filter = cond
	return f
}

func (f *FuncExpr) Condition() (string, []any) {
	return f.Render(gooq.NewRenderContext(gooq.DialectMySQL))
}

func (f *FuncExpr) Render(rc *gooq.RenderContext) (string, []any) {
	var (
		argsSQL []string
		argsAll []any
	)
	for _, arg := range f.args {
		if expr, ok := arg.(gooq.Expression); ok {
			sql, args := rc.Render(expr)
			argsSQL = append(argsSQL, sql)
			argsAll = append(argsAll, args...)
			continue
		}
		argsSQL = append(argsSQL, rc.AddArg(arg))
		argsAll = append(argsAll, arg)
	}
	var sql string
	if f.render != nil {
		var renderArgs []any
		sql, renderArgs = f.render(rc, argsSQL)
		argsAll = append(argsAll, renderArgs...)
	} else {
		sql = f.name + "(" + strings.Join(argsSQL, ", ") + ")"
	}
	if f.filter != nil {
		filterSQL, filterArgs := rc.Render(f.filter)
		sql += " FILTER (WHERE " + filterSQL + ")"
		argsAll = append(argsAll, filterArgs...)
	}
	if f.over != nil {
		sql += " OVER (" + renderOver(rc, f.over) + ")"
	}
	if f.alias != "" {
		sql += " AS " + f.alias
	}
	return sql, argsAll
}

func (f *FuncExpr) ValidateDialect(d gooq.Dialect) error {
	if f.validate != nil {
		return f.validate(d)
	}
	return nil
}

func (f *FuncExpr) SubExpressions() []gooq.Expression {
	var subs []gooq.Expression
	for _, arg := range f.args {
		if expr, ok := arg.(gooq.Expression); ok {
			subs = append(subs, expr)
		}
	}
	if f.filter != nil {
		subs = append(subs, f.filter)
	}
	return subs
}

type overClause struct {
	partitionBy []gooq.Expression
	orderBy     []gooq.Expression
	frame       *WindowFrame
}

// WindowFrame 是窗口框架（ROWS/RANGE BETWEEN start AND end）。
type WindowFrame struct {
	unit  string
	start string
	end   string
}

func RowsFrame(start, end string) WindowFrame {
	return WindowFrame{unit: "ROWS", start: start, end: end}
}

func RangeFrame(start, end string) WindowFrame {
	return WindowFrame{unit: "RANGE", start: start, end: end}
}

func renderOver(rc *gooq.RenderContext, over *overClause) string {
	var overParts []string
	if len(over.partitionBy) > 0 {
		var parts []string
		for _, p := range over.partitionBy {
			sqlPart, _ := rc.Render(p)
			parts = append(parts, sqlPart)
		}
		overParts = append(overParts, "PARTITION BY "+strings.Join(parts, ", "))
	}
	if len(over.orderBy) > 0 {
		var parts []string
		for _, o := range over.orderBy {
			sqlPart, _ := rc.Render(o)
			parts = append(parts, sqlPart)
		}
		overParts = append(overParts, "ORDER BY "+strings.Join(parts, ", "))
	}
	if over.frame != nil {
		overParts = append(overParts,
			over.frame.unit+" BETWEEN "+over.frame.start+" AND "+over.frame.end)
	}
	return strings.Join(overParts, " ")
}
