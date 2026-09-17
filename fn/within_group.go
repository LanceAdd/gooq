package fn

import (
	"fmt"
	"strings"

	"github.com/lanceadd/gooq"
)

func PercentileCont(fraction any, orderBy ...gooq.Expression) *FuncExpr {
	return New("PERCENTILE_CONT", fraction).RenderWith(
		func(rc *gooq.RenderContext, argsSQL []string) (string, []any) {
			return fmt.Sprintf(
				"PERCENTILE_CONT(%s) WITHIN GROUP (ORDER BY %s)",
				argsSQL[0], renderExprList(rc, orderBy),
			), nil
		},
	)
}

func PercentileDisc(fraction any, orderBy ...gooq.Expression) *FuncExpr {
	return New("PERCENTILE_DISC", fraction).RenderWith(
		func(rc *gooq.RenderContext, argsSQL []string) (string, []any) {
			return fmt.Sprintf(
				"PERCENTILE_DISC(%s) WITHIN GROUP (ORDER BY %s)",
				argsSQL[0], renderExprList(rc, orderBy),
			), nil
		},
	)
}

func Mode(orderBy ...gooq.Expression) *FuncExpr {
	return New("MODE").RenderWith(
		func(rc *gooq.RenderContext, _ []string) (string, []any) {
			return "MODE() WITHIN GROUP (ORDER BY " + renderExprList(rc, orderBy) + ")", nil
		},
	)
}

func Median(field gooq.Expression) *FuncExpr {
	return New("MEDIAN", field).RenderWith(
		func(rc *gooq.RenderContext, argsSQL []string) (string, []any) {
			if rc.Dialect() == gooq.DialectPgsql {
				return fmt.Sprintf("PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY %s)", argsSQL[0]), nil
			}
			return fmt.Sprintf("MEDIAN(%s)", argsSQL[0]), nil
		},
	)
}

func renderExprList(rc *gooq.RenderContext, exprs []gooq.Expression) string {
	parts := make([]string, 0, len(exprs))
	for _, e := range exprs {
		sqlPart, _ := rc.Render(e)
		parts = append(parts, sqlPart)
	}
	return strings.Join(parts, ", ")
}
