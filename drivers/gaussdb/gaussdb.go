// Package gaussdb 为 gooq/db 注册 GaussDB（openGauss）连接驱动。
// 协议驱动名是 opengauss。目录查询避开 PostgreSQL 10 之后才有的系统列。
package gaussdb

import (
	"context"
	"database/sql/driver"
	"fmt"
	"net/url"
	"sort"
	"strings"

	opengauss "gitee.com/opengauss/openGauss-connector-go-pq"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

type gaussdbDriver struct{}

func init() {
	if err := db.Register(gaussdbDriver{}); err != nil {
		panic(err)
	}
}

func (gaussdbDriver) Names() []string { return []string{"gaussdb"} }

func (gaussdbDriver) Dialect() gooq.Dialect { return gooq.DialectGaussDB }

func (gaussdbDriver) SQLDriver() string { return "opengauss" }

func (gaussdbDriver) Underlying() driver.Driver { return opengauss.Driver{} }

func (gaussdbDriver) DSN(link db.ParsedLink) (string, error) {
	port := link.Port
	if port == "" {
		port = "5432"
	}
	values := map[string]string{
		"host":     link.Host,
		"port":     port,
		"user":     link.User,
		"password": link.Pass,
		"dbname":   link.Name,
		"sslmode":  "disable",
	}
	var extras []string
	if link.Extra != "" {
		extra, err := url.ParseQuery(link.Extra)
		if err != nil {
			return "", fmt.Errorf("gooq: gaussdb link extra: %w", err)
		}
		keys := make([]string, 0, len(extra))
		for key := range extra {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if _, exists := values[key]; exists {
				values[key] = extra.Get(key)
				continue
			}
			extras = append(extras, key+"="+quoteDSNValue(extra.Get(key)))
		}
	}
	order := []string{"host", "port", "user", "password", "dbname", "sslmode"}
	parts := make([]string, 0, len(order)+len(extras))
	for _, key := range order {
		parts = append(parts, key+"="+quoteDSNValue(values[key]))
	}
	parts = append(parts, extras...)
	return strings.Join(parts, " "), nil
}

// quoteDSNValue 按 openGauss Go 驱动的规则给关键字的值加引号。
// 规则与 lib/pq 相同：空串，或包含空格、单引号、反斜杠的值，用单引号包起来。
func quoteDSNValue(value string) string {
	if value == "" || strings.ContainsAny(value, " '\\") {
		return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(value) + "'"
	}
	return value
}

func (gaussdbDriver) ApplySession(dsn string, cfg db.Config) string {
	if cfg.Charset != "" && !strings.Contains(dsn, "client_encoding=") {
		dsn += " client_encoding=" + quoteDSNValue(cfg.Charset)
	}
	if cfg.Timezone != "" && !strings.Contains(dsn, "TimeZone=") {
		dsn += " TimeZone=" + cfg.Timezone
	}
	return dsn
}

func (gaussdbDriver) Tables(ctx context.Context, query db.Queryer) ([]string, error) {
	return db.QueryStrings(ctx, query, `
SELECT c.relname
FROM pg_class c
INNER JOIN pg_namespace n ON c.relnamespace = n.oid
WHERE n.nspname = current_schema()
  AND c.relkind IN ('r', 'p')
ORDER BY c.relname`)
}

func (gaussdbDriver) Columns(ctx context.Context, query db.Queryer, table string) ([]db.Column, error) {
	records, err := db.QueryMaps(ctx, query, `
SELECT
    a.attname AS field,
    t.typname AS type,
    a.attnotnull AS notnull,
    (CASE WHEN d.contype = 'p' THEN 'pri' WHEN d.contype = 'u' THEN 'uni' ELSE '' END) AS key,
    ic.column_default AS default_value
FROM pg_attribute a
    LEFT JOIN pg_class c ON a.attrelid = c.oid
    LEFT JOIN pg_constraint d ON d.conrelid = c.oid AND a.attnum = d.conkey[1]
    LEFT JOIN pg_type t ON a.atttypid = t.oid
    LEFT JOIN information_schema.columns ic
      ON ic.column_name = a.attname
     AND ic.table_name = c.relname
     AND ic.table_schema = current_schema()
WHERE c.oid = $1::regclass
    AND a.attisdropped IS FALSE
    AND a.attnum > 0
ORDER BY a.attnum`, table)
	if err != nil {
		return nil, err
	}
	columns := make([]db.Column, 0, len(records))
	indexByName := make(map[string]int, len(records))
	for _, record := range records {
		name := record["field"]
		primary := record["key"] == "pri"
		unique := record["key"] == "uni"
		if index, ok := indexByName[name]; ok {
			if primary {
				columns[index].Primary = true
				columns[index].Unique = false
			} else if unique && !columns[index].Primary {
				columns[index].Unique = true
			}
			if strings.Contains(record["default_value"], "nextval(") {
				columns[index].AutoIncrement = true
			}
			continue
		}
		dbType := record["type"]
		indexByName[name] = len(columns)
		columns = append(columns, db.Column{
			Name:          name,
			DBType:        dbType,
			LocalType:     db.LocalTypeOf(dbType),
			Primary:       primary,
			Unique:        unique && !primary,
			AutoIncrement: strings.Contains(record["default_value"], "nextval("),
			Nullable:      !boolText(record["notnull"]),
			Index:         len(columns),
		})
	}
	return columns, nil
}

func boolText(value string) bool {
	switch strings.ToLower(value) {
	case "1", "t", "true", "yes":
		return true
	default:
		return false
	}
}

func (gaussdbDriver) SchemaSQL(name string) (string, error) {
	quoted, err := db.QuoteIdent(gooq.DialectGaussDB, name)
	if err != nil {
		return "", err
	}
	return "SET search_path TO " + quoted, nil
}
