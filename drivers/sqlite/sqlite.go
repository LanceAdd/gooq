// Package sqlite 为 gooq/db 注册 SQLite 连接驱动。
package sqlite

import (
	"context"
	"database/sql/driver"
	"fmt"
	"path/filepath"
	"strings"

	sqlite3 "modernc.org/sqlite"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

type sqliteDriver struct{}

func init() {
	if err := db.Register(sqliteDriver{}); err != nil {
		panic(err)
	}
}

func (sqliteDriver) Names() []string { return []string{"sqlite"} }

func (sqliteDriver) Dialect() gooq.Dialect { return gooq.DialectSQLite }

func (sqliteDriver) SQLDriver() string { return "sqlite" }

func (sqliteDriver) Underlying() driver.Driver { return &sqlite3.Driver{} }

func (sqliteDriver) DSN(link db.ParsedLink) (string, error) {
	path := link.Name
	if path == "" {
		return "", fmt.Errorf("gooq: sqlite link is missing a file path")
	}
	if path == ":memory:" || strings.HasPrefix(path, "file:") {
		return path, nil
	}
	return "file:" + filepath.ToSlash(path), nil
}

func (sqliteDriver) ApplySession(dsn string, _ db.Config) string { return dsn }

func (sqliteDriver) Tables(ctx context.Context, query db.Queryer) ([]string, error) {
	return db.QueryStrings(ctx, query, `
SELECT name FROM sqlite_master
WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
ORDER BY name`)
}

func (sqliteDriver) Columns(ctx context.Context, query db.Queryer, table string) ([]db.Column, error) {
	records, err := db.QueryMaps(ctx, query, fmt.Sprintf("PRAGMA table_info(%q)", table))
	if err != nil {
		return nil, err
	}
	autoColumn, err := sqliteAutoIncrementColumn(ctx, query, table, records)
	if err != nil {
		return nil, err
	}
	columns := make([]db.Column, 0, len(records))
	for i, record := range records {
		dbType := record["type"]
		primary := record["pk"] != "" && record["pk"] != "0"
		columns = append(columns, db.Column{
			Name:          record["name"],
			DBType:        dbType,
			LocalType:     db.LocalTypeOf(dbType),
			Primary:       primary,
			AutoIncrement: autoColumn != "" && record["name"] == autoColumn,
			Nullable:      record["notnull"] == "0" || record["notnull"] == "",
			Index:         i,
		})
	}
	return columns, nil
}

// sqliteAutoIncrementColumn 返回 rowid 别名列。
// 只有一列主键、声明类型去掉空白后与 INTEGER 大小写不敏感地相等，并且建表语句不是 WITHOUT ROWID 时才返回列名。
// sqlite_master.sql 为 NULL 时，QueryMaps 得到空字符串，不当成建表语句。
func sqliteAutoIncrementColumn(ctx context.Context, query db.Queryer, table string, records []map[string]string) (string, error) {
	rows, err := db.QueryMaps(ctx, query, `
SELECT sql FROM sqlite_master
WHERE type = 'table' AND name = ?`, table)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 || strings.TrimSpace(rows[0]["sql"]) == "" {
		return "", nil
	}
	if strings.Contains(strings.ToUpper(rows[0]["sql"]), "WITHOUT ROWID") {
		return "", nil
	}
	name, dbType, ok := singlePrimaryColumn(records)
	if !ok || !strings.EqualFold(strings.TrimSpace(dbType), "INTEGER") {
		return "", nil
	}
	return name, nil
}

func singlePrimaryColumn(records []map[string]string) (name, dbType string, ok bool) {
	for _, record := range records {
		if record["pk"] == "" || record["pk"] == "0" {
			continue
		}
		if ok {
			return "", "", false
		}
		name = record["name"]
		dbType = record["type"]
		ok = true
	}
	return name, dbType, ok
}

func (sqliteDriver) SchemaSQL(string) (string, error) { return "", nil }
