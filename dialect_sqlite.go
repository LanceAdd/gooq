package gooq

import (
	"fmt"
	"strings"
)

// SQLiteDialect 是 SQLite 的 SQL 渲染实现。
type SQLiteDialect struct{}

var sqliteCastTypes = map[LocalType]string{
	LocalTypeString:   "TEXT",
	LocalTypeInt:      "INTEGER",
	LocalTypeInt32:    "INTEGER",
	LocalTypeInt64:    "INTEGER",
	LocalTypeBigInt:   "INTEGER",
	LocalTypeUint:     "INTEGER",
	LocalTypeUint32:   "INTEGER",
	LocalTypeUint64:   "INTEGER",
	LocalTypeFloat32:  "REAL",
	LocalTypeFloat64:  "REAL",
	LocalTypeBool:     "INTEGER",
	LocalTypeDate:     "TEXT",
	LocalTypeDatetime: "TEXT",
	LocalTypeTime:     "TEXT",
	LocalTypeJson:     "TEXT",
	LocalTypeJsonb:    "TEXT",
	LocalTypeBytes:    "BLOB",
	LocalTypeUUID:     "TEXT",
}

var sqliteFormatReplacer = strings.NewReplacer(
	"%i", "%M",
	"%s", "%S",
)

const joinInnerLateral = "INNER JOIN LATERAL"

// Name 返回 sqlite。
func (SQLiteDialect) Name() Dialect { return DialectSQLite }

// Quote 使用双引号。
func (SQLiteDialect) Quote(ident string) string { return `"` + ident + `"` }

// Placeholder 返回问号占位符。
func (SQLiteDialect) Placeholder(int) string { return "?" }

// CastType 返回 SQLite 的 CAST 类型名。
func (SQLiteDialect) CastType(localType LocalType) string {
	return castType(sqliteCastTypes, localType)
}

// ShareLock 返回空字符串。SQLite 渲染会丢掉行锁。
func (SQLiteDialect) ShareLock() string { return "" }

// Returning 返回 RETURNING。
func (SQLiteDialect) Returning() string { return "RETURNING" }

// RenderLimit 渲染 LIMIT/OFFSET。只有偏移时不写 LIMIT。
func (SQLiteDialect) RenderLimit(rc *RenderContext, limit, offset int) string {
	return renderLimitOffset(rc, limit, offset)
}

// InsertKeyword 返回 INSERT。冲突动作由 ON CONFLICT 表达。
func (SQLiteDialect) InsertKeyword(bool) string { return "INSERT" }

// RenderUpsert 渲染 ON CONFLICT。
func (SQLiteDialect) RenderUpsert(rc *RenderContext, clause UpsertClause) (string, error) {
	return renderConflict(rc, clause)
}

// RenderDelete 渲染 DELETE。SQLite 不支持带连接的删除。
func (SQLiteDialect) RenderDelete(rc *RenderContext, target string, joins []JoinRender, where string) (string, error) {
	if len(joins) > 0 {
		return "", fmt.Errorf("gooq: DELETE with JOIN is not supported by dialect %s", DialectSQLite)
	}
	return renderDeleteFrom(target, where), nil
}

// QuoteLiteral 转义 SQLite 字符串字面量。只加倍单引号。
func (SQLiteDialect) QuoteLiteral(s string) string { return quoteLiteralStandard(s) }

// RenderIfNull 渲染 COALESCE。
func (SQLiteDialect) RenderIfNull(a, b string) string {
	return fmt.Sprintf("COALESCE(%s, %s)", a, b)
}

// RenderCurDate 渲染 date('now')。
func (SQLiteDialect) RenderCurDate() string { return "date('now')" }

// RenderDateAdd 渲染 datetime 与带符号的单位修饰符。
func (SQLiteDialect) RenderDateAdd(field, count, unit string) (string, error) {
	canonical, err := canonicalDateUnit(unit)
	if err != nil {
		return "", err
	}
	name := map[string]string{
		"YEAR": "years", "MONTH": "months", "DAY": "days",
		"HOUR": "hours", "MINUTE": "minutes", "SECOND": "seconds",
	}[canonical]
	return fmt.Sprintf("datetime(%s, printf('%%+d %s', %s))", field, name, count), nil
}

// RenderDateDiff 用 julianday 计算相差的天数。
func (SQLiteDialect) RenderDateDiff(a, b string) string {
	return fmt.Sprintf("CAST((julianday(%s) - julianday(%s)) AS INTEGER)", a, b)
}

// RenderRound 渲染 ROUND。
func (SQLiteDialect) RenderRound(field, decimals string) string {
	return fmt.Sprintf("ROUND(%s, %s)", field, decimals)
}

// UpdateFrom 为 true。多表 UPDATE 使用 FROM。
func (SQLiteDialect) UpdateFrom() bool { return true }

// JoinKeyword 把 INNER JOIN LATERAL 改成 CROSS JOIN LATERAL。
func (SQLiteDialect) JoinKeyword(keyword string) string {
	if keyword == joinInnerLateral {
		return "CROSS JOIN LATERAL"
	}
	return keyword
}

// SupportsGroup 接受全部 GROUP BY 扩展。
func (SQLiteDialect) SupportsGroup(string) error { return nil }

// RollupWith 为 false。ROLLUP 使用函数形式。
func (SQLiteDialect) RollupWith() bool { return false }

// OmitLock 为 true。SQLite 不渲染行锁。
func (SQLiteDialect) OmitLock() bool { return true }

// RenderDateFormat 把 MySQL 格式符转换成 strftime。
func (SQLiteDialect) RenderDateFormat(fieldSQL, format string) string {
	return fmt.Sprintf("strftime(%s, %s)", quoteLiteralStandard(sqliteFormatReplacer.Replace(format)), fieldSQL)
}

// RenderGroupConcat 渲染 GROUP_CONCAT。SQLite 不支持函数内 ORDER BY，调用方传入的排序被忽略。
// DISTINCT 只能有一个参数。未给分隔符时渲染单参数；给了分隔符则在渲染期报错。
func (SQLiteDialect) RenderGroupConcat(in GroupConcatInput) (string, error) {
	if in.Distinct && in.Separator != "" {
		return "", fmt.Errorf("gooq: GROUP_CONCAT DISTINCT with a separator is not supported by dialect %s", DialectSQLite)
	}
	if in.Distinct {
		return fmt.Sprintf("GROUP_CONCAT(DISTINCT %s)", in.FieldSQL), nil
	}
	separator := in.Separator
	if separator == "" {
		separator = ","
	}
	return fmt.Sprintf("GROUP_CONCAT(%s, %s)", in.FieldSQL, quoteLiteralStandard(separator)), nil
}

// RenderMedian 渲染 MEDIAN。
func (SQLiteDialect) RenderMedian(fieldSQL string) string {
	return fmt.Sprintf("MEDIAN(%s)", fieldSQL)
}
