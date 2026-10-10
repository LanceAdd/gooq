package fn

import (
	"fmt"

	"github.com/lanceadd/gooq"
)

// DateFormat 日期格式化（MySQL DATE_FORMAT / PG TO_CHAR / SQLite strftime）；
// format 使用 MySQL 格式符（%Y-%m-%d 等），由当前方言的 DialectSpec 转换。
func DateFormat(field gooq.Expression, format string) *FuncExpr {
	return New("DATE_FORMAT", field).RenderWith(
		func(rc *gooq.RenderContext, argsSQL []string) (string, []any) {
			if spec := rc.Spec(); spec != nil {
				return spec.RenderDateFormat(argsSQL[0], format), nil
			}
			return fmt.Sprintf("DATE_FORMAT(%s, '%s')", argsSQL[0], format), nil
		},
	)
}
