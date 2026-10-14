package gooq

// Expression 是表达式节点。渲染只返回 SQL，参数经 RenderContext.AddArg 累积。
type Expression interface {
	Render(rc *RenderContext) string
}

// Subquery 表示可作为子查询使用的表达式（典型为 SELECT 构建器）。
type Subquery interface {
	Expression
}

// RenderContext 是一次渲染过程的方言与参数状态。
type RenderContext struct {
	dialect  Dialect
	spec     DialectSpec
	args     []any
	argIndex int
	err      error
}

func NewRenderContext(dialect Dialect) *RenderContext {
	rc := &RenderContext{dialect: dialect}
	if dialect != "" {
		rc.spec = LookupDialect(dialect)
	}
	return rc
}

func (rc *RenderContext) Dialect() Dialect {
	return rc.dialect
}

func (rc *RenderContext) Spec() DialectSpec {
	return rc.spec
}

func (rc *RenderContext) Args() []any {
	return rc.args
}

// Err 返回渲染期间记录的第一个错误。
func (rc *RenderContext) Err() error {
	return rc.err
}

// Fail 记录渲染错误。已有错误时保留第一个。
func (rc *RenderContext) Fail(err error) {
	if rc.err == nil && err != nil {
		rc.err = err
	}
}

// Quote 按方言引号包裹标识符。未注册方言使用双引号。
func (rc *RenderContext) Quote(ident string) string {
	if rc.spec != nil {
		return rc.spec.Quote(ident)
	}
	return `"` + ident + `"`
}

func (rc *RenderContext) AddArg(v any) string {
	rc.args = append(rc.args, v)
	rc.argIndex++
	return rc.placeholder(rc.argIndex)
}

func (rc *RenderContext) placeholder(n int) string {
	if rc.spec != nil {
		return rc.spec.Placeholder(n)
	}
	return "?"
}

// ShareLockKeyword 返回共享行锁子句。未注册方言回退为 FOR SHARE。
func (rc *RenderContext) ShareLockKeyword() string {
	if rc.spec != nil && rc.spec.ShareLock() != "" {
		return rc.spec.ShareLock()
	}
	return "FOR SHARE"
}

// Render 渲染表达式节点。
func (rc *RenderContext) Render(e Expression) string {
	if e == nil {
		return ""
	}
	return e.Render(rc)
}

// RenderValue 渲染 SET/SELECT 位值：表达式渲染为 SQL 片段，普通值绑定为参数。
func (rc *RenderContext) RenderValue(v any) string {
	if e, ok := v.(Expression); ok && e != nil {
		return e.Render(rc)
	}
	return rc.AddArg(v)
}
