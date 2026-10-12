// Package mysqlwire 提供 MySQL 协议驱动共用的 DSN 和目录查询。
// MariaDB 和 TiDB 使用同一套连接格式，目录查询可以按库替换。
package mysqlwire

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	mysqldriver "github.com/go-sql-driver/mysql"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

// SQLDriver 是 database/sql 里的 MySQL 驱动名。
func SQLDriver() string { return "mysql" }

// Underlying 返回 go-sql-driver 的驱动实例。
func Underlying() *mysqldriver.MySQLDriver { return &mysqldriver.MySQLDriver{} }

// DSN 把 GoFrame 链接编成 go-sql-driver 的 DSN。
func DSN(link db.ParsedLink) (string, error) {
	dsn := fmt.Sprintf("%s:%s@%s(%s:%s)/%s?parseTime=true", link.User, link.Pass, link.Protocol, link.Host, link.Port, link.Name)
	if link.Port == "" {
		dsn = fmt.Sprintf("%s:%s@%s(%s)/%s?parseTime=true", link.User, link.Pass, link.Protocol, link.Host, link.Name)
	}
	if link.Extra == "" {
		return dsn, nil
	}
	extra, err := url.ParseQuery(link.Extra)
	if err != nil {
		return "", fmt.Errorf("gooq: mysql link extra: %w", err)
	}
	if extra.Get("parseTime") == "" {
		extra.Set("parseTime", "true")
	}
	return fmt.Sprintf("%s:%s@%s(%s:%s)/%s?%s", link.User, link.Pass, link.Protocol, link.Host, link.Port, link.Name, extra.Encode()), nil
}

// ApplySession 把字符集和时区参数加进 DSN。
func ApplySession(dsn string, cfg db.Config) string {
	if cfg.Charset != "" && !strings.Contains(dsn, "charset=") {
		dsn += "&charset=" + cfg.Charset
	}
	if cfg.Timezone != "" && !strings.Contains(dsn, "loc=") {
		dsn += "&loc=" + cfg.Timezone
	}
	return dsn
}

// Tables 返回当前库的用户表名。
func Tables(ctx context.Context, query db.Queryer) ([]string, error) {
	return db.QueryStrings(ctx, query, "SHOW TABLES")
}

// Columns 用 SHOW FULL COLUMNS 读取列。MySQL 和 TiDB 使用这份结果。
func Columns(ctx context.Context, query db.Queryer, table string) ([]db.Column, error) {
	records, err := db.QueryMaps(ctx, query, "SHOW FULL COLUMNS FROM `"+table+"`")
	if err != nil {
		return nil, err
	}
	columns := make([]db.Column, 0, len(records))
	for i, record := range records {
		dbType := record["type"]
		columns = append(columns, db.Column{
			Name:          record["field"],
			DBType:        dbType,
			LocalType:     db.LocalTypeOf(dbType),
			Primary:       strings.EqualFold(record["key"], "PRI"),
			Unique:        strings.EqualFold(record["key"], "UNI"),
			AutoIncrement: strings.Contains(strings.ToLower(record["extra"]), "auto_increment"),
			Nullable:      strings.EqualFold(record["null"], "YES"),
			Index:         i,
		})
	}
	return columns, nil
}

// SchemaSQL 返回 USE 语句。引号跟着传入的方言。
func SchemaSQL(dialect gooq.Dialect, name string) (string, error) {
	quoted, err := db.QuoteIdent(dialect, name)
	if err != nil {
		return "", err
	}
	return "USE " + quoted, nil
}
