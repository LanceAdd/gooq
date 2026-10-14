package gooq

import (
	"fmt"
	"strings"
)

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

func (PostgresDialect) Name() Dialect { return DialectPgsql }

func (PostgresDialect) Quote(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

func (PostgresDialect) Placeholder(n int) string { return fmt.Sprintf("$%d", n) }

func (PostgresDialect) CastType(localType LocalType) string {
	return castType(postgresCastTypes, localType)
}

func (PostgresDialect) ShareLock() string { return "FOR SHARE" }

func (PostgresDialect) RenderExists(_ *RenderContext, subSQL string) string {
	return renderSelectExists(subSQL)
}

func (PostgresDialect) Returning() string { return "RETURNING" }

func (PostgresDialect) RenderLimit(rc *RenderContext, limit, offset int) string {
	return renderLimitOffset(rc, limit, offset)
}

func (PostgresDialect) InsertKeyword(bool) string { return "INSERT" }

func (PostgresDialect) RenderUpsert(rc *RenderContext, clause UpsertClause) (string, error) {
	return renderConflict(rc, clause)
}

func (PostgresDialect) RenderExcluded(rc *RenderContext, column string) string {
	return "EXCLUDED." + rc.Quote(column)
}

// GaussDB 嵌入本方法，错误里的方言名取渲染上下文。
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

func (PostgresDialect) QuoteLiteral(s string) string { return quoteLiteralStandard(s) }

func (PostgresDialect) RenderIfNull(a, b string) string {
	return fmt.Sprintf("COALESCE(%s, %s)", a, b)
}

func (PostgresDialect) RenderCurDate() string { return "CURRENT_DATE" }

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

func (PostgresDialect) RenderDateDiff(a, b string) string {
	return fmt.Sprintf("(%s)::date - (%s)::date", a, b)
}

// RenderRound 先转成 numeric，避免 round(double precision, integer)。
func (PostgresDialect) RenderRound(field, decimals string) string {
	return fmt.Sprintf("ROUND((%s)::numeric, %s)", field, decimals)
}

// GaussDB 嵌入本方法，错误里的方言名取渲染上下文。
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

func (PostgresDialect) JoinKeyword(keyword string) string { return keyword }

func (PostgresDialect) SupportsGroup(string) error { return nil }

func (PostgresDialect) RollupWith() bool { return false }

func (PostgresDialect) OmitLock() bool { return false }

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

func (PostgresDialect) RenderMedian(fieldSQL string) string {
	return fmt.Sprintf("PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY %s)", fieldSQL)
}
