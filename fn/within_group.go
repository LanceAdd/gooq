package fn

import (
	"fmt"
	"strings"

	"github.com/lanceadd/gooq"
)

func PercentileCont(fraction any, orderBy ...gooq.Expression) *FuncExpr {
	return New("PERCENTILE_CONT", fraction).RenderWith(
		func(rc *gooq.RenderContext, argsSQL []string) string {
			return fmt.Sprintf(
				"PERCENTILE_CONT(%s) WITHIN GROUP (ORDER BY %s)",
				argsSQL[0], renderExprList(rc, orderBy),
			)
		},
	)
}

func PercentileDisc(fraction any, orderBy ...gooq.Expression) *FuncExpr {
	return New("PERCENTILE_DISC", fraction).RenderWith(
		func(rc *gooq.RenderContext, argsSQL []string) string {
			return fmt.Sprintf(
				"PERCENTILE_DISC(%s) WITHIN GROUP (ORDER BY %s)",
				argsSQL[0], renderExprList(rc, orderBy),
			)
		},
	)
}

func Mode(orderBy ...gooq.Expression) *FuncExpr {
	return New("MODE").RenderWith(
		func(rc *gooq.RenderContext, _ []string) string {
			return "MODE() WITHIN GROUP (ORDER BY " + renderExprList(rc, orderBy) + ")"
		},
	)
}

func Median(field gooq.Expression) *FuncExpr {
	return New("MEDIAN", field).RenderWith(
		func(rc *gooq.RenderContext, argsSQL []string) string {
			renderer, ok := dialectSupport[medianRenderer](rc, "Median")
			if !ok {
				return ""
			}
			return renderer.RenderMedian(argsSQL[0])
		},
	)
}

func renderExprList(rc *gooq.RenderContext, exprs []gooq.Expression) string {
	parts := make([]string, 0, len(exprs))
	for _, e := range exprs {
		parts = append(parts, rc.Render(e))
	}
	return strings.Join(parts, ", ")
}
