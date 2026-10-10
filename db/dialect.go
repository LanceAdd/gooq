package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/lanceadd/gooq"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

type dialectKind struct {
	dialect   gooq.Dialect
	sqlDriver string
	dsn       func(parsedLink) (string, error)
	tables    func(ctx context.Context, query queryer) ([]string, error)
	columns   func(ctx context.Context, query queryer, table string) ([]Column, error)
}

var dialects = map[string]dialectKind{
	"mysql": {
		dialect:   gooq.DialectMySQL,
		sqlDriver: "mysql",
		dsn:       mysqlDSN,
		tables:    mysqlTables,
		columns:   mysqlColumns,
	},
	"pgsql": {
		dialect:   gooq.DialectPgsql,
		sqlDriver: "postgres",
		dsn:       postgresDSN,
		tables:    postgresTables,
		columns:   postgresColumns,
	},
	"postgres": {
		dialect:   gooq.DialectPgsql,
		sqlDriver: "postgres",
		dsn:       postgresDSN,
		tables:    postgresTables,
		columns:   postgresColumns,
	},
	"sqlite": {
		dialect:   gooq.DialectSQLite,
		sqlDriver: "sqlite",
		dsn:       sqliteDSN,
		tables:    sqliteTables,
		columns:   sqliteColumns,
	},
}

func mysqlTables(ctx context.Context, query queryer) ([]string, error) {
	return queryStrings(ctx, query, "SHOW TABLES")
}

func mysqlColumns(ctx context.Context, query queryer, table string) ([]Column, error) {
	records, err := queryMaps(ctx, query, "SHOW FULL COLUMNS FROM `"+table+"`")
	if err != nil {
		return nil, err
	}
	columns := make([]Column, 0, len(records))
	for i, record := range records {
		dbType := record["type"]
		columns = append(columns, Column{
			Name:          record["field"],
			DBType:        dbType,
			LocalType:     LocalTypeOf(dbType),
			Primary:       strings.EqualFold(record["key"], "PRI"),
			Unique:        strings.EqualFold(record["key"], "UNI"),
			AutoIncrement: strings.Contains(strings.ToLower(record["extra"]), "auto_increment"),
			Index:         i,
		})
	}
	return columns, nil
}

func postgresTables(ctx context.Context, query queryer) ([]string, error) {
	return queryStrings(ctx, query, `
SELECT tablename
FROM pg_catalog.pg_tables
WHERE schemaname = current_schema()
ORDER BY tablename`)
}

func postgresColumns(ctx context.Context, query queryer, table string) ([]Column, error) {
	records, err := queryMaps(ctx, query, `
SELECT c.column_name, c.udt_name, c.column_default, c.ordinal_position::text AS ordinal_position
FROM information_schema.columns c
WHERE c.table_schema = current_schema() AND c.table_name = $1
ORDER BY c.ordinal_position`, table)
	if err != nil {
		return nil, err
	}
	primary, err := queryStrings(ctx, query, `
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
	unique, err := queryStrings(ctx, query, `
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
	columns := make([]Column, 0, len(records))
	for i, record := range records {
		dbType := record["udt_name"]
		columns = append(columns, Column{
			Name:          record["column_name"],
			DBType:        dbType,
			LocalType:     LocalTypeOf(dbType),
			Primary:       primarySet[record["column_name"]],
			Unique:        uniqueSet[record["column_name"]],
			AutoIncrement: strings.Contains(record["column_default"], "nextval("),
			Index:         i,
		})
	}
	return columns, nil
}

func sqliteTables(ctx context.Context, query queryer) ([]string, error) {
	return queryStrings(ctx, query, `
SELECT name FROM sqlite_master
WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
ORDER BY name`)
}

func sqliteColumns(ctx context.Context, query queryer, table string) ([]Column, error) {
	records, err := queryMaps(ctx, query, fmt.Sprintf("PRAGMA table_info(%q)", table))
	if err != nil {
		return nil, err
	}
	columns := make([]Column, 0, len(records))
	for i, record := range records {
		dbType := record["type"]
		primary := record["pk"] != "" && record["pk"] != "0"
		localType := LocalTypeOf(dbType)
		autoIncrement := primary && localType == gooq.LocalTypeInt
		columns = append(columns, Column{
			Name:          record["name"],
			DBType:        dbType,
			LocalType:     localType,
			Primary:       primary,
			AutoIncrement: autoIncrement,
			Index:         i,
		})
	}
	return columns, nil
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}
