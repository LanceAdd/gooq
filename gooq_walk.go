package gooq

// SubExpressioner 由携带子表达式的复合节点实现；遍历经此递归进入自定义节点类型。
type SubExpressioner interface {
	SubExpressions() []Expression
}

// DialectValidator 由需要做方言能力校验的节点实现（子查询构建器、方言受限函数等）。
type DialectValidator interface {
	ValidateDialect(d Dialect) error
}

// columnRef 是表达式树里的一列。Field[T] 实现它，但不是 SubExpressioner。
type columnRef interface {
	ColumnName() string
	BoundTable() *TableBase
	NamePrefix() string
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

// ReferencesColumn 判断条件树是否显式引用了这张表上的列。
// 有表指针时按实例比较。没有表指针时，用渲染前缀里的别名或表名。
func ReferencesColumn(conditions []Expression, table *TableBase, columnName string) bool {
	if table == nil || columnName == "" {
		return false
	}
	for _, c := range conditions {
		if referencesColumn(c, table, columnName) {
			return true
		}
	}
	return false
}

func referencesColumn(e Expression, table *TableBase, columnName string) bool {
	if e == nil {
		return false
	}
	if col, ok := e.(columnRef); ok && columnMatches(col, table, columnName) {
		return true
	}
	if s, ok := e.(SubExpressioner); ok {
		for _, sub := range s.SubExpressions() {
			if referencesColumn(sub, table, columnName) {
				return true
			}
		}
	}
	return false
}

func columnMatches(col columnRef, table *TableBase, columnName string) bool {
	if col.ColumnName() != columnName {
		return false
	}
	if bound := col.BoundTable(); bound != nil {
		return bound == table
	}
	prefix := col.NamePrefix()
	if prefix == "" {
		return false
	}
	if alias := table.AliasName(); alias != "" {
		return prefix == alias
	}
	return prefix == table.TableName()
}
