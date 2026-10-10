package db

import (
	"context"
	"database/sql/driver"
	"fmt"
	"sync"

	"github.com/lanceadd/gooq"
)

// Driver opens a database and reads its catalog.
// A driver package calls Register from init. The link prefix selects the driver.
// SQL rendering stays on gooq.DialectSpec and is registered separately.
type Driver interface {
	// Names lists link prefixes this driver accepts.
	Names() []string
	// Dialect is the rendering dialect for connections this driver opens.
	Dialect() gooq.Dialect
	// SQLDriver is the database/sql driver name passed to sql.Open.
	SQLDriver() string
	// Underlying returns the driver used to run session SQL on a new connection.
	Underlying() driver.Driver
	// DSN builds a driver DSN from a parsed GoFrame link.
	DSN(link ParsedLink) (string, error)
	// ApplySession adds charset and timezone parameters to the DSN.
	ApplySession(dsn string, cfg Config) string
	// Tables returns user table names.
	Tables(ctx context.Context, query Queryer) ([]string, error)
	// Columns returns columns of one table in definition order.
	Columns(ctx context.Context, query Queryer, table string) ([]Column, error)
	// SchemaSQL returns the statement that selects a schema on each new connection.
	// An empty string records the schema and does not run a statement.
	SchemaSQL(name string) (string, error)
}

var (
	driverMu sync.RWMutex
	drivers  = map[string]Driver{}
)

// Register installs a database driver under each of its link prefixes.
// Registering the same prefix twice returns an error.
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
