package fn

import (
	"fmt"

	"github.com/lanceadd/gooq"
)

// 聚合函数。
func Count(field gooq.Expression) *FuncExpr {
	return New("COUNT", field)
}

func CountDistinct(field gooq.Expression) *FuncExpr {
	return New("COUNT", gooq.Distinct(field))
}

func Sum(field gooq.Expression) *FuncExpr {
	return New("SUM", field)
}

func Avg(field gooq.Expression) *FuncExpr {
	return New("AVG", field)
}

func Min(field gooq.Expression) *FuncExpr {
	return New("MIN", field)
}

func Max(field gooq.Expression) *FuncExpr {
	return New("MAX", field)
}

func AnyValue(field gooq.Expression) *FuncExpr {
	return New("ANY_VALUE", field)
}

func ArrayAgg(field gooq.Expression) *FuncExpr {
	return New("ARRAY_AGG", field)
}

func BitAnd(field gooq.Expression) *FuncExpr {
	return New("BIT_AND", field)
}

func BitOr(field gooq.Expression) *FuncExpr {
	return New("BIT_OR", field)
}

func BitXor(field gooq.Expression) *FuncExpr {
	return New("BIT_XOR", field)
}

func BoolAnd(field gooq.Expression) *FuncExpr {
	return New("BOOL_AND", field)
}

func BoolOr(field gooq.Expression) *FuncExpr {
	return New("BOOL_OR", field)
}

func Every(field gooq.Expression) *FuncExpr {
	return New("EVERY", field)
}

func JsonArrayAgg(field gooq.Expression) *FuncExpr {
	return New("JSON_ARRAYAGG", field)
}

func JsonObjectAgg(key, value any) *FuncExpr {
	return New("JSON_OBJECTAGG", key, value)
}

func Grouping(field gooq.Expression) *FuncExpr {
	return New("GROUPING", field)
}

func XmlAgg(field gooq.Expression) *FuncExpr {
	return New("XMLAGG", field)
}

// 字符串与数学函数。
func Concat(args ...any) *FuncExpr {
	return New("CONCAT", args...)
}

func Substring(s any, start, length any) *FuncExpr {
	return New("SUBSTRING", s, start, length)
}

func Upper(e gooq.Expression) *FuncExpr {
	return New("UPPER", e)
}

func Lower(e gooq.Expression) *FuncExpr {
	return New("LOWER", e)
}

func Trim(e gooq.Expression) *FuncExpr {
	return New("TRIM", e)
}

func Replace(s, old, new any) *FuncExpr {
	return New("REPLACE", s, old, new)
}

func Length(e gooq.Expression) *FuncExpr {
	return New("LENGTH", e)
}

func Abs(e gooq.Expression) *FuncExpr {
	return New("ABS", e)
}

func Round(e gooq.Expression, decimals any) *FuncExpr {
	return New("ROUND", e, decimals).RenderWith(func(rc *gooq.RenderContext, argsSQL []string) (string, []any) {
		if spec := rc.Spec(); spec != nil {
			return spec.RenderRound(argsSQL[0], argsSQL[1]), nil
		}
		return fmt.Sprintf("ROUND(%s, %s)", argsSQL[0], argsSQL[1]), nil
	})
}

func Ceil(e gooq.Expression) *FuncExpr {
	return New("CEIL", e)
}

func Floor(e gooq.Expression) *FuncExpr {
	return New("FLOOR", e)
}

func Mod(a, b any) *FuncExpr {
	return New("MOD", a, b)
}

// 值函数。
func Coalesce(args ...any) *FuncExpr {
	return New("COALESCE", args...)
}

func IfNull(a, b gooq.Expression) *FuncExpr {
	return New("IFNULL", a, b).RenderWith(func(rc *gooq.RenderContext, argsSQL []string) (string, []any) {
		if spec := rc.Spec(); spec != nil {
			return spec.RenderIfNull(argsSQL[0], argsSQL[1]), nil
		}
		return fmt.Sprintf("IFNULL(%s, %s)", argsSQL[0], argsSQL[1]), nil
	})
}

// 日期函数。
func Now() *FuncExpr {
	return New("NOW")
}

func CurDate() *FuncExpr {
	return New("CURDATE").RenderWith(func(rc *gooq.RenderContext, _ []string) (string, []any) {
		if spec := rc.Spec(); spec != nil {
			return spec.RenderCurDate(), nil
		}
		return "CURDATE()", nil
	})
}

func DateAdd(date gooq.Expression, count any, unit string) *FuncExpr {
	f := New("DATE_ADD", date, count)
	f.validate = func(d gooq.Dialect) error {
		spec, err := gooq.DialectSpecOf(d)
		if err != nil {
			return err
		}
		_, err = spec.RenderDateAdd("d", "1", unit)
		return err
	}
	return f.RenderWith(func(rc *gooq.RenderContext, argsSQL []string) (string, []any) {
		spec := rc.Spec()
		if spec == nil {
			return fmt.Sprintf("DATE_ADD(%s, INTERVAL %s %s)", argsSQL[0], argsSQL[1], unit), nil
		}
		sqlStr, err := spec.RenderDateAdd(argsSQL[0], argsSQL[1], unit)
		if err != nil {
			return "", nil
		}
		return sqlStr, nil
	})
}

func DateDiff(a, b gooq.Expression) *FuncExpr {
	return New("DATEDIFF", a, b).RenderWith(func(rc *gooq.RenderContext, argsSQL []string) (string, []any) {
		if spec := rc.Spec(); spec != nil {
			return spec.RenderDateDiff(argsSQL[0], argsSQL[1]), nil
		}
		return fmt.Sprintf("DATEDIFF(%s, %s)", argsSQL[0], argsSQL[1]), nil
	})
}

// 窗口函数。
func Rank() *FuncExpr {
	return New("RANK")
}

func RowNumber() *FuncExpr {
	return New("ROW_NUMBER")
}

func DenseRank() *FuncExpr {
	return New("DENSE_RANK")
}

func Ntile(n int) *FuncExpr {
	return New("NTILE", n)
}

func Lag(field gooq.Expression, offset, defaultValue any) *FuncExpr {
	return New("LAG", field, offset, defaultValue)
}

func Lead(field gooq.Expression, offset, defaultValue any) *FuncExpr {
	return New("LEAD", field, offset, defaultValue)
}

func CumeDist() *FuncExpr {
	return New("CUME_DIST")
}

func PercentRank() *FuncExpr {
	return New("PERCENT_RANK")
}
