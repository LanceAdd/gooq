package gooq

// MariaDBDialect 是 MariaDB 的 SQL 渲染实现。
// 语句形状嵌入 MySQL。MariaDB 10.5 虽然有 RETURNING，这里仍跟 GoFrame 一样不生成它。
type MariaDBDialect struct {
	MySQLDialect
}

func (MariaDBDialect) Name() Dialect { return DialectMariaDB }

func (MariaDBDialect) SupportsGroup(ext string) error {
	return rejectMySQLGroups(DialectMariaDB, ext)
}
