package db

import (
	"context"
	"database/sql/driver"
	"fmt"

	"github.com/lanceadd/gooq"
)

// Schema returns a database view bound to another schema.
// The registered driver supplies the session statement. An empty statement records the name only.
// The caller closes a view that opened its own pools. Closing it does not close the original DB.
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

// GetSchema returns the schema selected by Schema.
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

// Ensure sql.OpenDB accepts the connector.
var _ driver.Connector = initConnector{}
