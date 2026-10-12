// Package tidb 为 gooq/db 注册 TiDB 连接驱动。
// TiDB 使用 MySQL 协议，语句形状与 MySQL 相同。单独注册是为了链接前缀和以后的 TiDB 差异有自己的位置。
package tidb

import (
	"context"
	"database/sql/driver"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
	"github.com/lanceadd/gooq/drivers/internal/mysqlwire"
)

type tidbDriver struct{}

func init() {
	if err := db.Register(tidbDriver{}); err != nil {
		panic(err)
	}
}

func (tidbDriver) Names() []string { return []string{"tidb"} }

func (tidbDriver) Dialect() gooq.Dialect { return gooq.DialectTiDB }

func (tidbDriver) SQLDriver() string { return mysqlwire.SQLDriver() }

func (tidbDriver) Underlying() driver.Driver { return mysqlwire.Underlying() }

func (tidbDriver) DSN(link db.ParsedLink) (string, error) { return mysqlwire.DSN(link) }

func (tidbDriver) ApplySession(dsn string, cfg db.Config) string {
	return mysqlwire.ApplySession(dsn, cfg)
}

func (tidbDriver) Tables(ctx context.Context, query db.Queryer) ([]string, error) {
	return mysqlwire.Tables(ctx, query)
}

func (tidbDriver) Columns(ctx context.Context, query db.Queryer, table string) ([]db.Column, error) {
	return mysqlwire.Columns(ctx, query, table)
}

func (tidbDriver) SchemaSQL(name string) (string, error) {
	return mysqlwire.SchemaSQL(gooq.DialectTiDB, name)
}
