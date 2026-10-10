// Package pgsql registers the PostgreSQL connection driver for gooq/db.
package pgsql

import (
	"context"
	"database/sql/driver"
	"fmt"
	"net/url"
	"strings"

	"github.com/lib/pq"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

type postgresDriver struct{}

func init() {
	if err := db.Register(postgresDriver{}); err != nil {
		panic(err)
	}
}

func (postgresDriver) Names() []string { return []string{"pgsql", "postgres"} }

func (postgresDriver) Dialect() gooq.Dialect { return gooq.DialectPgsql }

func (postgresDriver) SQLDriver() string { return "postgres" }

func (postgresDriver) Underlying() driver.Driver { return &pq.Driver{} }

func (postgresDriver) DSN(link db.ParsedLink) (string, error) {
	port := link.Port
	if port == "" {
		port = "5432"
	}
	parts := []string{
		"host=" + link.Host,
		"port=" + port,
		"user=" + link.User,
		"password=" + link.Pass,
		"dbname=" + link.Name,
		"sslmode=disable",
	}
	if link.Extra != "" {
		extra, err := url.ParseQuery(link.Extra)
		if err != nil {
			return "", fmt.Errorf("gooq: pgsql link extra: %w", err)
		}
		if mode := extra.Get("sslmode"); mode != "" {
			parts[5] = "sslmode=" + mode
		}
	}
	return strings.Join(parts, " "), nil
}

func (postgresDriver) ApplySession(dsn string, cfg db.Config) string {
	if cfg.Timezone != "" && !strings.Contains(dsn, "TimeZone=") {
		dsn += " TimeZone=" + cfg.Timezone
	}
	return dsn
}

func (postgresDriver) Tables(ctx context.Context, query db.Queryer) ([]string, error) {
	return db.QueryStrings(ctx, query, `
SELECT tablename
FROM pg_catalog.pg_tables
WHERE schemaname = current_schema()
ORDER BY tablename`)
}

func (postgresDriver) Columns(ctx context.Context, query db.Queryer, table string) ([]db.Column, error) {
	records, err := db.QueryMaps(ctx, query, `
SELECT c.column_name, c.udt_name, c.column_default, c.ordinal_position::text AS ordinal_position
FROM information_schema.columns c
WHERE c.table_schema = current_schema() AND c.table_name = $1
ORDER BY c.ordinal_position`, table)
	if err != nil {
		return nil, err
	}
	primary, err := db.QueryStrings(ctx, query, `
SELECT kcu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON tc.constraint_name = kcu.constraint_name
 AND tc.table_schema = kcu.table_schema
 AND tc.table_name = kcu.table_name
WHERE tc.table_schema = current_schema()
  AND tc.table_name = $1
  AND tc.constraint_type = 'PRIMARY KEY'`, table)
	if err != nil {
		return nil, err
	}
	unique, err := db.QueryStrings(ctx, query, `
SELECT kcu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON tc.constraint_name = kcu.constraint_name
 AND tc.table_schema = kcu.table_schema
 AND tc.table_name = kcu.table_name
WHERE tc.table_schema = current_schema()
  AND tc.table_name = $1
  AND tc.constraint_type = 'UNIQUE'`, table)
	if err != nil {
		return nil, err
	}
	primarySet := stringSet(primary)
	uniqueSet := stringSet(unique)
	columns := make([]db.Column, 0, len(records))
	for i, record := range records {
		dbType := record["udt_name"]
		columns = append(columns, db.Column{
			Name:          record["column_name"],
			DBType:        dbType,
			LocalType:     db.LocalTypeOf(dbType),
			Primary:       primarySet[record["column_name"]],
			Unique:        uniqueSet[record["column_name"]],
			AutoIncrement: strings.Contains(record["column_default"], "nextval("),
			Index:         i,
		})
	}
	return columns, nil
}

func (postgresDriver) SchemaSQL(name string) (string, error) {
	quoted, err := db.QuoteIdent(gooq.DialectPgsql, name)
	if err != nil {
		return "", err
	}
	return "SET search_path TO " + quoted, nil
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}
