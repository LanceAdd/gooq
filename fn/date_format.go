package fn

import (
	"github.com/lanceadd/gooq"
)

// DateFormat 日期格式化（MySQL DATE_FORMAT / PG TO_CHAR / SQLite strftime）；
// format 使用 MySQL 格式符（%Y-%m-%d 等），由具体方言类型转换。
func DateFormat(field gooq.Expression, format string) *FuncExpr {
	return New("DATE_FORMAT", field).RenderWith(
		func(rc *gooq.RenderContext, argsSQL []string) string {
			renderer, ok := dialectSupport[dateFormatRenderer](rc, "DateFormat")
			if !ok {
				return ""
			}
			return renderer.RenderDateFormat(argsSQL[0], format)
		},
	)
}
