package gooq

import (
	"fmt"
	"strings"
)

// DMDialect 是达梦的 SQL 渲染实现。
// 未加引号的对象名在达梦里折成大写。这里一律写成大写双引号，从而对上这种存储形式。
// 显式用小写引号建出来的对象对不上。
// 冲突插入写成 MERGE，必须给出冲突列。达梦没有 RETURNING。
type DMDialect struct{}

var dmCastTypes = map[LocalType]string{
	LocalTypeString:   "VARCHAR",
	LocalTypeInt:      "INT",
	LocalTypeInt32:    "INT",
	LocalTypeInt64:    "BIGINT",
	LocalTypeBigInt:   "BIGINT",
	LocalTypeUint:     "BIGINT",
	LocalTypeUint32:   "BIGINT",
	LocalTypeUint64:   "DECIMAL(20)",
	LocalTypeFloat32:  "REAL",
	LocalTypeFloat64:  "DOUBLE",
	LocalTypeBool:     "BIT",
	LocalTypeDate:     "DATE",
	LocalTypeDatetime: "TIMESTAMP",
	LocalTypeTime:     "TIME",
	LocalTypeJson:     "VARCHAR",
	LocalTypeJsonb:    "VARCHAR",
	LocalTypeBytes:    "VARBINARY",
	LocalTypeUUID:     "VARCHAR(36)",
}

var dmFormatReplacer = strings.NewReplacer(
	"%Y", "YYYY", "%y", "YY",
	"%m", "MM", "%d", "DD",
	"%H", "HH24", "%i", "MI", "%s", "SS",
	"%e", "FMDD", "%j", "DDD",
)

func (DMDialect) Name() Dialect { return DialectDM }

// Quote 用大写双引号包裹标识符。
func (DMDialect) Quote(ident string) string {
	return `"` + strings.ToUpper(strings.ReplaceAll(ident, `"`, `""`)) + `"`
}

func (DMDialect) Placeholder(int) string { return "?" }

func (DMDialect) CastType(localType LocalType) string {
	return castType(dmCastTypes, localType)
}

// ShareLock 返回 FOR SHARE。达梦原生语法只有 FOR UPDATE，共享锁可能被服务器拒绝。
func (DMDialect) ShareLock() string { return "FOR SHARE" }

func (DMDialect) RenderExists(_ *RenderContext, subSQL string) string {
	return "SELECT COUNT(*) FROM (" + subSQL + ") WHERE ROWNUM = 1"
}

func (DMDialect) Returning() string { return "" }

func (DMDialect) RenderLimit(rc *RenderContext, limit, offset int) string {
	if limit <= 0 && offset > 0 {
		return fmt.Sprintf(" OFFSET %d ROWS", offset)
	}
	return renderLimitOffset(rc, limit, offset)
}

// 冲突动作由 MERGE 整句表达。
func (DMDialect) InsertKeyword(bool) string { return "INSERT" }

// RenderUpsert 不生成后缀。达梦的冲突动作用 RenderInsert 写成 MERGE。
func (DMDialect) RenderUpsert(*RenderContext, UpsertClause) (string, error) {
	return "", fmt.Errorf("gooq: dialect %s renders upsert as MERGE", DialectDM)
}

// RenderExcluded 渲染 MERGE 源表里的本次插入列。
func (DMDialect) RenderExcluded(rc *RenderContext, column string) string {
	return "T2." + rc.Quote(column)
}

func (DMDialect) RenderInsert(rc *RenderContext, stmt InsertStatement) (string, error) {
	if stmt.Upsert == nil {
		return "", fmt.Errorf("gooq: dialect %s MERGE is missing an upsert clause", DialectDM)
	}
	if len(stmt.Upsert.ConflictCols) == 0 {
		return "", fmt.Errorf("gooq: dialect %s requires a conflict target for MERGE", DialectDM)
	}
	if len(stmt.Columns) == 0 || len(stmt.Rows) == 0 {
		return "", fmt.Errorf("gooq: dialect %s MERGE insert is empty", DialectDM)
	}
	quotedCols := make([]string, len(stmt.Columns))
	for i, column := range stmt.Columns {
		quotedCols[i] = rc.Quote(column)
	}
	selects := make([]string, len(stmt.Rows))
	for i, row := range stmt.Rows {
		parts := make([]string, len(stmt.Columns))
		for j := range stmt.Columns {
			if i == 0 {
				parts[j] = row[j] + " AS " + quotedCols[j]
				continue
			}
			parts[j] = row[j]
		}
		selects[i] = "SELECT " + strings.Join(parts, ", ") + " FROM DUAL"
	}
	ons := make([]string, len(stmt.Upsert.ConflictCols))
	for i, key := range stmt.Upsert.ConflictCols {
		column, ok := matchColumn(stmt.Columns, key)
		if !ok {
			return "", fmt.Errorf("gooq: MERGE conflict column %q is not in the insert list", key)
		}
		quoted := rc.Quote(column)
		ons[i] = "T1." + quoted + " = T2." + quoted
	}
	values := make([]string, len(quotedCols))
	for i, quoted := range quotedCols {
		values[i] = "T2." + quoted
	}
	sqlStr := fmt.Sprintf(
		"MERGE INTO %s T1 USING (%s) T2 ON (%s) WHEN NOT MATCHED THEN INSERT(%s) VALUES (%s)",
		stmt.Table,
		strings.Join(selects, " UNION ALL "),
		strings.Join(ons, " AND "),
		strings.Join(quotedCols, ", "),
		strings.Join(values, ", "),
	)
	if stmt.Upsert.DoNothing {
		return sqlStr, nil
	}
	if len(stmt.Upsert.Updates) == 0 {
		return "", fmt.Errorf("gooq: dialect %s MERGE update is empty", DialectDM)
	}
	sets, err := renderDMUpdateSets(rc, stmt.Upsert.Updates)
	if err != nil {
		return "", err
	}
	return sqlStr + " WHEN MATCHED THEN UPDATE SET " + sets, nil
}

func renderDMUpdateSets(rc *RenderContext, updates map[string]any) (string, error) {
	cols := sortedMapKeys(updates)
	sets := make([]string, 0, len(cols))
	for _, col := range cols {
		valueSQL, err := renderUpsertValue(rc, updates[col])
		if err != nil {
			return "", err
		}
		quoted := rc.Quote(col)
		sets = append(sets, "T1."+quoted+" = "+valueSQL)
	}
	return strings.Join(sets, ", "), nil
}

func matchColumn(columns []string, name string) (string, bool) {
	for _, column := range columns {
		if strings.EqualFold(column, name) {
			return column, true
		}
	}
	return "", false
}

func (DMDialect) RenderDelete(_ *RenderContext, target string, joins []JoinRender, where string) (string, error) {
	if len(joins) > 0 {
		return "", fmt.Errorf("gooq: DELETE with JOIN is not supported by dialect %s", DialectDM)
	}
	return renderDeleteFrom(target, where), nil
}

func (DMDialect) QuoteLiteral(s string) string { return quoteLiteralStandard(s) }

func (DMDialect) RenderIfNull(a, b string) string {
	return fmt.Sprintf("NVL(%s, %s)", a, b)
}

func (DMDialect) RenderCurDate() string { return "CURRENT_DATE" }

func (DMDialect) RenderDateAdd(field, count, unit string) (string, error) {
	canonical, err := canonicalDateUnit(unit)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("DATEADD(%s, %s, %s)", canonical, count, field), nil
}

// RenderDateDiff 渲染两个日期相差的天数。达梦的 DATEDIFF 是后一个日期减去前一个。
func (DMDialect) RenderDateDiff(a, b string) string {
	return fmt.Sprintf("DATEDIFF(DAY, %s, %s)", b, a)
}

func (DMDialect) RenderRound(field, decimals string) string {
	return fmt.Sprintf("ROUND(%s, %s)", field, decimals)
}

func (DMDialect) RenderUpdate(_ *RenderContext, target, sets, where string, joins []JoinRender) (string, error) {
	if joinsHaveOuter(joins) {
		return "", fmt.Errorf("gooq: UPDATE with an outer join is not supported by dialect %s", DialectDM)
	}
	return renderUpdateFrom(target, sets, where, joins), nil
}

func (DMDialect) JoinKeyword(keyword string) string { return keyword }

func (DMDialect) SupportsGroup(string) error { return nil }

func (DMDialect) RollupWith() bool { return false }

func (DMDialect) OmitLock() bool { return false }

func (DMDialect) RenderDateFormat(fieldSQL, format string) (string, error) {
	return fmt.Sprintf("TO_CHAR(%s, %s)", fieldSQL, quoteLiteralStandard(dmFormatReplacer.Replace(format))), nil
}

func (DMDialect) RenderGroupConcat(in GroupConcatInput) (string, error) {
	separator := in.Separator
	if separator == "" {
		separator = ","
	}
	distinct := ""
	if in.Distinct {
		distinct = "DISTINCT "
	}
	sqlStr := fmt.Sprintf("LISTAGG(%s%s, %s)", distinct, in.FieldSQL, quoteLiteralStandard(separator))
	if in.OrderSQL != "" {
		sqlStr += " WITHIN GROUP (ORDER BY " + in.OrderSQL + ")"
	}
	return sqlStr, nil
}

func (DMDialect) RenderMedian(fieldSQL string) string {
	return fmt.Sprintf("MEDIAN(%s)", fieldSQL)
}
