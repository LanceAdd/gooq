package gooq

import (
	"fmt"
	"strings"
)

// groupConcatExpr 是字符串聚合表达式节点（跨方言映射，携带结构化选项）。
type groupConcatExpr struct {
	field     Expression
	separator string
	distinct  bool
	orderBy   []OrderClause
}

// GroupConcatOptions 是字符串聚合选项（GroupConcatFunc 使用）。
type GroupConcatOptions struct {
	// Field 是聚合字段（必填）。
	Field Expression
	// Separator 是分隔符（默认 ","）。
	Separator string
	// Distinct 是否去重（MySQL/SQLite 支持；PG 不支持，ToSql 渲染报错）。
	Distinct bool
	// OrderBy 是组内排序（SQLite 忽略；MySQL/PG 支持）。
	OrderBy []OrderClause
}

// GroupConcatFunc 构造字符串聚合表达式。
// 跨方言映射：MySQL/SQLite GROUP_CONCAT、PG STRING_AGG；
// 分隔符参数自动转 SQL 字符串字面量。
func GroupConcatFunc(options GroupConcatOptions) Expression {
	return &groupConcatExpr{
		field:     options.Field,
		separator: options.Separator,
		distinct:  options.Distinct,
		orderBy:   options.OrderBy,
	}
}

// Condition 实现 Expression 接口（默认 MySQL 方言渲染）。
func (e *groupConcatExpr) Condition() (string, []any) {
	return e.render(newRenderContext(DialectMySQL))
}

func (e *groupConcatExpr) render(rc *renderContext) (string, []any) {
	fieldSQL, args := rc.render(e.field)
	separator := e.separator
	if separator == "" {
		separator = ","
	}
	var orderSQL string
	if len(e.orderBy) > 0 {
		var parts []string
		for _, o := range e.orderBy {
			orderSQLPart, _ := o.render(rc)
			parts = append(parts, orderSQLPart)
		}
		orderSQL = strings.Join(parts, ", ")
	}
	switch rc.dialect {
	case DialectPgsql:
		// STRING_AGG(field, 'sep' [ORDER BY ...])；DISTINCT 不支持（validate 已拦截）。
		sqlStr := fmt.Sprintf("STRING_AGG(%s, '%s'", fieldSQL, separator)
		if orderSQL != "" {
			sqlStr += " ORDER BY " + orderSQL
		}
		return sqlStr + ")", args
	case DialectSQLite:
		// GROUP_CONCAT([DISTINCT ]field, 'sep')；SQLite 不支持函数内 ORDER BY。
		distinct := ""
		if e.distinct {
			distinct = "DISTINCT "
		}
		return fmt.Sprintf("GROUP_CONCAT(%s%s, '%s')", distinct, fieldSQL, separator), args
	default:
		// MySQL：GROUP_CONCAT([DISTINCT ]field [ORDER BY ...] [SEPARATOR 'sep'])。
		distinct := ""
		if e.distinct {
			distinct = "DISTINCT "
		}
		sqlStr := fmt.Sprintf("GROUP_CONCAT(%s%s", distinct, fieldSQL)
		if orderSQL != "" {
			sqlStr += " ORDER BY " + orderSQL
		}
		if e.separator != "" && e.separator != "," {
			sqlStr += " SEPARATOR '" + e.separator + "'"
		}
		return sqlStr + ")", args
	}
}
