package gooq

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Dialect 是渲染方言的名字。连接驱动通过这个名字选择 SQL 渲染实现。
type Dialect string

const (
	DialectMySQL  Dialect = "mysql"
	DialectPgsql  Dialect = "pgsql"
	DialectSQLite Dialect = "sqlite"
	// DialectMariaDB 的语句形状与 MySQL 相同。
	DialectMariaDB Dialect = "mariadb"
	// DialectTiDB 的语句形状与 MySQL 相同。
	DialectTiDB Dialect = "tidb"
	// DialectGaussDB 是 GaussDB（openGauss）渲染方言。
	DialectGaussDB Dialect = "gaussdb"
	// DialectOceanBase 的语句形状与 MySQL 相同。
	DialectOceanBase Dialect = "oceanbase"
	// DialectDM 是达梦渲染方言。
	DialectDM Dialect = "dm"
)

const (
	GroupRollup       = "ROLLUP"
	GroupCube         = "CUBE"
	GroupGroupingSets = "GROUPING SETS"
)

// RowLockWait 是行锁的等待策略。零值表示数据库默认的等待。
// 方言实现 RenderRowLockWait 才支持。没实现时 ToSql 失败，不把子句丢掉。
type RowLockWait int

const (
	// RowLockSkipLocked 跳过已被锁住的行。
	RowLockSkipLocked RowLockWait = iota + 1
	// RowLockNoWait 遇到锁立刻失败，不等待。
	RowLockNoWait
)

// RowLockWaitSQL 返回等待策略的 SQL 关键字。
func RowLockWaitSQL(wait RowLockWait) string {
	switch wait {
	case RowLockSkipLocked:
		return "SKIP LOCKED"
	case RowLockNoWait:
		return "NOWAIT"
	default:
		return ""
	}
}

// UpsertClause 是 INSERT 冲突动作。方言决定把它渲染成 INSERT IGNORE、ON DUPLICATE KEY、ON CONFLICT 或 MERGE。
type UpsertClause struct {
	// DoNothing 为 true 时冲突不更新。
	DoNothing bool
	// ConflictCols 是冲突目标列。PostgreSQL 和 SQLite 会写进 ON CONFLICT。达梦 MERGE 的 ON 也使用它。
	ConflictCols []string
	// Updates 是冲突后要写入的列。MySQL 只使用列名。
	Updates map[string]any
}

// InsertStatement 是已经渲染好占位符的 INSERT。
// Table 含限定名和引号。Columns 是原始列名。Rows 的每一格是值的 SQL。
type InsertStatement struct {
	Table   string
	Columns []string
	Rows    [][]string
	Upsert  *UpsertClause
}

// InsertRenderer 在冲突动作无法接在 INSERT 后面时，由方言渲染整句。
// 达梦把 INSERT IGNORE 和冲突更新写成 MERGE。
type InsertRenderer interface {
	RenderInsert(rc *RenderContext, stmt InsertStatement) (string, error)
}

// TableLikeRenderer 由支持复制表结构的方言实现。没实现表示不支持 CREATE TABLE LIKE。
type TableLikeRenderer interface {
	RenderCreateTableLike(rc *RenderContext, template, target string) (string, error)
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

// JoinRender 是已经渲染好的一个连接，供 DELETE 按方言组装。
type JoinRender struct {
	// Keyword 是连接关键字，例如 LEFT JOIN。
	Keyword string
	// Table 是带别名的表或派生表。
	Table string
	// On 是连接条件，不含 ON 关键字。空字符串表示没有条件。
	On string
}

// DialectSpec 是一种数据库的语句形状契约。
// 内置方言在 init 中注册。新方言实现本接口后调用 RegisterDialect。
// 嵌入 MySQLDialect、PostgresDialect 或 SQLiteDialect 可以只覆盖有差异的方法。
// 未注册的方言不会回退成 MySQL。
// 日期、聚合等函数渲染留在具体方言类型上，不在本接口里。
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
	// RenderLimit 渲染分页。limit 为 0 且 offset 大于 0 时只表达偏移。
	// PostgreSQL 和 SQLite 写 OFFSET n。MySQL 写 LIMIT 18446744073709551615 OFFSET n。
	RenderLimit(rc *RenderContext, limit, offset int) string
	// InsertKeyword 返回 INSERT 或 INSERT IGNORE。
	InsertKeyword(doNothing bool) string
	// RenderUpsert 返回 INSERT 语句末尾的冲突子句。不需要额外子句时返回空字符串。
	// 冲突目标缺失且需要 DO UPDATE 时，PostgreSQL 和 SQLite 返回错误。
	RenderUpsert(rc *RenderContext, clause UpsertClause) (string, error)
	// RenderExcluded 渲染冲突更新里本次插入的列值。
	// MySQL 返回 VALUES(quoted)。PostgreSQL 和 SQLite 返回 EXCLUDED.quoted。
	// 列名引用不会失败，所以只返回字符串。
	RenderExcluded(rc *RenderContext, column string) string
	// RenderDelete 渲染硬删除。joins 为空时只删除目标表。
	// SQLite 在带连接时返回错误。
	RenderDelete(rc *RenderContext, target string, joins []JoinRender, where string) (string, error)
	// RenderExists 渲染存在性查询。subSQL 是已经渲染好的子查询。
	// 默认是 SELECT EXISTS (subSQL)。达梦改成 SELECT COUNT(*) FROM (subSQL) WHERE ROWNUM = 1。
	RenderExists(rc *RenderContext, subSQL string) string
	// RenderUpdate 渲染 UPDATE。where 只包含调用方自己的条件，不含连接条件。
	// 没有连接时写成 UPDATE target SET sets。
	// MySQL 有连接时把 JOIN 写在 SET 前面，并保留连接类型。
	// PostgreSQL、SQLite 和达梦在全部为内连接时使用 FROM，并把各连接的 ON 并入 WHERE，调用方条件放在最后。
	// PostgreSQL、GaussDB、SQLite 和达梦出现外连接时返回错误。
	// 带 JOIN 且 JOIN 写在 SET 前面时，方言要实现 UpdateJoinsBeforeSet。
	// 位置参数按 SQL 里问号的顺序绑定，调用方会把参数排成 JOIN、SET、WHERE。
	// 编号占位符不要实现这个方法，编号在 AddArg 时已经写进 SQL。
	RenderUpdate(rc *RenderContext, target, sets, where string, joins []JoinRender) (string, error)
	// JoinKeyword 改写连接关键字。大多数方言原样返回。
	JoinKeyword(keyword string) string
	// SupportsGroup 检查 GROUP BY 扩展。不支持时返回错误。
	SupportsGroup(ext string) error
	// RollupWith 为 true 时，ROLLUP 渲染为 GROUP BY cols WITH ROLLUP。
	RollupWith() bool
	// OmitLock 为 true 时，渲染阶段丢弃行锁子句。
	OmitLock() bool
	// QuoteLiteral 按该方言转义字符串字面量，结果含两端引号。
	// MySQL 同时转义反斜杠和单引号，对应默认 sql_mode（反斜杠是转义符）。
	// NO_BACKSLASH_ESCAPES 打开时反斜杠是普通字符，这里仍会加倍，从而多写出一个反斜杠。
	// PostgreSQL 和 SQLite 只把单引号加倍。PostgreSQL 默认 standard_conforming_strings=on，反斜杠不再加倍。
	QuoteLiteral(s string) string
}

var (
	dialectRegistryMu sync.RWMutex
	dialectRegistry   = map[Dialect]DialectSpec{}
)

func renderRowLockWait(dialect Dialect, wait RowLockWait) (string, error) {
	sqlText := RowLockWaitSQL(wait)
	if sqlText == "" {
		return "", fmt.Errorf("gooq: row lock wait %d is not supported by dialect %s", wait, dialect)
	}
	return sqlText, nil
}

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
	RegisterDialect(MariaDBDialect{})
	RegisterDialect(TiDBDialect{})
	RegisterDialect(GaussDBDialect{})
	RegisterDialect(OceanBaseDialect{})
	RegisterDialect(DMDialect{})
}

func castType(table map[LocalType]string, localType LocalType) string {
	if v, ok := table[localType]; ok {
		return v
	}
	return strings.ToUpper(string(localType))
}

func renderLimitOffset(_ *RenderContext, limit, offset int) string {
	if limit <= 0 && offset > 0 {
		return fmt.Sprintf(" OFFSET %d", offset)
	}
	if offset > 0 {
		return fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}
	return fmt.Sprintf(" LIMIT %d", limit)
}

func renderConflict(rc *RenderContext, clause UpsertClause) (string, error) {
	if clause.DoNothing && len(clause.ConflictCols) == 0 {
		return " ON CONFLICT DO NOTHING", nil
	}
	if !clause.DoNothing && len(clause.ConflictCols) == 0 {
		return "", fmt.Errorf("gooq: ON CONFLICT DO UPDATE requires a conflict target")
	}
	quoted := make([]string, len(clause.ConflictCols))
	for i, col := range clause.ConflictCols {
		quoted[i] = rc.Quote(col)
	}
	target := strings.Join(quoted, ", ")
	if clause.DoNothing {
		return fmt.Sprintf(" ON CONFLICT (%s) DO NOTHING", target), nil
	}
	sets, err := renderUpsertSets(rc, clause.Updates)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(" ON CONFLICT (%s) DO UPDATE SET %s", target, sets), nil
}

func renderUpsertSets(rc *RenderContext, updates map[string]any) (string, error) {
	if len(updates) == 0 {
		return "", fmt.Errorf("gooq: conflict update has no assignments")
	}
	cols := sortedMapKeys(updates)
	sets := make([]string, 0, len(cols))
	for _, col := range cols {
		valueSQL, err := renderUpsertValue(rc, updates[col])
		if err != nil {
			return "", err
		}
		sets = append(sets, fmt.Sprintf("%s = %s", rc.Quote(col), valueSQL))
	}
	return strings.Join(sets, ", "), nil
}

func renderUpsertValue(rc *RenderContext, value any) (string, error) {
	if excluded, ok := value.(*excludedExpr); ok {
		if spec := rc.Spec(); spec != nil {
			return spec.RenderExcluded(rc, excluded.column), nil
		}
		return "", nil
	}
	if expr, ok := value.(Expression); ok {
		return rc.Render(expr), nil
	}
	return rc.AddArg(value), nil
}

func quoteLiteralStandard(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func quoteLiteralMySQL(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `'`, `''`)
	return "'" + replacer.Replace(s) + "'"
}

func renderSelectExists(subSQL string) string {
	return "SELECT EXISTS (" + subSQL + ")"
}

func canonicalDateUnit(unit string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(unit)) {
	case "YEAR", "MONTH", "DAY", "HOUR", "MINUTE", "SECOND":
		return strings.ToUpper(strings.TrimSpace(unit)), nil
	default:
		return "", fmt.Errorf("gooq: DATE_ADD unit %q is not supported", unit)
	}
}

func deleteTargetAlias(target string) string {
	const marker = " AS "
	if index := strings.LastIndex(target, marker); index >= 0 {
		return target[index+len(marker):]
	}
	return target
}

func renderJoined(joins []JoinRender) string {
	parts := make([]string, 0, len(joins))
	for _, join := range joins {
		part := join.Keyword + " " + join.Table
		if join.On != "" {
			part += " ON " + join.On
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, " ")
}

func renderDeleteFrom(target, where string) string {
	sqlStr := "DELETE FROM " + target
	if where != "" {
		sqlStr += " WHERE " + where
	}
	return sqlStr
}

func renderUpdateBase(target, sets, where string) string {
	sqlStr := "UPDATE " + target + " SET " + sets
	if where != "" {
		sqlStr += " WHERE " + where
	}
	return sqlStr
}

func joinsHaveOuter(joins []JoinRender) bool {
	for _, join := range joins {
		switch {
		case strings.HasPrefix(join.Keyword, "LEFT"),
			strings.HasPrefix(join.Keyword, "RIGHT"),
			strings.HasPrefix(join.Keyword, "FULL"):
			return true
		}
	}
	return false
}

func renderUpdateFrom(target, sets, where string, joins []JoinRender) string {
	if len(joins) == 0 {
		return renderUpdateBase(target, sets, where)
	}
	tables := make([]string, len(joins))
	ons := make([]string, 0, len(joins)+1)
	for i, join := range joins {
		tables[i] = join.Table
		if join.On != "" {
			ons = append(ons, join.On)
		}
	}
	sqlStr := "UPDATE " + target + " SET " + sets + " FROM " + strings.Join(tables, ", ")
	if where != "" {
		ons = append(ons, where)
	}
	if len(ons) > 0 {
		sqlStr += " WHERE " + strings.Join(ons, " AND ")
	}
	return sqlStr
}

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
