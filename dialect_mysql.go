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
func (MySQLDialect) Quote(ident string) string { return "`" + ident + "`" }

// Placeholder 返回问号占位符。
func (MySQLDialect) Placeholder(int) string { return "?" }

// CastType 返回 MySQL 的 CAST 类型名。
func (MySQLDialect) CastType(localType LocalType) string {
	return castType(mysqlCastTypes, localType)
}

// ShareLock 返回 LOCK IN SHARE MODE。
func (MySQLDialect) ShareLock() string { return "LOCK IN SHARE MODE" }

// Returning 返回空字符串。MySQL 不支持 RETURNING。
func (MySQLDialect) Returning() string { return "" }

// RenderLimit 渲染 LIMIT/OFFSET。
func (MySQLDialect) RenderLimit(rc *RenderContext, limit, offset int) string {
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
func (MySQLDialect) RenderUpsert(rc *RenderContext, clause UpsertClause) string {
	if clause.DoNothing {
		return ""
	}
	cols := sortedMapKeys(clause.Updates)
	sets := make([]string, 0, len(cols))
	for _, col := range cols {
		sets = append(sets, fmt.Sprintf("%s = VALUES(%s)", rc.Quote(col), rc.Quote(col)))
	}
	return " ON DUPLICATE KEY UPDATE " + strings.Join(sets, ", ")
}

// UpdateFrom 为 false。MySQL 多表 UPDATE 使用 JOIN。
func (MySQLDialect) UpdateFrom() bool { return false }

// JoinKeyword 原样返回连接关键字。
func (MySQLDialect) JoinKeyword(keyword string) string { return keyword }

// SupportsGroup 拒绝 CUBE 和 GROUPING SETS。
func (MySQLDialect) SupportsGroup(ext string) error {
	switch ext {
	case GroupCube, GroupGroupingSets:
		return fmt.Errorf("gooq: GROUP BY %s is not supported by dialect %s", ext, DialectMySQL)
	default:
		return nil
	}
}

// RollupWith 为 true。ROLLUP 渲染为 WITH ROLLUP。
func (MySQLDialect) RollupWith() bool { return true }

// OmitLock 为 false。MySQL 保留行锁子句。
func (MySQLDialect) OmitLock() bool { return false }

// RenderDateFormat 渲染 DATE_FORMAT。
func (MySQLDialect) RenderDateFormat(fieldSQL, format string) string {
	return fmt.Sprintf("DATE_FORMAT(%s, '%s')", fieldSQL, format)
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
		sqlStr += " SEPARATOR '" + in.Separator + "'"
	}
	return sqlStr + ")", nil
}

// RenderMedian 渲染 MEDIAN。
func (MySQLDialect) RenderMedian(fieldSQL string) string {
	return fmt.Sprintf("MEDIAN(%s)", fieldSQL)
}
