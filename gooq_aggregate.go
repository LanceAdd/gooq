package gooq

import (
	"context"
	"fmt"
	"strings"
)

// groupConcatExpr 是字符串聚合表达式节点（跨方言映射，携带结构化选项）。
type groupConcatExpr struct {
	field     Expression
	separator string
	distinct  bool
	orderBy   []OrderClause
}

// GroupConcatOptions 是字符串聚合选项（GroupConcatFunc 使用）。
type GroupConcatOptions struct {
	// Field 是聚合字段（必填）。
	Field Expression
	// Separator 是分隔符（默认 ","）。
	Separator string
	// Distinct 是否去重（MySQL/SQLite 支持；PG/Oracle/MSSQL 不支持，ToSql 渲染报错）。
	Distinct bool
	// OrderBy 是组内排序（SQLite 忽略；MySQL/PG/Oracle/MSSQL 支持）。
	OrderBy []OrderClause
}

// GroupConcatFunc 构造字符串聚合表达式。
// 跨方言映射：MySQL/SQLite GROUP_CONCAT、PG/MSSQL STRING_AGG、Oracle LISTAGG；
// 分隔符参数自动转 SQL 字符串字面量。
func GroupConcatFunc(opts GroupConcatOptions) Expression {
	return &groupConcatExpr{
		field:     opts.Field,
		separator: opts.Separator,
		distinct:  opts.Distinct,
		orderBy:   opts.OrderBy,
	}
}

// Condition 实现 Expression 接口（默认 MySQL 方言渲染）。
func (e *groupConcatExpr) Condition() (string, []any) {
	return e.render(newRenderContext(context.Background(), DialectMySQL))
}

func (e *groupConcatExpr) render(rc *renderContext) (string, []any) {
	fieldSQL, args := rc.render(e.field)
	sep := e.separator
	if sep == "" {
		sep = ","
	}
	var orderSQL string
	if len(e.orderBy) > 0 {
		var parts []string
		for _, o := range e.orderBy {
			orderSQLPart, _ := o.render(rc)
			parts = append(parts, orderSQLPart)
		}
		orderSQL = strings.Join(parts, ", ")
	}
	switch rc.dialect {
	case DialectPgsql:
		// STRING_AGG(field, 'sep' [ORDER BY ...])；DISTINCT 不支持（validate 已拦截）。
		sqlStr := fmt.Sprintf("STRING_AGG(%s, '%s'", fieldSQL, sep)
		if orderSQL != "" {
			sqlStr += " ORDER BY " + orderSQL
		}
		return sqlStr + ")", args
	case DialectSQLite:
		// GROUP_CONCAT([DISTINCT ]field, 'sep')；SQLite 不支持函数内 ORDER BY。
		distinct := ""
		if e.distinct {
			distinct = "DISTINCT "
		}
		return fmt.Sprintf("GROUP_CONCAT(%s%s, '%s')", distinct, fieldSQL, sep), args
	case DialectOracle:
		// LISTAGG(field, 'sep') [WITHIN GROUP (ORDER BY ...)]；DISTINCT 不支持（validate 已拦截）。
		sqlStr := fmt.Sprintf("LISTAGG(%s, '%s')", fieldSQL, sep)
		if orderSQL != "" {
			sqlStr += " WITHIN GROUP (ORDER BY " + orderSQL + ")"
		}
		return sqlStr, args
	case DialectMssql:
		// STRING_AGG(field, 'sep') [WITHIN GROUP (ORDER BY ...)]；DISTINCT 不支持（validate 已拦截）。
		sqlStr := fmt.Sprintf("STRING_AGG(%s, '%s')", fieldSQL, sep)
		if orderSQL != "" {
			sqlStr += " WITHIN GROUP (ORDER BY " + orderSQL + ")"
		}
		return sqlStr, args
	default:
		// MySQL：GROUP_CONCAT([DISTINCT ]field [ORDER BY ...] [SEPARATOR 'sep'])。
		distinct := ""
		if e.distinct {
			distinct = "DISTINCT "
		}
		sqlStr := fmt.Sprintf("GROUP_CONCAT(%s%s", distinct, fieldSQL)
		if orderSQL != "" {
			sqlStr += " ORDER BY " + orderSQL
		}
		if e.separator != "" && e.separator != "," {
			sqlStr += " SEPARATOR '" + e.separator + "'"
		}
		return sqlStr + ")", args
	}
}

// walkExpression 深度优先遍历表达式树；visit 返回非 nil 错误即中止传播。
// 子查询节点按同一方言递归校验（聚合函数兼容性检查需深入嵌套表达式）。
func walkExpression(e Expression, dialect Dialect, visit func(e Expression) error) error {
	switch v := e.(type) {
	case *groupCondition:
		for _, c := range v.conds {
			if c == nil {
				continue
			}
			if err := walkExpression(c, dialect, visit); err != nil {
				return err
			}
		}
	case *fieldCondition:
		if err := visit(v); err != nil {
			return err
		}
		if sub, ok := v.val.(Expression); ok {
			if err := walkExpression(sub, dialect, visit); err != nil {
				return err
			}
		}
		for _, val := range v.vals {
			if sub, ok := val.(Expression); ok {
				if err := walkExpression(sub, dialect, visit); err != nil {
					return err
				}
			}
		}
	case *exprCondition:
		if err := visit(v); err != nil {
			return err
		}
		if err := walkExpression(v.left, dialect, visit); err != nil {
			return err
		}
		if sub, ok := v.val.(Expression); ok {
			if err := walkExpression(sub, dialect, visit); err != nil {
				return err
			}
		}
	case *arithExpr:
		if err := visit(v); err != nil {
			return err
		}
		if left, ok := v.left.(Expression); ok {
			if err := walkExpression(left, dialect, visit); err != nil {
				return err
			}
		}
		if right, ok := v.right.(Expression); ok {
			if err := walkExpression(right, dialect, visit); err != nil {
				return err
			}
		}
	case *negateExpr:
		if err := visit(v); err != nil {
			return err
		}
		if err := walkExpression(v.expr, dialect, visit); err != nil {
			return err
		}
	case *caseExpr:
		for _, w := range v.builder.whens {
			if err := walkExpression(w.when, dialect, visit); err != nil {
				return err
			}
			if thenExpr, ok := w.then.(Expression); ok {
				if err := walkExpression(thenExpr, dialect, visit); err != nil {
					return err
				}
			}
		}
		if elseVal, ok := v.builder.elseVal.(Expression); ok {
			if err := walkExpression(elseVal, dialect, visit); err != nil {
				return err
			}
		}
	case *distinctExpr:
		if err := visit(v); err != nil {
			return err
		}
		if err := walkExpression(v.expr, dialect, visit); err != nil {
			return err
		}
	case Fn:
		if err := visit(v); err != nil {
			return err
		}
		for _, arg := range v.args {
			if argExpr, ok := arg.(Expression); ok {
				if err := walkExpression(argExpr, dialect, visit); err != nil {
					return err
				}
			}
		}
	case *groupConcatExpr:
		if err := visit(v); err != nil {
			return err
		}
		if err := walkExpression(v.field, dialect, visit); err != nil {
			return err
		}
	case *existsCondition:
		if err := visit(v); err != nil {
			return err
		}
		return walkExpression(v.subquery, dialect, visit)
	case *SelectBuilder:
		if err := visit(v); err != nil {
			return err
		}
		return v.validate(dialect)
	case *rawExpr:
		return visit(v)
	default:
		return visit(e)
	}
	return nil
}
