// Package mysql 为 gooq/db 注册 MySQL 连接驱动。
package mysql

import (
	"context"
	"database/sql/driver"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
	"github.com/lanceadd/gooq/drivers/internal/mysqlwire"
)

type mysqlDriver struct{}

func init() {
	if err := db.Register(mysqlDriver{}); err != nil {
		panic(err)
	}
}

func (mysqlDriver) Names() []string { return []string{"mysql"} }

func (mysqlDriver) Dialect() gooq.Dialect { return gooq.DialectMySQL }

func (mysqlDriver) SQLDriver() string { return mysqlwire.SQLDriver() }

func (mysqlDriver) Underlying() driver.Driver { return mysqlwire.Underlying() }

func (mysqlDriver) DSN(link db.ParsedLink) (string, error) { return mysqlwire.DSN(link) }

func (mysqlDriver) ApplySession(dsn string, cfg db.Config) string {
	return mysqlwire.ApplySession(dsn, cfg)
}

func (mysqlDriver) Tables(ctx context.Context, query db.Queryer) ([]string, error) {
	return mysqlwire.Tables(ctx, query)
}

func (mysqlDriver) Columns(ctx context.Context, query db.Queryer, table string) ([]db.Column, error) {
	return mysqlwire.Columns(ctx, query, table)
}

func (mysqlDriver) SchemaSQL(name string) (string, error) {
	return mysqlwire.SchemaSQL(gooq.DialectMySQL, name)
}
