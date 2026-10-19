package gooq

import (
	"fmt"
	"strings"
)

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

func (MySQLDialect) Name() Dialect { return DialectMySQL }

func (MySQLDialect) Quote(ident string) string {
	return "`" + strings.ReplaceAll(ident, "`", "``") + "`"
}

// FoldIdent 为 true：列名比较忽略大小写。
// MySQL 家族的列名不区分大小写，反引号也不改变这一点。
// 自定义方言如果同样折叠标识符，也要实现这个方法。
func (MySQLDialect) FoldIdent() bool { return true }

func (MySQLDialect) Placeholder(int) string { return "?" }

func (MySQLDialect) CastType(localType LocalType) string {
	return castType(mysqlCastTypes, localType)
}

func (MySQLDialect) ShareLock() string { return "LOCK IN SHARE MODE" }

func (MySQLDialect) RenderExists(_ *RenderContext, subSQL string) string {
	return renderSelectExists(subSQL)
}

func (MySQLDialect) Returning() string { return "" }

// RenderLimit 渲染 LIMIT/OFFSET。只有偏移时用最大无符号 bigint 表示不限制行数。
func (MySQLDialect) RenderLimit(rc *RenderContext, limit, offset int) string {
	if limit <= 0 && offset > 0 {
		return fmt.Sprintf(" LIMIT 18446744073709551615 OFFSET %d", offset)
	}
	return renderLimitOffset(rc, limit, offset)
}

func (MySQLDialect) InsertKeyword(doNothing bool) string {
	if doNothing {
		return "INSERT IGNORE"
	}
	return "INSERT"
}

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

func (MySQLDialect) RenderExcluded(rc *RenderContext, column string) string {
	return "VALUES(" + rc.Quote(column) + ")"
}

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

func (MySQLDialect) RenderIfNull(a, b string) string {
	return fmt.Sprintf("IFNULL(%s, %s)", a, b)
}

func (MySQLDialect) RenderCurDate() string { return "CURDATE()" }

func (MySQLDialect) RenderDateAdd(field, count, unit string) (string, error) {
	canonical, err := canonicalDateUnit(unit)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("DATE_ADD(%s, INTERVAL %s %s)", field, count, canonical), nil
}

func (MySQLDialect) RenderDateDiff(a, b string) string {
	return fmt.Sprintf("DATEDIFF(%s, %s)", a, b)
}

func (MySQLDialect) RenderRound(field, decimals string) string {
	return fmt.Sprintf("ROUND(%s, %s)", field, decimals)
}

// UpdateJoinsBeforeSet 为 true：带 JOIN 的 UPDATE 把连接写在 SET 前面。
// 调用方据此把位置参数排成 JOIN、SET、WHERE。
// 自定义方言如果写出同样的语句形状，也要实现这个方法。编号占位符不要实现。
func (MySQLDialect) UpdateJoinsBeforeSet() bool { return true }

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

func (MySQLDialect) JoinKeyword(keyword string) string { return keyword }

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

func (MySQLDialect) RollupWith() bool { return true }

func (MySQLDialect) OmitLock() bool { return false }

// RenderRowLockWait 渲染 SKIP LOCKED 或 NOWAIT。MySQL 家族的语句形状相同。
func (MySQLDialect) RenderRowLockWait(wait RowLockWait) (string, error) {
	return renderRowLockWait(DialectMySQL, wait)
}

func (MySQLDialect) RenderDateFormat(fieldSQL, format string) (string, error) {
	return fmt.Sprintf("DATE_FORMAT(%s, %s)", fieldSQL, quoteLiteralMySQL(format)), nil
}

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

func (MySQLDialect) RenderMedian(fieldSQL string) string {
	return fmt.Sprintf("MEDIAN(%s)", fieldSQL)
}
