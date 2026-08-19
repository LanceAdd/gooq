// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件实现操作符注册表（OperatorFunc）与类型化别名函数。
package gooq

import (
	"context"
	"fmt"
	"strings"
)

// OperatorFn 是操作符实现：给定上下文、实例与参数，产出 SQL 片段与参数。
type OperatorFn func(ctx context.Context, args ...any) (sql string, argsOut []any, err error)

// operatorRegistry 是操作符注册表：name → driver → impl。
var operatorRegistry = make(map[string]map[string]OperatorFn)

// OperatorFunc 注册操作符实现；drivers 为空时作为默认实现（所有驱动回退）。
func OperatorFunc(name string, f OperatorFn, drivers ...string) {
	if len(drivers) == 0 {
		if operatorRegistry[name] == nil {
			operatorRegistry[name] = make(map[string]OperatorFn)
		}
		operatorRegistry[name][""] = f
		return
	}
	for _, driver := range drivers {
		if operatorRegistry[name] == nil {
			operatorRegistry[name] = make(map[string]OperatorFn)
		}
		operatorRegistry[name][driver] = f
	}
}

// getOperator 按驱动名解析操作符实现（驱动实现优先，回退默认）。
func getOperator(name, driver string) OperatorFn {
	drivers, ok := operatorRegistry[name]
	if !ok {
		return nil
	}
	if f, ok := drivers[driver]; ok {
		return f
	}
	return drivers[""]
}

// overClause 是窗口函数子句。
type overClause struct {
	partitionBy []Expression
	orderBy     []OrderClause
	frame       *WindowFrame
}

// WindowFrame 是窗口框架（ROWS/RANGE BETWEEN ... AND ...）。
type WindowFrame struct {
	unit  string // ROWS 或 RANGE。
	start string // 框架起点，如 UNBOUNDED PRECEDING / CURRENT ROW / 2 PRECEDING。
	end   string // 框架终点，如 CURRENT ROW / UNBOUNDED FOLLOWING / 2 FOLLOWING。
}

// RowsFrame 创建 ROWS 窗口框架。
func RowsFrame(start, end string) WindowFrame {
	return WindowFrame{unit: "ROWS", start: start, end: end}
}

// RangeFrame 创建 RANGE 窗口框架。
func RangeFrame(start, end string) WindowFrame {
	return WindowFrame{unit: "RANGE", start: start, end: end}
}

// Func 构造自定义操作符函数表达式（配合 OperatorFunc 注册使用）。
func Func(name string, args ...any) Fn {
	return Fn{name: name, args: args}
}

// Fn 是函数表达式节点：操作符名 + 参数（字段/字面量/嵌套函数/子查询）。
type Fn struct {
	name  string
	args  []any
	alias string
	over  *overClause
}

// As 返回带别名的函数表达式（SELECT 位置渲染为 fn() AS alias）。
func (f Fn) As(alias string) Fn {
	f.alias = alias
	return f
}

// Over 附加窗口子句：OVER (PARTITION BY ... ORDER BY ...)。
func (f Fn) Over(partitionBy []Expression, orderBy []OrderClause) Fn {
	f.over = &overClause{partitionBy: partitionBy, orderBy: orderBy}
	return f
}

// OverFrame 附加带框架的窗口子句：OVER (PARTITION BY ... ORDER BY ... ROWS BETWEEN ... AND ...)。
func (f Fn) OverFrame(partitionBy []Expression, orderBy []OrderClause, frame WindowFrame) Fn {
	f.over = &overClause{partitionBy: partitionBy, orderBy: orderBy, frame: &frame}
	return f
}

// Condition 实现 Expression 接口。
func (f Fn) Condition() (string, []any) {
	return f.render(newRenderContext(context.Background(), DialectMySQL))
}

func (f Fn) render(rc *renderContext) (string, []any) {
	var (
		argsSQL []string
		argsAll []any
	)
	for _, arg := range f.args {
		switch v := arg.(type) {
		case Expression:
			sql, args := rc.render(v)
			argsSQL = append(argsSQL, sql)
			argsAll = append(argsAll, args...)
		default:
			argsSQL = append(argsSQL, rc.addArg(v))
			argsAll = append(argsAll, v)
		}
	}
	var (
		sql    string
		impl   = getOperator(f.name, rc.driver())
		opArgs = make([]any, len(argsSQL))
	)
	for i := range argsSQL {
		opArgs[i] = argsSQL[i]
	}
	if impl != nil {
		implSQL, implArgs, err := impl(rc.ctx, opArgs...)
		if err == nil {
			sql = implSQL
			argsAll = append(argsAll, implArgs...)
		} else {
			sql = genericOperatorRender(f.name, argsSQL)
		}
	} else {
		sql = genericOperatorRender(f.name, argsSQL)
	}
	if f.over != nil {
		var overParts []string
		if len(f.over.partitionBy) > 0 {
			var parts []string
			for _, p := range f.over.partitionBy {
				sqlPart, _ := rc.render(p)
				parts = append(parts, sqlPart)
			}
			overParts = append(overParts, "PARTITION BY "+strings.Join(parts, ", "))
		}
		if len(f.over.orderBy) > 0 {
			var parts []string
			for _, o := range f.over.orderBy {
				sqlPart, _ := o.render(rc)
				parts = append(parts, sqlPart)
			}
			overParts = append(overParts, "ORDER BY "+strings.Join(parts, ", "))
		}
		if f.over.frame != nil {
			overParts = append(overParts,
				f.over.frame.unit+" BETWEEN "+f.over.frame.start+" AND "+f.over.frame.end)
		}
		sql += " OVER (" + strings.Join(overParts, " ") + ")"
	}
	if f.alias != "" {
		sql += " AS " + f.alias
	}
	return sql, argsAll
}

// genericOperatorRender 是未注册操作符的通用渲染：NAME(a1, a2, ...)。
func genericOperatorRender(name string, argsSQL []string) string {
	return name + "(" + strings.Join(argsSQL, ", ") + ")"
}

// Fn 系列别名函数（无字符串硬编码的开发入口）。

// DateFormatFunc 构造日期格式化函数表达式。
// 格式参数（string）自动转为 SQL 字符串字面量，供各方言实现内联转换。
func DateFormatFunc(args ...any) Fn {
	converted := make([]any, len(args))
	for i, arg := range args {
		if s, ok := arg.(string); ok {
			converted[i] = Str(s)
		} else {
			converted[i] = arg
		}
	}
	return Fn{name: "DATE_FORMAT", args: converted}
}

// CountFunc 构造 COUNT 聚合函数表达式。
func CountFunc(field Expression) Fn {
	return Fn{name: "COUNT", args: []any{field}}
}

// SumFunc 构造 SUM 聚合函数表达式。
func SumFunc(field Expression) Fn {
	return Fn{name: "SUM", args: []any{field}}
}

// AvgFunc 构造 AVG 聚合函数表达式。
func AvgFunc(field Expression) Fn {
	return Fn{name: "AVG", args: []any{field}}
}

// MinFunc 构造 MIN 聚合函数表达式。
func MinFunc(field Expression) Fn {
	return Fn{name: "MIN", args: []any{field}}
}

// MaxFunc 构造 MAX 聚合函数表达式。
func MaxFunc(field Expression) Fn {
	return Fn{name: "MAX", args: []any{field}}
}

// CoalesceFunc 构造 COALESCE 函数表达式。
func CoalesceFunc(args ...any) Fn {
	return Fn{name: "COALESCE", args: args}
}

// IfNullFunc 构造 IFNULL 函数表达式。
func IfNullFunc(a, b Expression) Fn {
	return Fn{name: "IFNULL", args: []any{a, b}}
}

// RankFunc 构造 RANK 窗口函数表达式。
func RankFunc() Fn {
	return Fn{name: "RANK"}
}

// NowFunc 构造 NOW 函数表达式。
func NowFunc() Fn {
	return Fn{name: "NOW"}
}

// init 注册内置操作符的默认实现（MySQL 系）。
func init() {
	OperatorFunc("DATE_FORMAT", func(ctx context.Context, args ...any) (string, []any, error) {
		if len(args) != 2 {
			return "", nil, fmt.Errorf("DATE_FORMAT expects 2 arguments")
		}
		return fmt.Sprintf(`DATE_FORMAT(%s, %s)`, args[0], args[1]), nil, nil
	})
	OperatorFunc("COUNT", func(ctx context.Context, args ...any) (string, []any, error) {
		return fmt.Sprintf(`COUNT(%s)`, args[0]), nil, nil
	})
	OperatorFunc("SUM", func(ctx context.Context, args ...any) (string, []any, error) {
		return fmt.Sprintf(`SUM(%s)`, args[0]), nil, nil
	})
	OperatorFunc("COALESCE", func(ctx context.Context, args ...any) (string, []any, error) {
		return "COALESCE(" + strings.Join(toStrings(args), ", ") + ")", nil, nil
	})
	OperatorFunc("IFNULL", func(ctx context.Context, args ...any) (string, []any, error) {
		return fmt.Sprintf(`IFNULL(%s, %s)`, args[0], args[1]), nil, nil
	})
	OperatorFunc("RANK", func(ctx context.Context, args ...any) (string, []any, error) {
		return "RANK()", nil, nil
	})
	OperatorFunc("NOW", func(ctx context.Context, args ...any) (string, []any, error) {
		return "NOW()", nil, nil
	})
	// DATE_FORMAT 跨库内置：PG 用 TO_CHAR（格式映射），SQLite 用 strftime（格式兼容）。
	OperatorFunc("DATE_FORMAT", func(ctx context.Context, args ...any) (string, []any, error) {
		if len(args) != 2 {
			return "", nil, fmt.Errorf("DATE_FORMAT expects 2 arguments")
		}
		pgFormat := mysqlToPgFormat(trimQuotes(args[1].(string)))
		return "TO_CHAR(" + args[0].(string) + ", '" + pgFormat + "')", nil, nil
	}, "pgsql")
	OperatorFunc("DATE_FORMAT", func(ctx context.Context, args ...any) (string, []any, error) {
		if len(args) != 2 {
			return "", nil, fmt.Errorf("DATE_FORMAT expects 2 arguments")
		}
		return "strftime(" + args[1].(string) + ", " + args[0].(string) + ")", nil, nil
	}, "sqlite")
}

// trimQuotes 去除 SQL 字符串字面量的引号。
func trimQuotes(s string) string {
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return s
}

// mysqlToPgFormat 将 MySQL 日期格式转换为 PostgreSQL TO_CHAR 格式（常见项）。
func mysqlToPgFormat(format string) string {
	var replacer = strings.NewReplacer(
		"%Y", "YYYY", "%y", "YY",
		"%m", "MM", "%d", "DD",
		"%H", "HH24", "%i", "MI", "%s", "SS",
		"%e", "FMDD", "%j", "DDD",
	)
	return replacer.Replace(format)
}

// toStrings 将 any 切片转换为字符串切片（渲染参数）。
func toStrings(args []any) []string {
	result := make([]string, len(args))
	for i, a := range args {
		result[i] = fmt.Sprintf(`%v`, a)
	}
	return result
}

// 第三批：扩展函数库（字符串/数学/日期/窗口/聚合变体）。
// 未注册实现的操作符由通用渲染 NAME(args) 兜底（MySQL 系语法兼容）。

// 字符串函数。

// ConcatFunc 构造 CONCAT 函数表达式。
func ConcatFunc(args ...any) Fn {
	return Fn{name: "CONCAT", args: args}
}

// SubstringFunc 构造 SUBSTRING 函数表达式。
func SubstringFunc(s any, start, length any) Fn {
	return Fn{name: "SUBSTRING", args: []any{s, start, length}}
}

// UpperFunc 构造 UPPER 函数表达式。
func UpperFunc(e Expression) Fn {
	return Fn{name: "UPPER", args: []any{e}}
}

// LowerFunc 构造 LOWER 函数表达式。
func LowerFunc(e Expression) Fn {
	return Fn{name: "LOWER", args: []any{e}}
}

// TrimFunc 构造 TRIM 函数表达式。
func TrimFunc(e Expression) Fn {
	return Fn{name: "TRIM", args: []any{e}}
}

// ReplaceFunc 构造 REPLACE 函数表达式。
func ReplaceFunc(s, old, new any) Fn {
	return Fn{name: "REPLACE", args: []any{s, old, new}}
}

// LengthFunc 构造 LENGTH 函数表达式。
func LengthFunc(e Expression) Fn {
	return Fn{name: "LENGTH", args: []any{e}}
}

// 数学函数。

// AbsFunc 构造 ABS 函数表达式。
func AbsFunc(e Expression) Fn {
	return Fn{name: "ABS", args: []any{e}}
}

// RoundFunc 构造 ROUND 函数表达式。
func RoundFunc(e Expression, decimals any) Fn {
	return Fn{name: "ROUND", args: []any{e, decimals}}
}

// CeilFunc 构造 CEIL 函数表达式。
func CeilFunc(e Expression) Fn {
	return Fn{name: "CEIL", args: []any{e}}
}

// FloorFunc 构造 FLOOR 函数表达式。
func FloorFunc(e Expression) Fn {
	return Fn{name: "FLOOR", args: []any{e}}
}

// ModFunc 构造 MOD 函数表达式。
func ModFunc(a, b any) Fn {
	return Fn{name: "MOD", args: []any{a, b}}
}

// 日期函数。

// CurDateFunc 构造 CURDATE 函数表达式。
func CurDateFunc() Fn {
	return Fn{name: "CURDATE"}
}

// DateAddFunc 构造 DATE_ADD 函数表达式。
func DateAddFunc(date Expression, interval string) Fn {
	return Fn{name: "DATE_ADD", args: []any{date, interval}}
}

// DateDiffFunc 构造 DATEDIFF 函数表达式。
func DateDiffFunc(a, b Expression) Fn {
	return Fn{name: "DATEDIFF", args: []any{a, b}}
}

// 聚合变体。

// CountDistinctFunc 构造 COUNT(DISTINCT field) 聚合表达式。
func CountDistinctFunc(field Expression) Fn {
	return Fn{name: "COUNT", args: []any{Distinct(field)}}
}

// 窗口函数。

// RowNumberFunc 构造 ROW_NUMBER 窗口函数。
func RowNumberFunc() Fn {
	return Fn{name: "ROW_NUMBER"}
}

// DenseRankFunc 构造 DENSE_RANK 窗口函数。
func DenseRankFunc() Fn {
	return Fn{name: "DENSE_RANK"}
}

// NtileFunc 构造 NTILE 窗口函数。
func NtileFunc(n int) Fn {
	return Fn{name: "NTILE", args: []any{n}}
}

// LagFunc 构造 LAG 窗口函数。
func LagFunc(field Expression, offset, defaultValue any) Fn {
	return Fn{name: "LAG", args: []any{field, offset, defaultValue}}
}

// LeadFunc 构造 LEAD 窗口函数。
func LeadFunc(field Expression, offset, defaultValue any) Fn {
	return Fn{name: "LEAD", args: []any{field, offset, defaultValue}}
}
