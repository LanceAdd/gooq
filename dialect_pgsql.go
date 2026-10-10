package gooq

import (
	"fmt"
	"strings"
)

// PostgresDialect 是 PostgreSQL 的 SQL 渲染实现。
type PostgresDialect struct{}

var postgresCastTypes = map[LocalType]string{
	LocalTypeString:   "TEXT",
	LocalTypeInt:      "INTEGER",
	LocalTypeInt32:    "INTEGER",
	LocalTypeInt64:    "BIGINT",
	LocalTypeBigInt:   "BIGINT",
	LocalTypeUint:     "BIGINT",
	LocalTypeUint32:   "BIGINT",
	LocalTypeUint64:   "NUMERIC(20)",
	LocalTypeFloat32:  "REAL",
	LocalTypeFloat64:  "DOUBLE PRECISION",
	LocalTypeBool:     "BOOLEAN",
	LocalTypeDate:     "DATE",
	LocalTypeDatetime: "TIMESTAMP",
	LocalTypeTime:     "TIME",
	LocalTypeJson:     "JSON",
	LocalTypeJsonb:    "JSONB",
	LocalTypeBytes:    "BYTEA",
	LocalTypeUUID:     "UUID",
}

var pgFormatReplacer = strings.NewReplacer(
	"%Y", "YYYY", "%y", "YY",
	"%m", "MM", "%d", "DD",
	"%H", "HH24", "%i", "MI", "%s", "SS",
	"%e", "FMDD", "%j", "DDD",
)

// Name 返回 pgsql。
func (PostgresDialect) Name() Dialect { return DialectPgsql }

// Quote 使用双引号。
func (PostgresDialect) Quote(ident string) string { return `"` + ident + `"` }

// Placeholder 返回 $n 占位符。
func (PostgresDialect) Placeholder(n int) string { return fmt.Sprintf("$%d", n) }

// CastType 返回 PostgreSQL 的 CAST 类型名。
func (PostgresDialect) CastType(localType LocalType) string {
	return castType(postgresCastTypes, localType)
}

// ShareLock 返回 FOR SHARE。
func (PostgresDialect) ShareLock() string { return "FOR SHARE" }

// Returning 返回 RETURNING。
func (PostgresDialect) Returning() string { return "RETURNING" }

// RenderLimit 渲染 LIMIT/OFFSET。
func (PostgresDialect) RenderLimit(rc *RenderContext, limit, offset int) string {
	return renderLimitOffset(rc, limit, offset)
}

// InsertKeyword 返回 INSERT。冲突动作由 ON CONFLICT 表达。
func (PostgresDialect) InsertKeyword(bool) string { return "INSERT" }

// RenderUpsert 渲染 ON CONFLICT。
func (PostgresDialect) RenderUpsert(rc *RenderContext, clause UpsertClause) string {
	return renderConflict(rc, clause)
}

// UpdateFrom 为 true。多表 UPDATE 使用 FROM。
func (PostgresDialect) UpdateFrom() bool { return true }

// JoinKeyword 原样返回连接关键字。
func (PostgresDialect) JoinKeyword(keyword string) string { return keyword }

// SupportsGroup 接受全部 GROUP BY 扩展。
func (PostgresDialect) SupportsGroup(string) error { return nil }

// RollupWith 为 false。ROLLUP 使用函数形式。
func (PostgresDialect) RollupWith() bool { return false }

// OmitLock 为 false。PostgreSQL 保留行锁子句。
func (PostgresDialect) OmitLock() bool { return false }

// RenderDateFormat 把 MySQL 格式符转换成 TO_CHAR。
func (PostgresDialect) RenderDateFormat(fieldSQL, format string) string {
	return fmt.Sprintf("TO_CHAR(%s, '%s')", fieldSQL, pgFormatReplacer.Replace(format))
}

// RenderGroupConcat 渲染 STRING_AGG。DISTINCT 返回错误。
func (PostgresDialect) RenderGroupConcat(in GroupConcatInput) (string, error) {
	if in.Distinct {
		return "", fmt.Errorf("gooq: GROUP_CONCAT DISTINCT is not supported by dialect %s", DialectPgsql)
	}
	separator := in.Separator
	if separator == "" {
		separator = ","
	}
	sqlStr := fmt.Sprintf("STRING_AGG(%s, '%s'", in.FieldSQL, separator)
	if in.OrderSQL != "" {
		sqlStr += " ORDER BY " + in.OrderSQL
	}
	return sqlStr + ")", nil
}

// RenderMedian 渲染为 PERCENTILE_CONT。
func (PostgresDialect) RenderMedian(fieldSQL string) string {
	return fmt.Sprintf("PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY %s)", fieldSQL)
}
