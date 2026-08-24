package gooq

import (
	"context"
	"strings"
)

type OperatorFn func(ctx context.Context, args ...any) (sql string, argsOut []any, err error)

// operatorRegistry 是操作符注册表：name → driver → impl。
var operatorRegistry = make(map[string]map[string]OperatorFn)

// OperatorFunc 注册操作符实现；drivers 为空时作为默认实现（所有驱动回退）。
func OperatorFunc(name string, f OperatorFn, drivers ...string) {
	if len(drivers) == 0 {
		if operatorRegistry[name] == nil {
			operatorRegistry[name] = make(map[string]OperatorFn)
		}
		operatorRegistry[name][""] = f
		return
	}
	for _, driver := range drivers {
		if operatorRegistry[name] == nil {
			operatorRegistry[name] = make(map[string]OperatorFn)
		}
		operatorRegistry[name][driver] = f
	}
}

func getOperator(name, driver string) OperatorFn {
	drivers, ok := operatorRegistry[name]
	if !ok {
		return nil
	}
	if f, ok := drivers[driver]; ok {
		return f
	}
	return drivers[""]
}

type overClause struct {
	partitionBy []Expression
	orderBy     []OrderClause
	frame       *WindowFrame
}

type WindowFrame struct {
	unit  string // ROWS 或 RANGE。
	start string // 框架起点，如 UNBOUNDED PRECEDING / CURRENT ROW / 2 PRECEDING。
	end   string // 框架终点，如 CURRENT ROW / UNBOUNDED FOLLOWING / 2 FOLLOWING。
}

func RowsFrame(start, end string) WindowFrame {
	return WindowFrame{unit: "ROWS", start: start, end: end}
}

func RangeFrame(start, end string) WindowFrame {
	return WindowFrame{unit: "RANGE", start: start, end: end}
}

func Func(name string, args ...any) Fn {
	return Fn{name: name, args: args}
}

type Fn struct {
	name  string
	args  []any
	alias string
	over  *overClause
}

func (f Fn) As(alias string) Fn {
	f.alias = alias
	return f
}

func (f Fn) Over(partitionBy []Expression, orderBy []OrderClause) Fn {
	f.over = &overClause{partitionBy: partitionBy, orderBy: orderBy}
	return f
}

func (f Fn) OverFrame(partitionBy []Expression, orderBy []OrderClause, frame WindowFrame) Fn {
	f.over = &overClause{partitionBy: partitionBy, orderBy: orderBy, frame: &frame}
	return f
}

func (f Fn) Condition() (string, []any) {
	return f.render(newRenderContext(context.Background(), DialectMySQL))
}

func (f Fn) render(rc *renderContext) (string, []any) {
	var (
		argsSQL []string
		argsAll []any
	)
	for _, arg := range f.args {
		switch v := arg.(type) {
		case Expression:
			sql, args := rc.render(v)
			argsSQL = append(argsSQL, sql)
			argsAll = append(argsAll, args...)
		default:
			argsSQL = append(argsSQL, rc.addArg(v))
			argsAll = append(argsAll, v)
		}
	}
	var (
		sql    string
		impl   = getOperator(f.name, rc.driver())
		opArgs = make([]any, len(argsSQL))
	)
	for i := range argsSQL {
		opArgs[i] = argsSQL[i]
	}
	if impl != nil {
		implSQL, implArgs, err := impl(rc.ctx, opArgs...)
		if err == nil {
			sql = implSQL
			argsAll = append(argsAll, implArgs...)
		} else {
			sql = genericOperatorRender(f.name, argsSQL)
		}
	} else {
		sql = genericOperatorRender(f.name, argsSQL)
	}
	if f.over != nil {
		var overParts []string
		if len(f.over.partitionBy) > 0 {
			var parts []string
			for _, p := range f.over.partitionBy {
				sqlPart, _ := rc.render(p)
				parts = append(parts, sqlPart)
			}
			overParts = append(overParts, "PARTITION BY "+strings.Join(parts, ", "))
		}
		if len(f.over.orderBy) > 0 {
			var parts []string
			for _, o := range f.over.orderBy {
				sqlPart, _ := o.render(rc)
				parts = append(parts, sqlPart)
			}
			overParts = append(overParts, "ORDER BY "+strings.Join(parts, ", "))
		}
		if f.over.frame != nil {
			overParts = append(overParts,
				f.over.frame.unit+" BETWEEN "+f.over.frame.start+" AND "+f.over.frame.end)
		}
		sql += " OVER (" + strings.Join(overParts, " ") + ")"
	}
	if f.alias != "" {
		sql += " AS " + f.alias
	}
	return sql, argsAll
}

func genericOperatorRender(name string, argsSQL []string) string {
	return name + "(" + strings.Join(argsSQL, ", ") + ")"
}

func DateFormatFunc(args ...any) Fn {
	converted := make([]any, len(args))
	for i, arg := range args {
		if s, ok := arg.(string); ok {
			converted[i] = Str(s)
		} else {
			converted[i] = arg
		}
	}
	return Fn{name: "DATE_FORMAT", args: converted}
}

func CountFunc(field Expression) Fn {
	return Fn{name: "COUNT", args: []any{field}}
}

func SumFunc(field Expression) Fn {
	return Fn{name: "SUM", args: []any{field}}
}

func AvgFunc(field Expression) Fn {
	return Fn{name: "AVG", args: []any{field}}
}

func MinFunc(field Expression) Fn {
	return Fn{name: "MIN", args: []any{field}}
}

func MaxFunc(field Expression) Fn {
	return Fn{name: "MAX", args: []any{field}}
}

func CoalesceFunc(args ...any) Fn {
	return Fn{name: "COALESCE", args: args}
}

func IfNullFunc(a, b Expression) Fn {
	return Fn{name: "IFNULL", args: []any{a, b}}
}

func RankFunc() Fn {
	return Fn{name: "RANK"}
}

func NowFunc() Fn {
	return Fn{name: "NOW"}
}

func ConcatFunc(args ...any) Fn {
	return Fn{name: "CONCAT", args: args}
}

func SubstringFunc(s any, start, length any) Fn {
	return Fn{name: "SUBSTRING", args: []any{s, start, length}}
}

func UpperFunc(e Expression) Fn {
	return Fn{name: "UPPER", args: []any{e}}
}

func LowerFunc(e Expression) Fn {
	return Fn{name: "LOWER", args: []any{e}}
}

func TrimFunc(e Expression) Fn {
	return Fn{name: "TRIM", args: []any{e}}
}

func ReplaceFunc(s, old, new any) Fn {
	return Fn{name: "REPLACE", args: []any{s, old, new}}
}

func LengthFunc(e Expression) Fn {
	return Fn{name: "LENGTH", args: []any{e}}
}

func AbsFunc(e Expression) Fn {
	return Fn{name: "ABS", args: []any{e}}
}

func RoundFunc(e Expression, decimals any) Fn {
	return Fn{name: "ROUND", args: []any{e, decimals}}
}

func CeilFunc(e Expression) Fn {
	return Fn{name: "CEIL", args: []any{e}}
}

func FloorFunc(e Expression) Fn {
	return Fn{name: "FLOOR", args: []any{e}}
}

func ModFunc(a, b any) Fn {
	return Fn{name: "MOD", args: []any{a, b}}
}

func CurDateFunc() Fn {
	return Fn{name: "CURDATE"}
}

func DateAddFunc(date Expression, interval string) Fn {
	return Fn{name: "DATE_ADD", args: []any{date, interval}}
}

func DateDiffFunc(a, b Expression) Fn {
	return Fn{name: "DATEDIFF", args: []any{a, b}}
}

func CountDistinctFunc(field Expression) Fn {
	return Fn{name: "COUNT", args: []any{Distinct(field)}}
}

func RowNumberFunc() Fn {
	return Fn{name: "ROW_NUMBER"}
}

func DenseRankFunc() Fn {
	return Fn{name: "DENSE_RANK"}
}

func NtileFunc(n int) Fn {
	return Fn{name: "NTILE", args: []any{n}}
}

func LagFunc(field Expression, offset, defaultValue any) Fn {
	return Fn{name: "LAG", args: []any{field, offset, defaultValue}}
}

func LeadFunc(field Expression, offset, defaultValue any) Fn {
	return Fn{name: "LEAD", args: []any{field, offset, defaultValue}}
}
