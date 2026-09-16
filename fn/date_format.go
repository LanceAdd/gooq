package fn

import (
	"fmt"
	"strings"

	"github.com/lanceadd/gooq"
)

// DateFormat 日期格式化（MySQL DATE_FORMAT / PG TO_CHAR / SQLite strftime）；
// format 使用 MySQL 格式符（%Y-%m-%d 等），PG 渲染时自动转换。
func DateFormat(field gooq.Expression, format string) *FuncExpr {
	return New("DATE_FORMAT", field).RenderWith(
		func(rc *gooq.RenderContext, argsSQL []string) (string, []any) {
			switch rc.Dialect() {
			case gooq.DialectPgsql:
				return fmt.Sprintf("TO_CHAR(%s, '%s')", argsSQL[0], mysqlToPgFormat(format)), nil
			case gooq.DialectSQLite:
				return fmt.Sprintf("strftime('%s', %s)", format, argsSQL[0]), nil
			default:
				return fmt.Sprintf("DATE_FORMAT(%s, '%s')", argsSQL[0], format), nil
			}
		},
	)
}

var pgFormatReplacer = strings.NewReplacer(
	"%Y", "YYYY", "%y", "YY",
	"%m", "MM", "%d", "DD",
	"%H", "HH24", "%i", "MI", "%s", "SS",
	"%e", "FMDD", "%j", "DDD",
)

func mysqlToPgFormat(format string) string {
	return pgFormatReplacer.Replace(format)
}
