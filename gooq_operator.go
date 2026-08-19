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

	"github.com/gogf/gf/v2/database/gdb"
)

// OperatorFn 是操作符实现：给定上下文、实例与参数，产出 SQL 片段与参数。
type OperatorFn func(ctx context.Context, db gdb.DB, args ...any) (sql string, argsOut []any, err error)

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

// Condition 实现 Expression 接口。
func (f Fn) Condition() (string, []any) {
	return f.render(newRenderContext(context.Background(), nil, DialectMySQL))
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
		implSQL, implArgs, err := impl(rc.ctx, rc.db, opArgs...)
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

// operatorDriverName 返回实例的驱动名（无实例时为空串，走默认实现）。
func operatorDriverName(db gdb.DB) string {
	if db == nil {
		return ""
	}
	if core, ok := db.(interface{ GetConfig() *gdb.ConfigNode }); ok {
		return core.GetConfig().Type
	}
	return ""
}

// Fn 系列别名函数（无字符串硬编码的开发入口）。

// DateFormatFunc 构造日期格式化函数表达式。
func DateFormatFunc(args ...any) Fn {
	return Fn{name: "DATE_FORMAT", args: args}
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
	OperatorFunc("DATE_FORMAT", func(ctx context.Context, db gdb.DB, args ...any) (string, []any, error) {
		if len(args) != 2 {
			return "", nil, fmt.Errorf("DATE_FORMAT expects 2 arguments")
		}
		return fmt.Sprintf(`DATE_FORMAT(%s, %s)`, args[0], args[1]), nil, nil
	})
	OperatorFunc("COUNT", func(ctx context.Context, db gdb.DB, args ...any) (string, []any, error) {
		return fmt.Sprintf(`COUNT(%s)`, args[0]), nil, nil
	})
	OperatorFunc("SUM", func(ctx context.Context, db gdb.DB, args ...any) (string, []any, error) {
		return fmt.Sprintf(`SUM(%s)`, args[0]), nil, nil
	})
	OperatorFunc("COALESCE", func(ctx context.Context, db gdb.DB, args ...any) (string, []any, error) {
		return "COALESCE(" + strings.Join(toStrings(args), ", ") + ")", nil, nil
	})
	OperatorFunc("IFNULL", func(ctx context.Context, db gdb.DB, args ...any) (string, []any, error) {
		return fmt.Sprintf(`IFNULL(%s, %s)`, args[0], args[1]), nil, nil
	})
	OperatorFunc("RANK", func(ctx context.Context, db gdb.DB, args ...any) (string, []any, error) {
		return "RANK()", nil, nil
	})
	OperatorFunc("NOW", func(ctx context.Context, db gdb.DB, args ...any) (string, []any, error) {
		return "NOW()", nil, nil
	})
}

// toStrings 将 any 切片转换为字符串切片（渲染参数）。
func toStrings(args []any) []string {
	result := make([]string, len(args))
	for i, a := range args {
		result[i] = fmt.Sprintf(`%v`, a)
	}
	return result
}
