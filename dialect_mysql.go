package gooq

import (
	"fmt"
	"strings"
)

// MySQLDialect 是 MySQL 的 SQL 渲染实现。
type MySQLDialect struct{}

var mysqlCastTypes = map[LocalType]string{
	LocalTypeString:   "CHAR",
	LocalTypeInt:      "SIGNED",
	LocalTypeInt32:    "SIGNED",
	LocalTypeInt64:    "SIGNED",
	LocalTypeBigInt:   "SIGNED",
	LocalTypeUint:     "UNSIGNED",
	LocalTypeUint32:   "UNSIGNED",
	LocalTypeUint64:   "UNSIGNED",
	LocalTypeFloat32:  "DECIMAL(10, 4)",
	LocalTypeFloat64:  "DECIMAL(10, 4)",
	LocalTypeBool:     "UNSIGNED",
	LocalTypeDate:     "DATE",
	LocalTypeDatetime: "DATETIME",
	LocalTypeTime:     "TIME",
	LocalTypeJson:     "CHAR",
	LocalTypeJsonb:    "CHAR",
	LocalTypeBytes:    "BINARY",
	LocalTypeUUID:     "CHAR(36)",
}

// Name 返回 mysql。
func (MySQLDialect) Name() Dialect { return DialectMySQL }

// Quote 使用反引号。
func (MySQLDialect) Quote(ident string) string {
	return "`" + strings.ReplaceAll(ident, "`", "``") + "`"
}

// Placeholder 返回问号占位符。
func (MySQLDialect) Placeholder(int) string { return "?" }

// CastType 返回 MySQL 的 CAST 类型名。
func (MySQLDialect) CastType(localType LocalType) string {
	return castType(mysqlCastTypes, localType)
}

// ShareLock 返回 LOCK IN SHARE MODE。
func (MySQLDialect) ShareLock() string { return "LOCK IN SHARE MODE" }

func (MySQLDialect) RenderExists(_ *RenderContext, subSQL string) string {
	return renderSelectExists(subSQL)
}

// Returning 返回空字符串。MySQL 不支持 RETURNING。
func (MySQLDialect) Returning() string { return "" }

// RenderLimit 渲染 LIMIT/OFFSET。只有偏移时用最大无符号 bigint 表示不限制行数。
func (MySQLDialect) RenderLimit(rc *RenderContext, limit, offset int) string {
	if limit <= 0 && offset > 0 {
		return fmt.Sprintf(" LIMIT 18446744073709551615 OFFSET %d", offset)
	}
	return renderLimitOffset(rc, limit, offset)
}

// InsertKeyword 在冲突忽略时返回 INSERT IGNORE。
func (MySQLDialect) InsertKeyword(doNothing bool) string {
	if doNothing {
		return "INSERT IGNORE"
	}
	return "INSERT"
}

// RenderUpsert 渲染 ON DUPLICATE KEY UPDATE。INSERT IGNORE 不再追加子句。
// MySQL 没有冲突目标，DoUpdate 不需要 OnConflictKey。
func (MySQLDialect) RenderUpsert(rc *RenderContext, clause UpsertClause) (string, error) {
	if clause.DoNothing {
		return "", nil
	}
	sets, err := renderUpsertSets(rc, clause.Updates)
	if err != nil {
		return "", err
	}
	return " ON DUPLICATE KEY UPDATE " + sets, nil
}

// RenderExcluded 渲染本次插入的列值。
func (MySQLDialect) RenderExcluded(rc *RenderContext, column string) string {
	return "VALUES(" + rc.Quote(column) + ")"
}

// RenderDelete 渲染 DELETE。带连接时写出 DELETE alias FROM ... JOIN。
func (MySQLDialect) RenderDelete(rc *RenderContext, target string, joins []JoinRender, where string) (string, error) {
	if len(joins) == 0 {
		return renderDeleteFrom(target, where), nil
	}
	sqlStr := "DELETE " + deleteTargetAlias(target) + " FROM " + target + " " + renderJoined(joins)
	if where != "" {
		sqlStr += " WHERE " + where
	}
	return sqlStr, nil
}

// QuoteLiteral 转义 MySQL 字符串字面量。
// 默认 sql_mode 把反斜杠当转义符，所以同时转义 \ 和 '。
// NO_BACKSLASH_ESCAPES 打开时反斜杠是普通字符，这里仍会加倍，多写出一个反斜杠。
func (MySQLDialect) QuoteLiteral(s string) string { return quoteLiteralMySQL(s) }

// RenderIfNull 渲染 IFNULL。
func (MySQLDialect) RenderIfNull(a, b string) string {
	return fmt.Sprintf("IFNULL(%s, %s)", a, b)
}

// RenderCurDate 渲染 CURDATE()。
func (MySQLDialect) RenderCurDate() string { return "CURDATE()" }

// RenderDateAdd 渲染 DATE_ADD。单位必须在白名单内。
func (MySQLDialect) RenderDateAdd(field, count, unit string) (string, error) {
	canonical, err := canonicalDateUnit(unit)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("DATE_ADD(%s, INTERVAL %s %s)", field, count, canonical), nil
}

// RenderDateDiff 渲染 DATEDIFF。
func (MySQLDialect) RenderDateDiff(a, b string) string {
	return fmt.Sprintf("DATEDIFF(%s, %s)", a, b)
}

// RenderRound 渲染 ROUND。
func (MySQLDialect) RenderRound(field, decimals string) string {
	return fmt.Sprintf("ROUND(%s, %s)", field, decimals)
}

// RenderUpdate 渲染 UPDATE。有连接时写成 UPDATE target JOIN ... SET。
func (MySQLDialect) RenderUpdate(_ *RenderContext, target, sets, where string, joins []JoinRender) (string, error) {
	if len(joins) == 0 {
		return renderUpdateBase(target, sets, where), nil
	}
	sqlStr := "UPDATE " + target + " " + renderJoined(joins) + " SET " + sets
	if where != "" {
		sqlStr += " WHERE " + where
	}
	return sqlStr, nil
}

// JoinKeyword 原样返回连接关键字。
func (MySQLDialect) JoinKeyword(keyword string) string { return keyword }

// SupportsGroup 拒绝 CUBE 和 GROUPING SETS。
func (MySQLDialect) SupportsGroup(ext string) error {
	return rejectMySQLGroups(DialectMySQL, ext)
}

func rejectMySQLGroups(dialect Dialect, ext string) error {
	switch ext {
	case GroupCube, GroupGroupingSets:
		return fmt.Errorf("gooq: GROUP BY %s is not supported by dialect %s", ext, dialect)
	default:
		return nil
	}
}

// RollupWith 为 true。ROLLUP 渲染为 WITH ROLLUP。
func (MySQLDialect) RollupWith() bool { return true }

// OmitLock 为 false。MySQL 保留行锁子句。
func (MySQLDialect) OmitLock() bool { return false }

// RenderDateFormat 渲染 DATE_FORMAT。
func (MySQLDialect) RenderDateFormat(fieldSQL, format string) (string, error) {
	return fmt.Sprintf("DATE_FORMAT(%s, %s)", fieldSQL, quoteLiteralMySQL(format)), nil
}

// RenderGroupConcat 渲染带 DISTINCT、ORDER BY 和 SEPARATOR 的 GROUP_CONCAT。
func (MySQLDialect) RenderGroupConcat(in GroupConcatInput) (string, error) {
	distinct := ""
	if in.Distinct {
		distinct = "DISTINCT "
	}
	sqlStr := fmt.Sprintf("GROUP_CONCAT(%s%s", distinct, in.FieldSQL)
	if in.OrderSQL != "" {
		sqlStr += " ORDER BY " + in.OrderSQL
	}
	if in.Separator != "" && in.Separator != "," {
		sqlStr += " SEPARATOR " + quoteLiteralMySQL(in.Separator)
	}
	return sqlStr + ")", nil
}

// RenderMedian 渲染 MEDIAN。
func (MySQLDialect) RenderMedian(fieldSQL string) string {
	return fmt.Sprintf("MEDIAN(%s)", fieldSQL)
}
