package db

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/lanceadd/gooq"
)

// Column 是 gooq-gen 使用的一列。
type Column struct {
	Name          string
	DBType        string
	LocalType     gooq.LocalType
	Primary       bool
	AutoIncrement bool
	Unique        bool
	Nullable      bool
	Index         int
}

var identPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validIdent(name string) error {
	if !identPattern.MatchString(name) {
		return fmt.Errorf("gooq: invalid identifier %q", name)
	}
	return nil
}

// QuoteIdent 用已注册的渲染方言给一个普通标识符加引号。
// 标识符不合法或方言未注册时返回错误。
func QuoteIdent(dialect gooq.Dialect, name string) (string, error) {
	if err := validIdent(name); err != nil {
		return "", err
	}
	spec := gooq.LookupDialect(dialect)
	if spec == nil {
		return "", fmt.Errorf("gooq: dialect %q is not registered", dialect)
	}
	return spec.Quote(name), nil
}

// LocalTypeOf 把数据库类型名映射成 gooq 的本地类型。
// 先去掉末尾的 unsigned 和 zerofill，再去掉括号里的宽度。
// 数组类型使用 PostgreSQL 的 udt 名，例如 _int4、_text。
// 没有列出的类型映射成 string，包括未列出的数组名和范围类型。
func LocalTypeOf(dbType string) gooq.LocalType {
	typed := strings.ToLower(strings.TrimSpace(dbType))
	changed := true
	for changed {
		changed = false
		for _, suffix := range []string{" unsigned", " zerofill"} {
			if strings.HasSuffix(typed, suffix) {
				typed = strings.TrimSpace(strings.TrimSuffix(typed, suffix))
				changed = true
			}
		}
	}
	if index := strings.IndexByte(typed, '('); index > 0 {
		typed = strings.TrimSpace(typed[:index])
	}
	switch typed {
	case "bool", "boolean":
		return gooq.LocalTypeBool
	case "tinyint", "smallint", "mediumint", "int", "int2", "int4", "integer":
		return gooq.LocalTypeInt
	case "bigint", "int8":
		return gooq.LocalTypeInt64
	case "float", "float4", "float8", "double", "real":
		return gooq.LocalTypeFloat64
	case "numeric", "decimal":
		return gooq.LocalTypeDecimal
	case "date":
		return gooq.LocalTypeDate
	case "time", "timetz":
		return gooq.LocalTypeTime
	case "datetime", "timestamp", "timestamptz":
		return gooq.LocalTypeDatetime
	case "json":
		return gooq.LocalTypeJson
	case "jsonb":
		return gooq.LocalTypeJsonb
	case "uuid":
		return gooq.LocalTypeUUID
	case "bytea", "blob", "binary", "varbinary":
		return gooq.LocalTypeBytes
	case "_int2", "_int4", "_int":
		return gooq.LocalTypeIntSlice
	case "_int8":
		return gooq.LocalTypeInt64Slice
	case "_text", "_varchar", "_bpchar":
		return gooq.LocalTypeStringSlice
	case "_bool":
		return gooq.LocalTypeBoolSlice
	case "_float4":
		return gooq.LocalTypeFloat32Slice
	case "_float8":
		return gooq.LocalTypeFloat64Slice
	case "_bytea":
		return gooq.LocalTypeBytesSlice
	case "_uuid":
		return gooq.LocalTypeUUIDSlice
	case "interval", "inet", "cidr", "macaddr":
		return gooq.LocalTypeString
	default:
		return gooq.LocalTypeString
	}
}

// QueryStrings 把每一行的第一列扫成字符串。
func QueryStrings(ctx context.Context, query Queryer, statement string, args ...any) (names []string, err error) {
	rows, err := query.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = closeRows(rows, err)
	}()
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return names, nil
}

// QueryMaps 把每一行扫成列名小写的文本映射。NULL 写成空字符串。
func QueryMaps(ctx context.Context, query Queryer, statement string, args ...any) (records []map[string]string, err error) {
	rows, err := query.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = closeRows(rows, err)
	}()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		values := make([]any, len(columns))
		targets := make([]any, len(columns))
		for i := range values {
			targets[i] = &values[i]
		}
		if err = rows.Scan(targets...); err != nil {
			return nil, err
		}
		record := make(map[string]string, len(columns))
		for i, column := range columns {
			record[strings.ToLower(column)] = scalarText(values[i])
		}
		records = append(records, record)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func scalarText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}

// Tables 返回当前 schema 里的用户表名。查询在主库上执行。
func (d *DB) Tables(ctx context.Context) ([]string, error) {
	conn, err := d.writeDB()
	if err != nil {
		return nil, err
	}
	return d.driver.Tables(ctx, conn)
}

// TableFields 按定义顺序返回一张表的列。表名必须是普通标识符。查询在主库上执行。
func (d *DB) TableFields(ctx context.Context, table string) ([]Column, error) {
	if err := validIdent(table); err != nil {
		return nil, err
	}
	conn, err := d.writeDB()
	if err != nil {
		return nil, err
	}
	return d.driver.Columns(ctx, conn, table)
}
