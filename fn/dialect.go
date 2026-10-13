package fn

import (
	"fmt"

	"github.com/lanceadd/gooq"
)

// 函数方言不在 DialectSpec 上。具体方言类型继续实现这些方法，fn 经类型断言调用。

type dateFormatRenderer interface {
	RenderDateFormat(fieldSQL, format string) (string, error)
}

type groupConcatRenderer interface {
	RenderGroupConcat(in gooq.GroupConcatInput) (string, error)
}

type medianRenderer interface {
	RenderMedian(fieldSQL string) string
}

type ifNullRenderer interface {
	RenderIfNull(a, b string) string
}

type curDateRenderer interface {
	RenderCurDate() string
}

type dateAddRenderer interface {
	RenderDateAdd(field, count, unit string) (string, error)
}

type dateDiffRenderer interface {
	RenderDateDiff(a, b string) string
}

type roundRenderer interface {
	RenderRound(field, decimals string) string
}

func unsupportedDialect(dialect gooq.Dialect, name string) error {
	return fmt.Errorf("gooq: dialect %q does not support %s", dialect, name)
}

func dialectSupport[T any](rc *gooq.RenderContext, name string) (T, bool) {
	var zero T
	spec := rc.Spec()
	if spec == nil {
		rc.Fail(unsupportedDialect(rc.Dialect(), name))
		return zero, false
	}
	renderer, ok := spec.(T)
	if !ok {
		rc.Fail(unsupportedDialect(rc.Dialect(), name))
		return zero, false
	}
	return renderer, true
}
