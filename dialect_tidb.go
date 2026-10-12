package gooq

// TiDBDialect 是 TiDB 的 SQL 渲染实现。
// TiDB 使用 MySQL 协议，当前语句形状与 MySQL 相同。
type TiDBDialect struct {
	MySQLDialect
}

// Name 返回 tidb。
func (TiDBDialect) Name() Dialect { return DialectTiDB }

// SupportsGroup 拒绝 CUBE 和 GROUPING SETS，错误里写 tidb。
func (TiDBDialect) SupportsGroup(ext string) error {
	return rejectMySQLGroups(DialectTiDB, ext)
}
