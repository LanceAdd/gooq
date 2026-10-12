package gooq

// OceanBaseDialect 是 OceanBase MySQL 模式的 SQL 渲染实现。
// OceanBase 使用 MySQL 协议，当前语句形状与 MySQL 相同。单独注册是为了链接前缀和以后的差异有自己的位置。
type OceanBaseDialect struct {
	MySQLDialect
}

// Name 返回 oceanbase。
func (OceanBaseDialect) Name() Dialect { return DialectOceanBase }

// SupportsGroup 拒绝 CUBE 和 GROUPING SETS，错误里写 oceanbase。
func (OceanBaseDialect) SupportsGroup(ext string) error {
	return rejectMySQLGroups(DialectOceanBase, ext)
}
