package gooq

import (
	"fmt"
	"strings"
)

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

func sqliteDateFormat(format string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			b.WriteByte(format[i])
			continue
		}
		if i+1 >= len(format) {
			return "", fmt.Errorf("gooq: DATE_FORMAT specifier %% is not supported by dialect %s", DialectSQLite)
		}
		token := format[i : i+2]
		i++
		switch token {
		case "%%":
			b.WriteString("%%")
		case "%Y", "%m", "%d", "%H", "%j", "%w":
			b.WriteString(token)
		case "%i":
			b.WriteString("%M")
		case "%s", "%S":
			b.WriteString("%S")
		default:
			return "", fmt.Errorf("gooq: DATE_FORMAT specifier %s is not supported by dialect %s", token, DialectSQLite)
		}
	}
	return b.String(), nil
}

func (SQLiteDialect) Name() Dialect { return DialectSQLite }

func (SQLiteDialect) Quote(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

func (SQLiteDialect) Placeholder(int) string { return "?" }

func (SQLiteDialect) CastType(localType LocalType) string {
	return castType(sqliteCastTypes, localType)
}

func (SQLiteDialect) ShareLock() string { return "" }

func (SQLiteDialect) RenderExists(_ *RenderContext, subSQL string) string {
	return renderSelectExists(subSQL)
}

func (SQLiteDialect) Returning() string { return "RETURNING" }

func (SQLiteDialect) RenderLimit(rc *RenderContext, limit, offset int) string {
	return renderLimitOffset(rc, limit, offset)
}

func (SQLiteDialect) InsertKeyword(bool) string { return "INSERT" }

func (SQLiteDialect) RenderUpsert(rc *RenderContext, clause UpsertClause) (string, error) {
	return renderConflict(rc, clause)
}

func (SQLiteDialect) RenderExcluded(rc *RenderContext, column string) string {
	return "EXCLUDED." + rc.Quote(column)
}

func (SQLiteDialect) RenderDelete(rc *RenderContext, target string, joins []JoinRender, where string) (string, error) {
	if len(joins) > 0 {
		return "", fmt.Errorf("gooq: DELETE with JOIN is not supported by dialect %s", DialectSQLite)
	}
	return renderDeleteFrom(target, where), nil
}

func (SQLiteDialect) QuoteLiteral(s string) string { return quoteLiteralStandard(s) }

func (SQLiteDialect) RenderIfNull(a, b string) string {
	return fmt.Sprintf("COALESCE(%s, %s)", a, b)
}

func (SQLiteDialect) RenderCurDate() string { return "date('now')" }

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

func (SQLiteDialect) RenderDateDiff(a, b string) string {
	return fmt.Sprintf("CAST((julianday(%s) - julianday(%s)) AS INTEGER)", a, b)
}

func (SQLiteDialect) RenderRound(field, decimals string) string {
	return fmt.Sprintf("ROUND(%s, %s)", field, decimals)
}

func (SQLiteDialect) RenderUpdate(_ *RenderContext, target, sets, where string, joins []JoinRender) (string, error) {
	if joinsHaveOuter(joins) {
		return "", fmt.Errorf("gooq: UPDATE with an outer join is not supported by dialect %s", DialectSQLite)
	}
	return renderUpdateFrom(target, sets, where, joins), nil
}

// JoinKeyword 原样返回连接关键字。LATERAL 由渲染阶段拒绝。
func (SQLiteDialect) JoinKeyword(keyword string) string { return keyword }

func (SQLiteDialect) SupportsGroup(ext string) error {
	if ext == "" {
		return nil
	}
	return fmt.Errorf("gooq: %s is not supported by dialect %s", ext, DialectSQLite)
}

func (SQLiteDialect) RollupWith() bool { return false }

func (SQLiteDialect) OmitLock() bool { return true }

func (SQLiteDialect) RenderDateFormat(fieldSQL, format string) (string, error) {
	converted, err := sqliteDateFormat(format)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("strftime(%s, %s)", quoteLiteralStandard(converted), fieldSQL), nil
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

func (SQLiteDialect) RenderMedian(fieldSQL string) string {
	return fmt.Sprintf("MEDIAN(%s)", fieldSQL)
}
