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
	columns := make([]db.Column, 0, len(records))
	for i, record := range records {
		dbType := record["type"]
		primary := record["pk"] != "" && record["pk"] != "0"
		localType := db.LocalTypeOf(dbType)
		autoIncrement := primary && localType == gooq.LocalTypeInt
		columns = append(columns, db.Column{
			Name:          record["name"],
			DBType:        dbType,
			LocalType:     localType,
			Primary:       primary,
			AutoIncrement: autoIncrement,
			Nullable:      record["notnull"] == "0" || record["notnull"] == "",
			Index:         i,
		})
	}
	return columns, nil
}

func (sqliteDriver) SchemaSQL(string) (string, error) { return "", nil }
