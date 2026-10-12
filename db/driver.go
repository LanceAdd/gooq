package db

import (
	"context"
	"database/sql/driver"
	"fmt"
	"sync"

	"github.com/lanceadd/gooq"
)

// Driver 打开数据库并读取目录。
// 驱动包在 init 里调用 Register。链接前缀决定用哪个驱动。
// SQL 渲染留在 gooq.DialectSpec，另行注册。
type Driver interface {
	// Names 返回这个驱动接受的链接前缀。
	Names() []string
	// Dialect 返回这个驱动打开的连接所使用的渲染方言。
	Dialect() gooq.Dialect
	// SQLDriver 返回传给 sql.Open 的 database/sql 驱动名。
	SQLDriver() string
	// Underlying 返回在新连接上执行会话 SQL 的驱动。
	Underlying() driver.Driver
	// DSN 把解析后的 GoFrame 链接编成驱动 DSN。
	DSN(link ParsedLink) (string, error)
	// ApplySession 把字符集和时区参数加进 DSN。
	ApplySession(dsn string, cfg Config) string
	// Tables 返回用户表名。
	Tables(ctx context.Context, query Queryer) ([]string, error)
	// Columns 按定义顺序返回一张表的列。
	Columns(ctx context.Context, query Queryer, table string) ([]Column, error)
	// SchemaSQL 返回每条新连接上用来选择 schema 的语句。
	// 空字符串只记下 schema，不执行语句。
	SchemaSQL(name string) (string, error)
}

var (
	driverMu sync.RWMutex
	drivers  = map[string]Driver{}
)

// Register 按驱动的每个链接前缀注册它。
// 同一个前缀注册两次时返回错误。驱动为空或前缀为空时也返回错误。
func Register(d Driver) error {
	if d == nil {
		return fmt.Errorf("gooq: database driver is nil")
	}
	names := d.Names()
	if len(names) == 0 {
		return fmt.Errorf("gooq: database driver %s has no link prefix", d.Dialect())
	}
	driverMu.Lock()
	defer driverMu.Unlock()
	for _, name := range names {
		if name == "" {
			return fmt.Errorf("gooq: database driver %s has an empty link prefix", d.Dialect())
		}
		if _, exists := drivers[name]; exists {
			return fmt.Errorf("gooq: database driver %q is already registered", name)
		}
	}
	for _, name := range names {
		drivers[name] = d
	}
	return nil
}

func driverByLink(name string) Driver {
	driverMu.RLock()
	defer driverMu.RUnlock()
	return drivers[name]
}

func driverByDialect(dialect gooq.Dialect) Driver {
	driverMu.RLock()
	defer driverMu.RUnlock()
	for _, d := range drivers {
		if d.Dialect() == dialect {
			return d
		}
	}
	return nil
}

func underlyingByName(sqlDriver string) (driver.Driver, error) {
	driverMu.RLock()
	defer driverMu.RUnlock()
	seen := map[Driver]bool{}
	for _, d := range drivers {
		if seen[d] {
			continue
		}
		seen[d] = true
		if d.SQLDriver() == sqlDriver {
			return d.Underlying(), nil
		}
	}
	return nil, fmt.Errorf("gooq: sql driver %q is not registered", sqlDriver)
}
