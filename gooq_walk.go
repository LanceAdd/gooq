// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

package gooq

// walkExpression 深度优先遍历表达式树；visit 返回非 nil 错误即中止传播。
// 子查询节点按同一方言递归校验（聚合函数兼容性检查需深入嵌套表达式）。
func walkExpression(e Expression, dialect Dialect, visit func(e Expression) error) error {
	switch v := e.(type) {
	case *groupCondition:
		for _, c := range v.conditions {
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
		if subquery, ok := v.value.(Expression); ok {
			if err := walkExpression(subquery, dialect, visit); err != nil {
				return err
			}
		}
		for _, value := range v.values {
			if subquery, ok := value.(Expression); ok {
				if err := walkExpression(subquery, dialect, visit); err != nil {
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
		if subquery, ok := v.value.(Expression); ok {
			if err := walkExpression(subquery, dialect, visit); err != nil {
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
	case *castExpr:
		if err := visit(v); err != nil {
			return err
		}
		return walkExpression(v.expr, dialect, visit)
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
