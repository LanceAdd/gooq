package fn

import (
	"strings"

	"github.com/lanceadd/gooq"
)

// GroupConcatOptions 是字符串聚合选项。
type GroupConcatOptions struct {
	// Field 是聚合字段（必填）。
	Field gooq.Expression
	// Separator 是分隔符（默认 ","）。
	Separator string
	// Distinct 是否去重。MySQL 支持。SQLite 只支持单参数 DISTINCT，带分隔符时渲染报错。
	// PostgreSQL 16 起支持 STRING_AGG(DISTINCT expr, delim)。
	Distinct bool
	// OrderBy 是组内排序（SQLite 忽略；MySQL/PG 支持），表达式自带方向（Field.Asc() 等）。
	OrderBy []gooq.Expression
}

// GroupConcat 构造字符串聚合表达式。
// 跨方言映射：MySQL/SQLite GROUP_CONCAT、PG STRING_AGG。
func GroupConcat(options GroupConcatOptions) *FuncExpr {
	f := New("GROUP_CONCAT", options.Field)
	f.validate = func(d gooq.Dialect) error {
		spec, err := gooq.DialectSpecOf(d)
		if err != nil {
			return err
		}
		_, err = spec.RenderGroupConcat(gooq.GroupConcatInput{
			Distinct:  options.Distinct,
			Separator: options.Separator,
		})
		return err
	}
	return f.RenderWith(func(rc *gooq.RenderContext, argsSQL []string) (string, []any) {
		spec := rc.Spec()
		if spec == nil {
			return "", nil
		}
		var orderSQL string
		if len(options.OrderBy) > 0 {
			parts := make([]string, 0, len(options.OrderBy))
			for _, o := range options.OrderBy {
				orderSQLPart, _ := rc.Render(o)
				parts = append(parts, orderSQLPart)
			}
			orderSQL = strings.Join(parts, ", ")
		}
		sqlStr, err := spec.RenderGroupConcat(gooq.GroupConcatInput{
			FieldSQL:  argsSQL[0],
			Separator: options.Separator,
			Distinct:  options.Distinct,
			OrderSQL:  orderSQL,
		})
		if err != nil {
			// ValidateDialect 会先返回这个错误。这里不再拼出错误方言的函数。
			return "", nil
		}
		return sqlStr, nil
	})
}
