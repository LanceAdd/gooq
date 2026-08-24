package gooq

import (
	"context"
)

type Dialect string

const (
	DialectMySQL  Dialect = "mysql"
	DialectPgsql  Dialect = "pgsql"
	DialectSQLite Dialect = "sqlite"
)

type Expression interface {
	Condition() (where string, args []any)
}

type expr interface {
	render(rc *renderContext) (sql string, args []any)
}

type renderContext struct {
	ctx         context.Context
	dialect     Dialect
	dialectInfo *DialectInfo // 方言元数据（注册表解析）。
	driverName  string       // 操作符驱动名（由方言推断）。
	args        []any
	aliases     map[string]string // tableName → alias。
	argIndex    int
}

func newRenderContext(ctx context.Context, dialect Dialect) *renderContext {
	rc := &renderContext{
		ctx:     ctx,
		dialect: dialect,
		aliases: make(map[string]string),
	}
	if dialect != "" {
		rc.driverName = string(dialect)
		rc.dialectInfo = getDialectInfo(string(dialect))
	}
	return rc
}

func (rc *renderContext) driver() string {
	return rc.driverName
}

func (rc *renderContext) quote(ident string) string {
	if rc.dialectInfo != nil && rc.dialectInfo.QuoteChar != "" {
		return rc.dialectInfo.QuoteChar + ident + rc.dialectInfo.QuoteChar
	}
	return `"` + ident + `"`
}

func (rc *renderContext) addArg(v any) string {
	rc.args = append(rc.args, v)
	rc.argIndex++
	return rc.placeholder(rc.argIndex)
}

func (rc *renderContext) aliasFor(tableName string) string {
	return rc.aliases[tableName]
}

func (rc *renderContext) registerAlias(tableName, alias string) {
	rc.aliases[tableName] = alias
}

func (rc *renderContext) render(e Expression) (sql string, args []any) {
	if r, ok := e.(interface {
		render(rc *renderContext) (sql string, args []any)
	}); ok {
		return r.render(rc)
	}
	return rc.addArg(e), nil
}
