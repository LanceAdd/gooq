package fn

import (
	"fmt"
	"strings"

	"github.com/lanceadd/gooq"
)

// GroupConcatOptions 是字符串聚合选项。
type GroupConcatOptions struct {
	// Field 是聚合字段（必填）。
	Field gooq.Expression
	// Separator 是分隔符（默认 ","）。
	Separator string
	// Distinct 是否去重（MySQL/SQLite 支持；PG 不支持，渲染校验报错）。
	Distinct bool
	// OrderBy 是组内排序（SQLite 忽略；MySQL/PG 支持），表达式自带方向（Field.Asc() 等）。
	OrderBy []gooq.Expression
}

// GroupConcat 构造字符串聚合表达式。
// 跨方言映射：MySQL/SQLite GROUP_CONCAT、PG STRING_AGG。
func GroupConcat(options GroupConcatOptions) *FuncExpr {
	f := New("GROUP_CONCAT", options.Field)
	f.validate = func(d gooq.Dialect) error {
		if options.Distinct && d == gooq.DialectPgsql {
			return fmt.Errorf("gooq: GROUP_CONCAT DISTINCT is not supported by dialect %s", d)
		}
		return nil
	}
	return f.RenderWith(func(rc *gooq.RenderContext, argsSQL []string) (string, []any) {
		fieldSQL := argsSQL[0]
		separator := options.Separator
		if separator == "" {
			separator = ","
		}
		var orderSQL string
		if len(options.OrderBy) > 0 {
			var parts []string
			for _, o := range options.OrderBy {
				orderSQLPart, _ := rc.Render(o)
				parts = append(parts, orderSQLPart)
			}
			orderSQL = strings.Join(parts, ", ")
		}
		switch rc.Dialect() {
		case gooq.DialectPgsql:
			// STRING_AGG(field, 'sep' [ORDER BY ...])；DISTINCT 不支持（validate 已拦截）。
			sqlStr := fmt.Sprintf("STRING_AGG(%s, '%s'", fieldSQL, separator)
			if orderSQL != "" {
				sqlStr += " ORDER BY " + orderSQL
			}
			return sqlStr + ")", nil
		case gooq.DialectSQLite:
			// GROUP_CONCAT([DISTINCT ]field, 'sep')；SQLite 不支持函数内 ORDER BY。
			distinct := ""
			if options.Distinct {
				distinct = "DISTINCT "
			}
			return fmt.Sprintf("GROUP_CONCAT(%s%s, '%s')", distinct, fieldSQL, separator), nil
		default:
			// MySQL：GROUP_CONCAT([DISTINCT ]field [ORDER BY ...] [SEPARATOR 'sep'])。
			distinct := ""
			if options.Distinct {
				distinct = "DISTINCT "
			}
			sqlStr := fmt.Sprintf("GROUP_CONCAT(%s%s", distinct, fieldSQL)
			if orderSQL != "" {
				sqlStr += " ORDER BY " + orderSQL
			}
			if options.Separator != "" && options.Separator != "," {
				sqlStr += " SEPARATOR '" + options.Separator + "'"
			}
			return sqlStr + ")", nil
		}
	})
}
