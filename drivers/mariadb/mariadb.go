// Package mariadb 为 gooq/db 注册 MariaDB 连接驱动。
// 连接使用 MySQL 协议。JSON 列在 MariaDB 里常保存成 longtext 加 json_valid 检查，目录查询把这种列标成 json。
package mariadb

import (
	"context"
	"database/sql/driver"
	"fmt"
	"net"
	"net/url"
	"strings"

	mysqldriver "github.com/go-sql-driver/mysql"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

type mariadbDriver struct{}

func init() {
	if err := db.Register(mariadbDriver{}); err != nil {
		panic(err)
	}
}

func (mariadbDriver) Names() []string { return []string{"mariadb"} }

func (mariadbDriver) Dialect() gooq.Dialect { return gooq.DialectMariaDB }

func (mariadbDriver) SQLDriver() string { return "mysql" }

func (mariadbDriver) Underlying() driver.Driver { return &mysqldriver.MySQLDriver{} }

func (mariadbDriver) DSN(link db.ParsedLink) (string, error) {
	endpoint, err := mysqlFamilyEndpoint(link)
	if err != nil {
		return "", err
	}
	dsn := fmt.Sprintf("%s:%s@%s/%s?parseTime=true", link.User, link.Pass, endpoint, link.Name)
	if link.Extra == "" {
		return dsn, nil
	}
	extra, err := url.ParseQuery(link.Extra)
	if err != nil {
		return "", fmt.Errorf("gooq: mariadb link extra: %w", err)
	}
	if extra.Get("parseTime") == "" {
		extra.Set("parseTime", "true")
	}
	return fmt.Sprintf("%s:%s@%s/%s?%s", link.User, link.Pass, endpoint, link.Name, extra.Encode()), nil
}

func mysqlFamilyEndpoint(link db.ParsedLink) (string, error) {
	switch link.Protocol {
	case "unix":
		if link.Port != "" {
			return "", fmt.Errorf("gooq: mariadb unix link must not include a port")
		}
		return "unix(" + link.Host + ")", nil
	case "tcp", "tcp4", "tcp6":
		host := link.Host
		if link.Port != "" {
			host = net.JoinHostPort(link.Host, link.Port)
		}
		return fmt.Sprintf("%s(%s)", link.Protocol, host), nil
	default:
		if link.Port == "" {
			return fmt.Sprintf("%s(%s)", link.Protocol, link.Host), nil
		}
		return fmt.Sprintf("%s(%s:%s)", link.Protocol, link.Host, link.Port), nil
	}
}

func (mariadbDriver) ApplySession(dsn string, cfg db.Config) string {
	base, raw, _ := strings.Cut(dsn, "?")
	values, err := url.ParseQuery(raw)
	if err != nil {
		return dsn
	}
	if cfg.Charset != "" && !values.Has("charset") {
		values.Set("charset", cfg.Charset)
	}
	if cfg.Timezone != "" && !values.Has("loc") {
		values.Set("loc", cfg.Timezone)
	}
	encoded := values.Encode()
	if encoded == "" {
		return base
	}
	return base + "?" + encoded
}

func (mariadbDriver) Tables(ctx context.Context, query db.Queryer) ([]string, error) {
	return db.QueryStrings(ctx, query, "SHOW TABLES")
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
	quoted, err := db.QuoteIdent(gooq.DialectMariaDB, name)
	if err != nil {
		return "", err
	}
	return "USE " + quoted, nil
}
