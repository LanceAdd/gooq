// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

package gooq

// SubExpressioner 由携带子表达式的复合节点实现；遍历经此递归进入自定义节点类型。
type SubExpressioner interface {
	SubExpressions() []Expression
}

// DialectValidator 由需要做方言能力校验的节点实现（子查询构建器、方言受限函数等）。
type DialectValidator interface {
	ValidateDialect(d Dialect) error
}

// WalkExpression 深度优先遍历表达式树：先触发节点自身的方言校验，再递归子表达式。
func WalkExpression(e Expression, dialect Dialect) error {
	if e == nil {
		return nil
	}
	if v, ok := e.(DialectValidator); ok {
		if err := v.ValidateDialect(dialect); err != nil {
			return err
		}
	}
	if s, ok := e.(SubExpressioner); ok {
		for _, sub := range s.SubExpressions() {
			if err := WalkExpression(sub, dialect); err != nil {
				return err
			}
		}
	}
	return nil
}

// ReferencesColumn 判断条件树中是否显式引用了指定列（软删显式接管检测）。
func ReferencesColumn(conditions []Expression, columnName string) bool {
	for _, c := range conditions {
		if referencesColumn(c, columnName) {
			return true
		}
	}
	return false
}

func referencesColumn(e Expression, columnName string) bool {
	switch v := e.(type) {
	case *fieldCondition:
		return v.columnName == columnName
	case *groupCondition:
		for _, c := range v.conditions {
			if referencesColumn(c, columnName) {
				return true
			}
		}
	}
	return false
}
