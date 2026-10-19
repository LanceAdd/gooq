package db

import (
	"context"
	"database/sql/driver"
	"fmt"

	"github.com/lanceadd/gooq"
)

// Schema 返回绑定到另一个 schema 的数据库视图。
// 已注册的驱动提供会话语句。语句为空时只记下名字，不执行语句，关闭这个视图也不会关闭原连接池。
// 语句非空时为视图打开新的连接池。调用方要关闭这个视图。关闭它不会关闭原来的 DB。
func (d *DB) Schema(name string) (*DB, error) {
	if name == "" || name == d.schema {
		return d, nil
	}
	if err := validIdent(name); err != nil {
		return nil, err
	}
	if d.driver == nil {
		return nil, fmt.Errorf("gooq: database driver is not set")
	}
	initSQL, err := d.driver.SchemaSQL(name)
	if err != nil {
		return nil, err
	}
	if initSQL == "" {
		copied := *d
		copied.schema = name
		copied.shared = true
		return &copied, nil
	}
	copied := *d
	copied.schema = name
	copied.shared = false
	copied.masters = nil
	copied.slaves = nil
	for _, item := range d.masters {
		opened, openErr := reopenNode(item, initSQL)
		if openErr != nil {
			closeErr := copied.Close()
			if closeErr != nil {
				return nil, fmt.Errorf("%w; close: %v", openErr, closeErr)
			}
			return nil, openErr
		}
		copied.masters = append(copied.masters, opened)
	}
	for _, item := range d.slaves {
		opened, openErr := reopenNode(item, initSQL)
		if openErr != nil {
			closeErr := copied.Close()
			if closeErr != nil {
				return nil, fmt.Errorf("%w; close: %v", openErr, closeErr)
			}
			return nil, openErr
		}
		copied.slaves = append(copied.slaves, opened)
	}
	return &copied, nil
}

// GetSchema 返回 Schema 选中的 schema 名。没有选过时为空字符串。
func (d *DB) GetSchema() string {
	return d.schema
}

func schemaSQL(dialect gooq.Dialect, name string) (string, error) {
	drv := driverByDialect(dialect)
	if drv == nil {
		return "", fmt.Errorf("gooq: database driver for dialect %s is not registered", dialect)
	}
	return drv.SchemaSQL(name)
}

func reopenNode(item node, initSQL string) (node, error) {
	cfg := Config{
		MaxOpenConns:    item.maxOpen,
		MaxIdleConns:    item.maxIdle,
		ConnMaxLifetime: item.maxLife,
		MaxIdleTime:     item.maxIdleFor,
	}
	sqlDB, err := openSQL(item.driverName, item.dsn, initSQL, cfg)
	if err != nil {
		return node{}, err
	}
	item.sql = sqlDB
	return item, nil
}

type initConnector struct {
	driver  driver.Driver
	dsn     string
	initSQL string
}

func (c initConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.driver.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	if err = execDriver(ctx, conn, c.initSQL); err != nil {
		closeErr := conn.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("%w; close: %v", err, closeErr)
		}
		return nil, err
	}
	return conn, nil
}

func (c initConnector) Driver() driver.Driver {
	return c.driver
}

func execDriver(ctx context.Context, conn driver.Conn, query string) error {
	if execer, ok := conn.(driver.ExecerContext); ok {
		_, err := execer.ExecContext(ctx, query, nil)
		return err
	}
	if execer, ok := conn.(driver.Execer); ok {
		_, err := execer.Exec(query, nil)
		return err
	}
	return fmt.Errorf("gooq: driver connection cannot execute %s", query)
}

// 编译期确认 initConnector 实现 driver.Connector，从而能传给 sql.OpenDB。
var _ driver.Connector = initConnector{}
