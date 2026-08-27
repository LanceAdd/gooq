package gooq

type Expression interface {
	Condition() (where string, args []any)
}

type expr interface {
	render(rc *renderContext) (sql string, args []any)
}

type renderContext struct {
	dialect     Dialect
	dialectInfo *DialectInfo // 方言元数据（注册表解析）。
	args        []any
	argIndex    int
}

func newRenderContext(dialect Dialect) *renderContext {
	rc := &renderContext{dialect: dialect}
	if dialect != "" {
		rc.dialectInfo = getDialectInfo(string(dialect))
	}
	return rc
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

func (rc *renderContext) render(e Expression) (sql string, args []any) {
	if r, ok := e.(interface {
		render(rc *renderContext) (sql string, args []any)
	}); ok {
		return r.render(rc)
	}
	return rc.addArg(e), nil
}
