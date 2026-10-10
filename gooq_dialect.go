package gooq

import (
	"fmt"
	"strings"
	"sync"
)

// Dialect 是渲染方言的名字。连接驱动通过这个名字选择 SQL 渲染实现。
type Dialect string

const (
	// DialectMySQL 是 MySQL 渲染方言。
	DialectMySQL Dialect = "mysql"
	// DialectPgsql 是 PostgreSQL 渲染方言。
	DialectPgsql Dialect = "pgsql"
	// DialectSQLite 是 SQLite 渲染方言。
	DialectSQLite Dialect = "sqlite"
)

const (
	// GroupRollup 是 GROUP BY ROLLUP。
	GroupRollup = "ROLLUP"
	// GroupCube 是 GROUP BY CUBE。
	GroupCube = "CUBE"
	// GroupGroupingSets 是 GROUP BY GROUPING SETS。
	GroupGroupingSets = "GROUPING SETS"
)

// UpsertClause 是 INSERT 冲突动作。方言决定把它渲染成 INSERT IGNORE、ON DUPLICATE KEY 或 ON CONFLICT。
type UpsertClause struct {
	// DoNothing 为 true 时冲突不更新。
	DoNothing bool
	// ConflictCols 是冲突目标列。PostgreSQL 和 SQLite 会写进 ON CONFLICT。
	ConflictCols []string
	// Updates 是冲突后要写入的列。MySQL 只使用列名。
	Updates map[string]any
}

// GroupConcatInput 是字符串聚合在各方言上的渲染输入。
type GroupConcatInput struct {
	// FieldSQL 是已经渲染好的聚合表达式。
	FieldSQL string
	// Separator 是调用方传入的分隔符，空字符串表示未指定。
	Separator string
	// Distinct 为 true 时要求去重。
	Distinct bool
	// OrderSQL 是已经渲染好的组内排序，空字符串表示不排序。
	OrderSQL string
}

// DialectSpec 是一种数据库的 SQL 渲染契约。
// 内置方言在 init 中注册。新方言实现本接口后调用 RegisterDialect。
// 嵌入 MySQLDialect、PostgresDialect 或 SQLiteDialect 可以只覆盖有差异的方法。
// 未注册的方言不会回退成 MySQL。
type DialectSpec interface {
	// Name 返回注册名，与 Dialect 常量或自定义名字一致。
	Name() Dialect
	// Quote 按该方言的标识符引号包裹名字。
	Quote(ident string) string
	// Placeholder 返回第 n 个参数的占位符，n 从 1 开始。
	Placeholder(n int) string
	// CastType 返回 CAST 的目标类型名。
	CastType(localType LocalType) string
	// ShareLock 返回共享行锁子句。不支持时返回空字符串。
	ShareLock() string
	// Returning 返回 RETURNING 关键字。不支持时返回空字符串。
	Returning() string
	// RenderLimit 渲染分页。limit 大于 0。数字直接写入 SQL。
	RenderLimit(rc *RenderContext, limit, offset int) string
	// InsertKeyword 返回 INSERT 或 INSERT IGNORE。
	InsertKeyword(doNothing bool) string
	// RenderUpsert 返回 INSERT 语句末尾的冲突子句。不需要额外子句时返回空字符串。
	RenderUpsert(rc *RenderContext, clause UpsertClause) string
	// UpdateFrom 为 true 时，多表 UPDATE 使用 FROM，并把 JOIN 条件并入 WHERE。
	UpdateFrom() bool
	// JoinKeyword 改写连接关键字。大多数方言原样返回。
	JoinKeyword(keyword string) string
	// SupportsGroup 检查 GROUP BY 扩展。不支持时返回错误。
	SupportsGroup(ext string) error
	// RollupWith 为 true 时，ROLLUP 渲染为 GROUP BY cols WITH ROLLUP。
	RollupWith() bool
	// OmitLock 为 true 时，渲染阶段丢弃行锁子句。
	OmitLock() bool
	// RenderDateFormat 按该方言渲染日期格式化。format 使用 MySQL 格式符。
	RenderDateFormat(fieldSQL, format string) string
	// RenderGroupConcat 按该方言渲染字符串聚合。
	RenderGroupConcat(in GroupConcatInput) (string, error)
	// RenderMedian 按该方言渲染中位数。
	RenderMedian(fieldSQL string) string
}

var (
	dialectRegistryMu sync.RWMutex
	dialectRegistry   = map[Dialect]DialectSpec{}
)

// RegisterDialect 注册或替换一种渲染方言。spec 为空或没有名字时 panic。
func RegisterDialect(spec DialectSpec) {
	if spec == nil || spec.Name() == "" {
		panic("gooq: RegisterDialect requires a named spec")
	}
	dialectRegistryMu.Lock()
	dialectRegistry[spec.Name()] = spec
	dialectRegistryMu.Unlock()
}

// LookupDialect 返回已注册的渲染方言。未注册时返回 nil。
func LookupDialect(name Dialect) DialectSpec {
	dialectRegistryMu.RLock()
	defer dialectRegistryMu.RUnlock()
	return dialectRegistry[name]
}

// DialectSpecOf 返回已注册的渲染方言。未注册时返回错误。
func DialectSpecOf(name Dialect) (DialectSpec, error) {
	spec := LookupDialect(name)
	if spec == nil {
		return nil, fmt.Errorf("gooq: dialect %q is not registered", name)
	}
	return spec, nil
}

func init() {
	RegisterDialect(MySQLDialect{})
	RegisterDialect(PostgresDialect{})
	RegisterDialect(SQLiteDialect{})
}

func castType(table map[LocalType]string, localType LocalType) string {
	if v, ok := table[localType]; ok {
		return v
	}
	return strings.ToUpper(string(localType))
}

func renderLimitOffset(_ *RenderContext, limit, offset int) string {
	if offset > 0 {
		return fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}
	return fmt.Sprintf(" LIMIT %d", limit)
}

func renderConflict(rc *RenderContext, clause UpsertClause) string {
	quoted := make([]string, len(clause.ConflictCols))
	for i, col := range clause.ConflictCols {
		quoted[i] = rc.Quote(col)
	}
	target := strings.Join(quoted, ", ")
	if clause.DoNothing {
		return fmt.Sprintf(" ON CONFLICT (%s) DO NOTHING", target)
	}
	sets := make([]string, 0, len(clause.Updates))
	for col, val := range clause.Updates {
		sets = append(sets, fmt.Sprintf("%s = %s", rc.Quote(col), rc.AddArg(val)))
	}
	return fmt.Sprintf(" ON CONFLICT (%s) DO UPDATE SET %s", target, strings.Join(sets, ", "))
}
