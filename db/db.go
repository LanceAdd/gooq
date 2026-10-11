// Package db opens MySQL, PostgreSQL, and SQLite connections and executes SQL
// that gooq has already rendered.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"time"

	"github.com/gogf/gf/v2/container/gtype"
	"github.com/gogf/gf/v2/os/glog"

	"github.com/lanceadd/gooq"
)

// Role selects which pool a node belongs to.
type Role string

const (
	// RoleMaster serves writes and is the fallback when no slave exists.
	RoleMaster Role = "master"
	// RoleSlave serves reads when a query does not force the master.
	RoleSlave Role = "slave"
)

// Route forces a later operation onto one pool.
type Route int

const (
	// RouteAuto sends reads to a slave when one exists, and writes to a master.
	RouteAuto Route = iota
	// RouteMaster sends the operation to a master.
	RouteMaster
	// RouteSlave sends the operation to a slave, or a master when no slave exists.
	RouteSlave
)

// Config is one connection node. Role defaults to master. Weight defaults to 1.
type Config struct {
	Link            string
	Role            Role
	Weight          int
	Debug           bool
	DryRun          bool
	Charset         string
	Timezone        string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	MaxIdleTime     time.Duration
}

type node struct {
	sql        *sql.DB
	weight     int
	dsn        string
	driverName string
	maxOpen    int
	maxIdle    int
	maxLife    time.Duration
	maxIdleFor time.Duration
}

// DB is a group of master and slave pools that share one dialect.
type DB struct {
	group   string
	masters []node
	slaves  []node
	route   Route
	schema  string
	shared  bool
	dialect gooq.Dialect
	driver  Driver
	debug   *gtype.Bool
	dryRun  *gtype.Bool
	logger  *glog.Logger
}

// Tx is a transaction started on a master connection.
type Tx struct {
	sql     *sql.Tx
	db      *DB
	dialect gooq.Dialect
	ctx     context.Context
	depth   int
	closed  bool
}

// Queryer is the subset of a connection used to read rows and run catalog statements.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Open parses a GoFrame-style link and pings the database.
//
//	mysql:user:pass@tcp(127.0.0.1:3306)/app
//	pgsql:user:pass@tcp(127.0.0.1:5432)/app
//	sqlite::@file(path)
func Open(cfg Config) (*DB, error) {
	return openGroup("", []Config{cfg})
}

func openGroup(group string, configs []Config) (*DB, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("gooq: empty database configuration for group %q", group)
	}
	database := &DB{group: group}
	for _, cfg := range configs {
		opened, drv, err := openNode(cfg)
		if err != nil {
			closeErr := database.Close()
			if closeErr != nil {
				return nil, fmt.Errorf("%w; close: %v", err, closeErr)
			}
			return nil, err
		}
		if database.driver == nil {
			database.driver = drv
			database.dialect = drv.Dialect()
		} else if drv.Dialect() != database.dialect {
			closeErr := database.Close()
			if closeErr != nil {
				return nil, fmt.Errorf("gooq: group %q mixes dialects %s and %s; close: %v", group, database.dialect, drv.Dialect(), closeErr)
			}
			return nil, fmt.Errorf("gooq: group %q mixes dialects %s and %s", group, database.dialect, drv.Dialect())
		}
		if cfg.Role == RoleSlave {
			database.slaves = append(database.slaves, opened)
			continue
		}
		database.masters = append(database.masters, opened)
	}
	if len(database.masters) == 0 {
		closeErr := database.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("gooq: group %q requires a master node; close: %v", group, closeErr)
		}
		return nil, fmt.Errorf("gooq: group %q requires a master node", group)
	}
	database.debug = gtype.NewBool(false)
	database.dryRun = gtype.NewBool(false)
	database.logger = glog.New()
	for _, cfg := range configs {
		if cfg.Debug {
			database.debug.Set(true)
		}
		if cfg.DryRun {
			database.dryRun.Set(true)
		}
	}
	return database, nil
}

func openNode(cfg Config) (node, Driver, error) {
	parsed, err := parseLink(cfg.Link)
	if err != nil {
		return node{}, nil, err
	}
	drv := driverByLink(parsed.Type)
	if drv == nil {
		return node{}, nil, fmt.Errorf("gooq: database driver %q is not registered", parsed.Type)
	}
	dsn, err := drv.DSN(parsed)
	if err != nil {
		return node{}, nil, err
	}
	dsn = drv.ApplySession(dsn, cfg)
	sqlDB, err := openSQL(drv.SQLDriver(), dsn, "", cfg)
	if err != nil {
		return node{}, nil, err
	}
	weight := cfg.Weight
	if weight <= 0 {
		weight = 1
	}
	return node{
		sql:        sqlDB,
		weight:     weight,
		dsn:        dsn,
		driverName: drv.SQLDriver(),
		maxOpen:    cfg.MaxOpenConns,
		maxIdle:    cfg.MaxIdleConns,
		maxLife:    cfg.ConnMaxLifetime,
		maxIdleFor: cfg.MaxIdleTime,
	}, drv, nil
}

func openSQL(driverName, dsn, initSQL string, cfg Config) (*sql.DB, error) {
	var sqlDB *sql.DB
	if initSQL == "" {
		var err error
		sqlDB, err = sql.Open(driverName, dsn)
		if err != nil {
			return nil, err
		}
	} else {
		drv, err := underlyingByName(driverName)
		if err != nil {
			return nil, err
		}
		sqlDB = sql.OpenDB(initConnector{driver: drv, dsn: dsn, initSQL: initSQL})
	}
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}
	if cfg.MaxIdleTime > 0 {
		sqlDB.SetConnMaxIdleTime(cfg.MaxIdleTime)
	}
	if err := sqlDB.Ping(); err != nil {
		closeErr := sqlDB.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("%w; close: %v", err, closeErr)
		}
		return nil, err
	}
	return sqlDB, nil
}

// Close closes every master and slave pool.
func (d *DB) Close() error {
	if d == nil || d.shared {
		return nil
	}
	var errs []error
	for _, item := range d.masters {
		if err := item.sql.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	for _, item := range d.slaves {
		if err := item.sql.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Group returns the configuration group name. An empty name means the DB came from Open.
func (d *DB) Group() string {
	return d.group
}

// Master returns a view that sends every operation to a master.
func (d *DB) Master() *DB {
	return d.WithRoute(RouteMaster)
}

// Slave returns a view that sends every operation to a slave when one is configured.
func (d *DB) Slave() *DB {
	return d.WithRoute(RouteSlave)
}

// WithRoute returns a view that uses route. The underlying pools are shared.
// Closing the view does not close those pools. Close the original DB to release them.
// When d already uses route, WithRoute returns d itself.
func (d *DB) WithRoute(route Route) *DB {
	if d.route == route {
		return d
	}
	copied := *d
	copied.route = route
	copied.shared = true
	return &copied
}

func (d *DB) readDB() (*sql.DB, error) {
	if d.route == RouteMaster {
		return pick(d.masters)
	}
	if d.route == RouteSlave || len(d.slaves) > 0 {
		return pick(prefer(d.slaves, d.masters))
	}
	return pick(d.masters)
}

func (d *DB) writeDB() (*sql.DB, error) {
	if d.route == RouteSlave {
		return pick(prefer(d.slaves, d.masters))
	}
	return pick(d.masters)
}

func prefer(primary, fallback []node) []node {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}

func pick(nodes []node) (*sql.DB, error) {
	if len(nodes) == 0 {
		return nil, fmt.Errorf("gooq: no database node is configured")
	}
	if len(nodes) == 1 {
		return nodes[0].sql, nil
	}
	total := 0
	for _, item := range nodes {
		total += item.weight
	}
	target := rand.IntN(total)
	for _, item := range nodes {
		target -= item.weight
		if target < 0 {
			return item.sql, nil
		}
	}
	return nodes[len(nodes)-1].sql, nil
}

// Dialect returns the dialect used to render SQL for this connection.
func (d *DB) Dialect() gooq.Dialect {
	return d.dialect
}

// Dialect returns the dialect of the transaction's connection.
func (t *Tx) Dialect() gooq.Dialect {
	return t.dialect
}

// Query runs a read query and returns the raw rows. The caller closes the rows.
func (d *DB) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	var rows *sql.Rows
	err := d.observe(ctx, query, args, func(ctx context.Context) error {
		conn, err := d.readDB()
		if err != nil {
			return err
		}
		rows, err = conn.QueryContext(ctx, query, args...)
		return err
	})
	return rows, err
}

// Exec executes a write statement.
func (d *DB) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	var result sql.Result = dryResult{}
	err := d.observe(ctx, query, args, func(ctx context.Context) error {
		conn, err := d.writeDB()
		if err != nil {
			return err
		}
		result, err = conn.ExecContext(ctx, query, args...)
		return err
	})
	return result, err
}

// Exec executes a statement inside the transaction.
func (t *Tx) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	var result sql.Result = dryResult{}
	err := t.db.observe(ctx, query, args, func(ctx context.Context) error {
		var err error
		result, err = t.sql.ExecContext(ctx, query, args...)
		return err
	})
	return result, err
}

// GetAll reads every row. No rows returns a nil result.
func (d *DB) GetAll(ctx context.Context, query string, args ...any) (gooq.Result, error) {
	var result gooq.Result
	err := d.observe(ctx, query, args, func(ctx context.Context) error {
		conn, err := d.readDB()
		if err != nil {
			return err
		}
		result, err = readAll(ctx, conn, query, args...)
		return err
	})
	return result, err
}

// GetAll reads every row inside the transaction.
func (t *Tx) GetAll(ctx context.Context, query string, args ...any) (gooq.Result, error) {
	var result gooq.Result
	err := t.db.observe(ctx, query, args, func(ctx context.Context) error {
		var err error
		result, err = readAll(ctx, t.sql, query, args...)
		return err
	})
	return result, err
}

// GetOne reads the first row. No rows returns a nil record.
func (d *DB) GetOne(ctx context.Context, query string, args ...any) (gooq.Record, error) {
	var record gooq.Record
	err := d.observe(ctx, query, args, func(ctx context.Context) error {
		conn, err := d.readDB()
		if err != nil {
			return err
		}
		record, err = readOne(ctx, conn, query, args...)
		return err
	})
	return record, err
}

// GetOne reads the first row inside the transaction.
func (t *Tx) GetOne(ctx context.Context, query string, args ...any) (gooq.Record, error) {
	var record gooq.Record
	err := t.db.observe(ctx, query, args, func(ctx context.Context) error {
		var err error
		record, err = readOne(ctx, t.sql, query, args...)
		return err
	})
	return record, err
}

// GetValue reads the first column of the first row. No rows returns nil.
func (d *DB) GetValue(ctx context.Context, query string, args ...any) (any, error) {
	var value any
	err := d.observe(ctx, query, args, func(ctx context.Context) error {
		conn, err := d.readDB()
		if err != nil {
			return err
		}
		value, err = readValue(ctx, conn, query, args...)
		return err
	})
	return value, err
}

// GetValue reads the first column of the first row inside the transaction.
func (t *Tx) GetValue(ctx context.Context, query string, args ...any) (any, error) {
	var value any
	err := t.db.observe(ctx, query, args, func(ctx context.Context) error {
		var err error
		value, err = readValue(ctx, t.sql, query, args...)
		return err
	})
	return value, err
}

// Value writes the first column of the first row into dest.
// dest is a non-nil pointer to one cell, such as *time.Time, *sql.NullString, or *int.
// No row returns sql.ErrNoRows. NULL writes the zero value. A row struct uses GetScan.
func (d *DB) Value(ctx context.Context, dest any, query string, args ...any) error {
	if err := gooq.CheckValue(dest); err != nil {
		return err
	}
	if d != nil && d.dryRun != nil && d.dryRun.Val() {
		return d.observe(ctx, query, args, func(context.Context) error { return nil })
	}
	value, found, err := d.scalar(ctx, query, args...)
	if err != nil {
		return err
	}
	if !found {
		return sql.ErrNoRows
	}
	return gooq.Assign(dest, value)
}

// Value writes the first column of the first row into dest inside the transaction.
func (t *Tx) Value(ctx context.Context, dest any, query string, args ...any) error {
	if err := gooq.CheckValue(dest); err != nil {
		return err
	}
	if t != nil && t.db != nil && t.db.dryRun != nil && t.db.dryRun.Val() {
		return t.db.observe(ctx, query, args, func(context.Context) error { return nil })
	}
	value, found, err := t.scalar(ctx, query, args...)
	if err != nil {
		return err
	}
	if !found {
		return sql.ErrNoRows
	}
	return gooq.Assign(dest, value)
}

// Column writes the first column of every row into dest.
// dest is a pointer to a slice of cells. No rows sets an empty slice.
func (d *DB) Column(ctx context.Context, dest any, query string, args ...any) error {
	if err := gooq.CheckColumn(dest); err != nil {
		return err
	}
	if d != nil && d.dryRun != nil && d.dryRun.Val() {
		return d.observe(ctx, query, args, func(context.Context) error { return nil })
	}
	values, err := d.column(ctx, query, args...)
	if err != nil {
		return err
	}
	return assignColumn(dest, values)
}

// Column writes the first column of every row into dest inside the transaction.
func (t *Tx) Column(ctx context.Context, dest any, query string, args ...any) error {
	if err := gooq.CheckColumn(dest); err != nil {
		return err
	}
	if t != nil && t.db != nil && t.db.dryRun != nil && t.db.dryRun.Val() {
		return t.db.observe(ctx, query, args, func(context.Context) error { return nil })
	}
	values, err := t.column(ctx, query, args...)
	if err != nil {
		return err
	}
	return assignColumn(dest, values)
}

func (d *DB) scalar(ctx context.Context, query string, args ...any) (any, bool, error) {
	var (
		value any
		found bool
	)
	err := d.observe(ctx, query, args, func(ctx context.Context) error {
		conn, err := d.readDB()
		if err != nil {
			return err
		}
		value, found, err = readScalar(ctx, conn, query, args...)
		return err
	})
	return value, found, err
}

func (t *Tx) scalar(ctx context.Context, query string, args ...any) (any, bool, error) {
	var (
		value any
		found bool
	)
	err := t.db.observe(ctx, query, args, func(ctx context.Context) error {
		var err error
		value, found, err = readScalar(ctx, t.sql, query, args...)
		return err
	})
	return value, found, err
}

func (d *DB) column(ctx context.Context, query string, args ...any) ([]any, error) {
	var values []any
	err := d.observe(ctx, query, args, func(ctx context.Context) error {
		conn, err := d.readDB()
		if err != nil {
			return err
		}
		values, err = readColumn(ctx, conn, query, args...)
		return err
	})
	return values, err
}

func (t *Tx) column(ctx context.Context, query string, args ...any) ([]any, error) {
	var values []any
	err := t.db.observe(ctx, query, args, func(ctx context.Context) error {
		var err error
		values, err = readColumn(ctx, t.sql, query, args...)
		return err
	})
	return values, err
}

func assignColumn(dest any, values []any) error {
	slice := reflect.ValueOf(dest).Elem()
	elemType := slice.Type().Elem()
	out := reflect.MakeSlice(slice.Type(), 0, len(values))
	for _, value := range values {
		elem := reflect.New(elemType).Elem()
		if err := gooq.Assign(elem.Addr().Interface(), value); err != nil {
			return err
		}
		out = reflect.Append(out, elem)
	}
	slice.Set(out)
	return nil
}

// GetScan writes the result into dest.
func (d *DB) GetScan(ctx context.Context, dest any, query string, args ...any) error {
	return d.observe(ctx, query, args, func(ctx context.Context) error {
		conn, err := d.readDB()
		if err != nil {
			return err
		}
		return scanDest(ctx, conn, dest, query, args...)
	})
}

// GetScan writes the result into dest inside the transaction.
func (t *Tx) GetScan(ctx context.Context, dest any, query string, args ...any) error {
	return t.db.observe(ctx, query, args, func(ctx context.Context) error {
		return scanDest(ctx, t.sql, dest, query, args...)
	})
}

// Tables returns user table names in the current schema.
func (d *DB) Tables(ctx context.Context) ([]string, error) {
	conn, err := d.writeDB()
	if err != nil {
		return nil, err
	}
	return d.driver.Tables(ctx, conn)
}

// TableFields returns columns of one table in definition order.
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
