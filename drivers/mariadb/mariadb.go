// Package mariadb 为 gooq/db 注册 MariaDB 连接驱动。
// 连接使用 MySQL 协议。JSON 列在 MariaDB 里常保存成 longtext 加 json_valid 检查，目录查询把这种列标成 json。
package mariadb

import (
	"context"
	"database/sql/driver"
	"strings"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
	"github.com/lanceadd/gooq/drivers/internal/mysqlwire"
)

type mariadbDriver struct{}

func init() {
	if err := db.Register(mariadbDriver{}); err != nil {
		panic(err)
	}
}

func (mariadbDriver) Names() []string { return []string{"mariadb"} }

func (mariadbDriver) Dialect() gooq.Dialect { return gooq.DialectMariaDB }

func (mariadbDriver) SQLDriver() string { return mysqlwire.SQLDriver() }

func (mariadbDriver) Underlying() driver.Driver { return mysqlwire.Underlying() }

func (mariadbDriver) DSN(link db.ParsedLink) (string, error) { return mysqlwire.DSN(link) }

func (mariadbDriver) ApplySession(dsn string, cfg db.Config) string {
	return mysqlwire.ApplySession(dsn, cfg)
}

func (mariadbDriver) Tables(ctx context.Context, query db.Queryer) ([]string, error) {
	return mysqlwire.Tables(ctx, query)
}

func (mariadbDriver) Columns(ctx context.Context, query db.Queryer, table string) ([]db.Column, error) {
	records, err := db.QueryMaps(ctx, query, `
SELECT
    c.COLUMN_NAME AS field,
    (CASE WHEN ch.CHECK_CLAUSE LIKE 'json_valid%' THEN 'json' ELSE c.COLUMN_TYPE END) AS type,
    c.IS_NULLABLE AS nullable,
    c.COLUMN_KEY AS column_key,
    c.EXTRA AS extra
FROM information_schema.COLUMNS AS c
LEFT JOIN information_schema.CHECK_CONSTRAINTS AS ch
  ON c.TABLE_NAME = ch.TABLE_NAME
 AND c.TABLE_SCHEMA = ch.CONSTRAINT_SCHEMA
 AND c.COLUMN_NAME = ch.CONSTRAINT_NAME
WHERE c.TABLE_SCHEMA = DATABASE()
  AND c.TABLE_NAME = ?
ORDER BY c.ORDINAL_POSITION`, table)
	if err != nil {
		return nil, err
	}
	columns := make([]db.Column, 0, len(records))
	indexByName := make(map[string]int, len(records))
	for _, record := range records {
		name := record["field"]
		dbType := record["type"]
		primary := strings.EqualFold(record["column_key"], "PRI")
		unique := strings.EqualFold(record["column_key"], "UNI")
		autoIncrement := strings.Contains(strings.ToLower(record["extra"]), "auto_increment")
		if index, ok := indexByName[name]; ok {
			if dbType == "json" {
				columns[index].DBType = dbType
				columns[index].LocalType = db.LocalTypeOf(dbType)
			}
			if primary {
				columns[index].Primary = true
				columns[index].Unique = false
			} else if unique && !columns[index].Primary {
				columns[index].Unique = true
			}
			if autoIncrement {
				columns[index].AutoIncrement = true
			}
			continue
		}
		indexByName[name] = len(columns)
		columns = append(columns, db.Column{
			Name:          name,
			DBType:        dbType,
			LocalType:     db.LocalTypeOf(dbType),
			Primary:       primary,
			Unique:        unique,
			AutoIncrement: autoIncrement,
			Nullable:      strings.EqualFold(record["nullable"], "YES"),
			Index:         len(columns),
		})
	}
	return columns, nil
}

func (mariadbDriver) SchemaSQL(name string) (string, error) {
	return mysqlwire.SchemaSQL(gooq.DialectMariaDB, name)
}
