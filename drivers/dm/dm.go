// Package dm 为 gooq/db 注册达梦连接驱动。
// database/sql 的驱动名是 dm。链接前缀也是 dm。
package dm

import (
	"context"
	"database/sql/driver"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	dmdb "gitee.com/chunanyong/dm"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

const (
	defaultPort                  = "5236"
	dmTimezoneOffsetLimitMinutes = 12 * 60
)

type dmDriver struct{}

func init() {
	if err := db.Register(dmDriver{}); err != nil {
		panic(err)
	}
}

func (dmDriver) Names() []string { return []string{"dm"} }

func (dmDriver) Dialect() gooq.Dialect { return gooq.DialectDM }

func (dmDriver) SQLDriver() string { return "dm" }

func (dmDriver) Underlying() driver.Driver { return &dmdb.DmDriver{} }

func (dmDriver) DSN(link db.ParsedLink) (string, error) {
	if link.Name == "" {
		return "", fmt.Errorf("gooq: dm link is missing a database name")
	}
	if link.Host == "" {
		return "", fmt.Errorf("gooq: dm link is missing a host")
	}
	port := link.Port
	if port == "" {
		port = defaultPort
	}
	query := "schema=" + url.QueryEscape(link.Name)
	if link.Extra != "" {
		query += "&" + link.Extra
	}
	u := url.URL{
		Scheme:   "dm",
		User:     url.UserPassword(link.User, link.Pass),
		Host:     link.Host + ":" + port,
		Path:     "/" + link.Name,
		RawQuery: query,
	}
	return u.String(), nil
}

func (dmDriver) ApplySession(dsn string, cfg db.Config) string {
	configured, err := dmDriver{}.ConfigureSession(dsn, cfg)
	if err != nil {
		return dsn
	}
	return configured
}

// ConfigureSession 把字符集和时区写进 DSN。
// 达梦只接受分钟偏移，范围是 ±12 小时。夏令时按调用这一刻的偏移计算。
func (dmDriver) ConfigureSession(dsn string, cfg db.Config) (string, error) {
	if cfg.Charset != "" && !strings.Contains(dsn, "charset=") {
		dsn = appendQuery(dsn, "charset="+cfg.Charset)
	}
	if cfg.Timezone != "" && !strings.Contains(strings.ToLower(dsn), "timezone=") {
		minutes, err := dmTimezoneOffset(cfg.Timezone, time.Now())
		if err != nil {
			return "", err
		}
		dsn = appendQuery(dsn, "timeZone="+strconv.Itoa(minutes))
	}
	return dsn, nil
}

func appendQuery(dsn, pair string) string {
	if strings.Contains(dsn, "?") {
		return dsn + "&" + pair
	}
	return dsn + "?" + pair
}

func dmTimezoneOffset(timezone string, now time.Time) (int, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return 0, fmt.Errorf("gooq: dm timezone %q: %w", timezone, err)
	}
	_, offset := now.In(location).Zone()
	minutes := offset / 60
	if minutes < -dmTimezoneOffsetLimitMinutes || minutes > dmTimezoneOffsetLimitMinutes {
		return 0, fmt.Errorf(
			"gooq: dm timezone %q has an offset of %d minutes; DM supports offsets from -%d to %d minutes",
			timezone, minutes, dmTimezoneOffsetLimitMinutes, dmTimezoneOffsetLimitMinutes,
		)
	}
	return minutes, nil
}

func (dmDriver) Bind(v any) any {
	switch typed := v.(type) {
	case time.Time:
		if typed.IsZero() {
			return nil
		}
		return typed.Format("2006-01-02 15:04:05")
	case *time.Time:
		if typed == nil || typed.IsZero() {
			return nil
		}
		return typed.Format("2006-01-02 15:04:05")
	default:
		return v
	}
}

func (dmDriver) Tables(ctx context.Context, query db.Queryer) ([]string, error) {
	owner, err := currentSchema(ctx, query)
	if err != nil {
		return nil, err
	}
	records, err := db.QueryMaps(ctx, query, `
SELECT TABLE_NAME, IOT_NAME
FROM ALL_TABLES
WHERE OWNER = ?
ORDER BY TABLE_NAME`, owner)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(records))
	for _, record := range records {
		name := record["table_name"]
		if name == "" {
			name = record["iot_name"]
		}
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

func (dmDriver) Columns(ctx context.Context, query db.Queryer, table string) ([]db.Column, error) {
	owner, err := currentSchema(ctx, query)
	if err != nil {
		return nil, err
	}
	var records []map[string]string
	var used string
	for _, candidate := range tableNameCandidates(table) {
		records, err = db.QueryMaps(ctx, query, `
SELECT COLUMN_NAME, DATA_TYPE, DATA_LENGTH, NULLABLE
FROM ALL_TAB_COLUMNS
WHERE OWNER = ? AND TABLE_NAME = ?
ORDER BY COLUMN_ID`, owner, candidate)
		if err != nil {
			return nil, err
		}
		if len(records) > 0 {
			used = candidate
			break
		}
	}
	if used == "" {
		return []db.Column{}, nil
	}
	keys, err := db.QueryMaps(ctx, query, `
SELECT COLS.COLUMN_NAME AS COLUMN_NAME, CONS.CONSTRAINT_TYPE AS CONSTRAINT_TYPE
FROM ALL_CONSTRAINTS CONS
JOIN ALL_CONS_COLUMNS COLS
  ON CONS.OWNER = COLS.OWNER
 AND CONS.CONSTRAINT_NAME = COLS.CONSTRAINT_NAME
 AND CONS.TABLE_NAME = COLS.TABLE_NAME
WHERE CONS.OWNER = ? AND CONS.TABLE_NAME = ? AND CONS.CONSTRAINT_TYPE IN ('P', 'U')`, owner, used)
	if err != nil {
		return nil, err
	}
	primary := map[string]bool{}
	unique := map[string]bool{}
	for _, key := range keys {
		name := key["column_name"]
		switch strings.ToUpper(key["constraint_type"]) {
		case "P":
			primary[strings.ToUpper(name)] = true
		case "U":
			unique[strings.ToUpper(name)] = true
		}
	}
	columns := make([]db.Column, 0, len(records))
	for i, record := range records {
		dbType := columnType(record["data_type"], record["data_length"])
		name := record["column_name"]
		upper := strings.ToUpper(name)
		columns = append(columns, db.Column{
			Name:      name,
			DBType:    dbType,
			LocalType: localTypeOf(dbType),
			Primary:   primary[upper],
			Unique:    unique[upper] && !primary[upper],
			Nullable:  !strings.EqualFold(record["nullable"], "N"),
			Index:     i,
		})
	}
	return columns, nil
}

func currentSchema(ctx context.Context, query db.Queryer) (string, error) {
	names, err := db.QueryStrings(ctx, query, `SELECT NAME FROM SYSOBJECTS WHERE ID = CURRENT_SCHID`)
	if err != nil {
		return "", err
	}
	if len(names) == 0 || names[0] == "" {
		return "", fmt.Errorf("gooq: dm current schema is empty")
	}
	return names[0], nil
}

func tableNameCandidates(table string) []string {
	table = strings.Trim(table, `"`)
	upper := strings.ToUpper(table)
	if upper == table {
		return []string{table}
	}
	return []string{upper, table}
}

func columnType(dataType, dataLength string) string {
	length, err := strconv.Atoi(strings.TrimSpace(dataLength))
	if err != nil || length <= 0 || strings.Contains(dataType, "(") {
		return dataType
	}
	return fmt.Sprintf("%s(%d)", dataType, length)
}

func (dmDriver) SchemaSQL(name string) (string, error) {
	quoted, err := db.QuoteIdent(gooq.DialectDM, name)
	if err != nil {
		return "", err
	}
	return "SET SCHEMA " + quoted, nil
}
