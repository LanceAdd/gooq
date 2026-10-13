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
func (PostgresDialect) Quote(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

// Placeholder 返回 $n 占位符。
func (PostgresDialect) Placeholder(n int) string { return fmt.Sprintf("$%d", n) }

// CastType 返回 PostgreSQL 的 CAST 类型名。
func (PostgresDialect) CastType(localType LocalType) string {
	return castType(postgresCastTypes, localType)
}

// ShareLock 返回 FOR SHARE。
func (PostgresDialect) ShareLock() string { return "FOR SHARE" }

func (PostgresDialect) RenderExists(_ *RenderContext, subSQL string) string {
	return renderSelectExists(subSQL)
}

// Returning 返回 RETURNING。
func (PostgresDialect) Returning() string { return "RETURNING" }

// RenderLimit 渲染 LIMIT/OFFSET。只有偏移时不写 LIMIT。
func (PostgresDialect) RenderLimit(rc *RenderContext, limit, offset int) string {
	return renderLimitOffset(rc, limit, offset)
}

// InsertKeyword 返回 INSERT。冲突动作由 ON CONFLICT 表达。
func (PostgresDialect) InsertKeyword(bool) string { return "INSERT" }

// RenderUpsert 渲染 ON CONFLICT。
func (PostgresDialect) RenderUpsert(rc *RenderContext, clause UpsertClause) (string, error) {
	return renderConflict(rc, clause)
}

// RenderExcluded 渲染本次插入的列值。
func (PostgresDialect) RenderExcluded(rc *RenderContext, column string) string {
	return "EXCLUDED." + rc.Quote(column)
}

// RenderDelete 渲染 DELETE。没有连接时只写 FROM。
// 全部内连接时使用 USING，连接条件并入 WHERE。
// 出现外连接时返回错误。GaussDB 嵌入本方法，错误里的方言名取渲染上下文。
func (PostgresDialect) RenderDelete(rc *RenderContext, target string, joins []JoinRender, where string) (string, error) {
	if joinsHaveOuter(joins) {
		name := DialectPgsql
		if rc != nil && rc.Dialect() != "" {
			name = rc.Dialect()
		}
		return "", fmt.Errorf("gooq: DELETE with an outer join is not supported by dialect %s", name)
	}
	if len(joins) == 0 {
		return renderDeleteFrom(target, where), nil
	}
	tables := make([]string, len(joins))
	var ons []string
	for i, join := range joins {
		tables[i] = join.Table
		if join.On != "" {
			ons = append(ons, join.On)
		}
	}
	sqlStr := "DELETE FROM " + target + " USING " + strings.Join(tables, ", ")
	if where != "" {
		ons = append(ons, where)
	}
	if len(ons) > 0 {
		sqlStr += " WHERE " + strings.Join(ons, " AND ")
	}
	return sqlStr, nil
}

// QuoteLiteral 转义 PostgreSQL 字符串字面量。只加倍单引号，反斜杠保持原样。
func (PostgresDialect) QuoteLiteral(s string) string { return quoteLiteralStandard(s) }

// RenderIfNull 渲染 COALESCE。
func (PostgresDialect) RenderIfNull(a, b string) string {
	return fmt.Sprintf("COALESCE(%s, %s)", a, b)
}

// RenderCurDate 渲染 CURRENT_DATE。
func (PostgresDialect) RenderCurDate() string { return "CURRENT_DATE" }

// RenderDateAdd 渲染 make_interval。单位映射到具名参数。
func (PostgresDialect) RenderDateAdd(field, count, unit string) (string, error) {
	canonical, err := canonicalDateUnit(unit)
	if err != nil {
		return "", err
	}
	arg := map[string]string{
		"YEAR": "years", "MONTH": "months", "DAY": "days",
		"HOUR": "hours", "MINUTE": "mins", "SECOND": "secs",
	}[canonical]
	return fmt.Sprintf("(%s + make_interval(%s => %s::int))", field, arg, count), nil
}

// RenderDateDiff 渲染两个 date 相减。
func (PostgresDialect) RenderDateDiff(a, b string) string {
	return fmt.Sprintf("(%s)::date - (%s)::date", a, b)
}

// RenderRound 先转成 numeric，避免 round(double precision, integer)。
func (PostgresDialect) RenderRound(field, decimals string) string {
	return fmt.Sprintf("ROUND((%s)::numeric, %s)", field, decimals)
}

// RenderUpdate 渲染 UPDATE。全部内连接时使用 FROM，并把 ON 并入 WHERE。
// 出现外连接时返回错误。GaussDB 嵌入本方法，错误里的方言名取渲染上下文。
func (PostgresDialect) RenderUpdate(rc *RenderContext, target, sets, where string, joins []JoinRender) (string, error) {
	if joinsHaveOuter(joins) {
		name := DialectPgsql
		if rc != nil && rc.Dialect() != "" {
			name = rc.Dialect()
		}
		return "", fmt.Errorf("gooq: UPDATE with an outer join is not supported by dialect %s", name)
	}
	return renderUpdateFrom(target, sets, where, joins), nil
}

// JoinKeyword 原样返回连接关键字。
func (PostgresDialect) JoinKeyword(keyword string) string { return keyword }

// SupportsGroup 接受全部 GROUP BY 扩展。
func (PostgresDialect) SupportsGroup(string) error { return nil }

// RollupWith 为 false。ROLLUP 使用函数形式。
func (PostgresDialect) RollupWith() bool { return false }

// OmitLock 为 false。PostgreSQL 保留行锁子句。
func (PostgresDialect) OmitLock() bool { return false }

// RenderDateFormat 把 MySQL 格式符转换成 TO_CHAR。
func (PostgresDialect) RenderDateFormat(fieldSQL, format string) (string, error) {
	return fmt.Sprintf("TO_CHAR(%s, %s)", fieldSQL, quoteLiteralStandard(pgFormatReplacer.Replace(format))), nil
}

// RenderGroupConcat 渲染 STRING_AGG。DISTINCT 自 PostgreSQL 16 起可用。
func (PostgresDialect) RenderGroupConcat(in GroupConcatInput) (string, error) {
	separator := in.Separator
	if separator == "" {
		separator = ","
	}
	distinct := ""
	if in.Distinct {
		distinct = "DISTINCT "
	}
	if in.Distinct && in.OrderSQL != "" && !sameAggregateOrder(in.FieldSQL, in.OrderSQL) {
		return "", fmt.Errorf("gooq: GROUP_CONCAT DISTINCT with a different ORDER BY is not supported by dialect %s", DialectPgsql)
	}
	sqlStr := fmt.Sprintf("STRING_AGG(%s%s, %s", distinct, in.FieldSQL, quoteLiteralStandard(separator))
	if in.OrderSQL != "" {
		sqlStr += " ORDER BY " + in.OrderSQL
	}
	return sqlStr + ")", nil
}

func sameAggregateOrder(field, order string) bool {
	for _, part := range strings.Split(order, ",") {
		part = strings.TrimSpace(part)
		if part != field && part != field+" ASC" && part != field+" DESC" {
			return false
		}
	}
	return order != ""
}

// RenderMedian 渲染为 PERCENTILE_CONT。
func (PostgresDialect) RenderMedian(fieldSQL string) string {
	return fmt.Sprintf("PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY %s)", fieldSQL)
}
