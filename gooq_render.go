package gooq

type Expression interface {
	Condition() (where string, args []any)
}

// Renderer 是表达式节点的方言渲染契约；包外自定义节点实现它即可参与渲染。
// 片段中的值必须经 RenderContext.AddArg 走占位符，保证参数序号一致。
type Renderer interface {
	Render(rc *RenderContext) (sql string, args []any)
}

// Subquery 表示可作为子查询使用的表达式（典型为 SELECT 构建器）。
type Subquery interface {
	Expression
	Renderer
}

// RenderContext 是一次渲染过程的方言与参数状态。
type RenderContext struct {
	dialect  Dialect
	spec     DialectSpec
	args     []any
	argIndex int
}

func NewRenderContext(dialect Dialect) *RenderContext {
	rc := &RenderContext{dialect: dialect}
	if dialect != "" {
		rc.spec = LookupDialect(dialect)
	}
	return rc
}

// Dialect 返回当前渲染方言。
func (rc *RenderContext) Dialect() Dialect {
	return rc.dialect
}

// Spec 返回当前渲染方言。未注册方言时为 nil。
func (rc *RenderContext) Spec() DialectSpec {
	return rc.spec
}

// Args 返回当前累积的参数列表。
func (rc *RenderContext) Args() []any {
	return rc.args
}

// Quote 按方言引号包裹标识符。未注册方言使用双引号。
func (rc *RenderContext) Quote(ident string) string {
	if rc.spec != nil {
		return rc.spec.Quote(ident)
	}
	return `"` + ident + `"`
}

// AddArg 追加值参数并返回其占位符。
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

// Render 渲染表达式节点；非节点值退化为绑定参数。
func (rc *RenderContext) Render(e Expression) (sql string, args []any) {
	if r, ok := e.(Renderer); ok {
		return r.Render(rc)
	}
	return rc.AddArg(e), nil
}

// RenderValue 渲染 SET/SELECT 位值：表达式渲染为 SQL 片段，普通值绑定为参数。
func (rc *RenderContext) RenderValue(v any) (string, []any) {
	if e, ok := v.(Expression); ok {
		return rc.Render(e)
	}
	return rc.AddArg(v), []any{v}
}
