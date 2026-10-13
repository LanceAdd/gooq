// Package oceanbase 为 gooq/db 注册 OceanBase 连接驱动。
// OceanBase MySQL 模式使用 MySQL 协议，语句形状与 MySQL 相同。单独注册是为了链接前缀和以后的差异有自己的位置。
package oceanbase

import (
	"context"
	"database/sql/driver"
	"fmt"
	"net/url"
	"strings"

	mysqldriver "github.com/go-sql-driver/mysql"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

type oceanbaseDriver struct{}

func init() {
	if err := db.Register(oceanbaseDriver{}); err != nil {
		panic(err)
	}
}

func (oceanbaseDriver) Names() []string { return []string{"oceanbase"} }

func (oceanbaseDriver) Dialect() gooq.Dialect { return gooq.DialectOceanBase }

func (oceanbaseDriver) SQLDriver() string { return "mysql" }

func (oceanbaseDriver) Underlying() driver.Driver { return &mysqldriver.MySQLDriver{} }

func (oceanbaseDriver) DSN(link db.ParsedLink) (string, error) {
	endpoint := oceanbaseEndpoint(link)
	dsn := fmt.Sprintf("%s:%s@%s/%s?parseTime=true", link.User, link.Pass, endpoint, link.Name)
	if link.Extra == "" {
		return dsn, nil
	}
	extra, err := url.ParseQuery(link.Extra)
	if err != nil {
		return "", fmt.Errorf("gooq: oceanbase link extra: %w", err)
	}
	if extra.Get("parseTime") == "" {
		extra.Set("parseTime", "true")
	}
	return fmt.Sprintf("%s:%s@%s/%s?%s", link.User, link.Pass, endpoint, link.Name, extra.Encode()), nil
}

func oceanbaseEndpoint(link db.ParsedLink) string {
	if link.Port == "" {
		return fmt.Sprintf("%s(%s)", link.Protocol, link.Host)
	}
	return fmt.Sprintf("%s(%s:%s)", link.Protocol, link.Host, link.Port)
}

func (oceanbaseDriver) ApplySession(dsn string, cfg db.Config) string {
	if cfg.Charset != "" && !strings.Contains(dsn, "charset=") {
		dsn += "&charset=" + cfg.Charset
	}
	if cfg.Timezone != "" && !strings.Contains(dsn, "loc=") {
		dsn += "&loc=" + cfg.Timezone
	}
	return dsn
}

func (oceanbaseDriver) Tables(ctx context.Context, query db.Queryer) ([]string, error) {
	return db.QueryStrings(ctx, query, "SHOW TABLES")
}

func (oceanbaseDriver) Columns(ctx context.Context, query db.Queryer, table string) ([]db.Column, error) {
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

func (oceanbaseDriver) SchemaSQL(name string) (string, error) {
	quoted, err := db.QuoteIdent(gooq.DialectOceanBase, name)
	if err != nil {
		return "", err
	}
	return "USE " + quoted, nil
}
