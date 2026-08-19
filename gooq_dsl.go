// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// Package gooq 的 DSL 基础文件：方言、渲染上下文与表达式接口。
// 本文件定义类型化查询 DSL 的地基：Dialect、renderContext 与 Expression 接口。
// gooq 是纯 SQL 构建器：不依赖任何数据库实例，SQL 生成后可由调用方自选执行方式。
package gooq

import (
	"context"
	"fmt"
)

// Dialect 表示 SQL 方言，决定引号、占位符与语法差异。
type Dialect string

const (
	// DialectMySQL 是 MySQL 方言。
	DialectMySQL Dialect = "mysql"
	// DialectPgsql 是 PostgreSQL 方言。
	DialectPgsql Dialect = "pgsql"
	// DialectSQLite 是 SQLite 方言。
	DialectSQLite Dialect = "sqlite"
)

// Expression 是 DSL 中所有可渲染节点的接口：字段、条件、函数、Raw、子查询等。
// Condition 方法使用默认上下文渲染（MySQL 方言、无表别名、? 占位符），
// 主要用于单测断言与文档对齐；DSL 内部渲染统一走 render(renderContext)。
type Expression interface {
	Condition() (where string, args []any)
}

// expr 是渲染上下文感知的内部节点接口，所有 DSL 节点实现它。
type expr interface {
	render(rc *renderContext) (sql string, args []any)
}

// renderContext 是 DSL 渲染上下文，承载方言、参数与表别名映射。
// gooq 不依赖任何数据库实例：方言显式传入，驱动名由方言推断。
type renderContext struct {
	ctx        context.Context
	dialect    Dialect
	driverName string // 操作符驱动名（由方言推断）。
	args       []any
	aliases    map[string]string // tableName → alias。
	argIndex   int
}

// newRenderContext 创建渲染上下文；驱动名由方言推断（操作符按驱动分派）。
func newRenderContext(ctx context.Context, dialect Dialect) *renderContext {
	rc := &renderContext{
		ctx:     ctx,
		dialect: dialect,
		aliases: make(map[string]string),
	}
	if dialect != "" {
		rc.driverName = string(dialect)
	}
	return rc
}

// driver 返回操作符解析使用的驱动名（由方言推断）。
func (rc *renderContext) driver() string {
	return rc.driverName
}

// quote 按方言规则为标识符（表名/列名）添加引号。
func (rc *renderContext) quote(ident string) string {
	switch rc.dialect {
	case DialectMySQL:
		return "`" + ident + "`"
	default:
		return `"` + ident + `"`
	}
}

// addArg 追加一个参数并返回其占位符。
func (rc *renderContext) addArg(v any) string {
	rc.args = append(rc.args, v)
	rc.argIndex++
	switch rc.dialect {
	case DialectPgsql:
		return fmt.Sprintf(`$%d`, rc.argIndex)
	default:
		return "?"
	}
}

// aliasFor 返回表名在当前查询中的别名（无别名返回空串）。
func (rc *renderContext) aliasFor(tableName string) string {
	return rc.aliases[tableName]
}

// registerAlias 注册表名到别名的映射。
func (rc *renderContext) registerAlias(tableName, alias string) {
	rc.aliases[tableName] = alias
}

// render 渲染任意表达式；非 DSL 节点的字面量参数化为占位符。
func (rc *renderContext) render(e Expression) (sql string, args []any) {
	if r, ok := e.(interface {
		render(rc *renderContext) (sql string, args []any)
	}); ok {
		return r.render(rc)
	}
	return rc.addArg(e), nil
}
