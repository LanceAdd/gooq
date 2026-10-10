// Package db opens MySQL, PostgreSQL, and SQLite connections and executes SQL
// that gooq has already rendered.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

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
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type node struct {
	sql    *sql.DB
	weight int
}

// DB is a group of master and slave pools that share one dialect.
type DB struct {
	group   string
	masters []node
	slaves  []node
	route   Route
	dialect gooq.Dialect
	kind    dialectKind
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

type queryer interface {
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
		opened, kind, err := openNode(cfg)
		if err != nil {
			closeErr := database.Close()
			if closeErr != nil {
				return nil, fmt.Errorf("%w; close: %v", err, closeErr)
			}
			return nil, err
		}
		if database.kind.sqlDriver == "" {
			database.kind = kind
			database.dialect = kind.dialect
		} else if kind.dialect != database.dialect {
			closeErr := database.Close()
			if closeErr != nil {
				return nil, fmt.Errorf("gooq: group %q mixes dialects %s and %s; close: %v", group, database.dialect, kind.dialect, closeErr)
			}
			return nil, fmt.Errorf("gooq: group %q mixes dialects %s and %s", group, database.dialect, kind.dialect)
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
	return database, nil
}

func openNode(cfg Config) (node, dialectKind, error) {
	parsed, err := parseLink(cfg.Link)
	if err != nil {
		return node{}, dialectKind{}, err
	}
	kind, ok := dialects[parsed.Type]
	if !ok {
		return node{}, dialectKind{}, fmt.Errorf("gooq: unsupported database type %q", parsed.Type)
	}
	dsn, err := kind.dsn(parsed)
	if err != nil {
		return node{}, dialectKind{}, err
	}
	sqlDB, err := sql.Open(kind.sqlDriver, dsn)
	if err != nil {
		return node{}, dialectKind{}, err
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
	if err = sqlDB.Ping(); err != nil {
		closeErr := sqlDB.Close()
		if closeErr != nil {
			return node{}, dialectKind{}, fmt.Errorf("%w; close: %v", err, closeErr)
		}
		return node{}, dialectKind{}, err
	}
	weight := cfg.Weight
	if weight <= 0 {
		weight = 1
	}
	return node{sql: sqlDB, weight: weight}, kind, nil
}

// Close closes every master and slave pool.
func (d *DB) Close() error {
	if d == nil {
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
func (d *DB) WithRoute(route Route) *DB {
	if d.route == route {
		return d
	}
	copied := *d
	copied.route = route
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
	conn, err := d.readDB()
	if err != nil {
		return nil, err
	}
	return conn.QueryContext(ctx, query, args...)
}

// Exec executes a write statement.
func (d *DB) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	conn, err := d.writeDB()
	if err != nil {
		return nil, err
	}
	return conn.ExecContext(ctx, query, args...)
}

// Exec executes a statement inside the transaction.
func (t *Tx) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.sql.ExecContext(ctx, query, args...)
}

// GetAll reads every row. No rows returns a nil result.
func (d *DB) GetAll(ctx context.Context, query string, args ...any) (gooq.Result, error) {
	conn, err := d.readDB()
	if err != nil {
		return nil, err
	}
	return readAll(ctx, conn, query, args...)
}

// GetAll reads every row inside the transaction.
func (t *Tx) GetAll(ctx context.Context, query string, args ...any) (gooq.Result, error) {
	return readAll(ctx, t.sql, query, args...)
}

// GetOne reads the first row. No rows returns a nil record.
func (d *DB) GetOne(ctx context.Context, query string, args ...any) (gooq.Record, error) {
	conn, err := d.readDB()
	if err != nil {
		return nil, err
	}
	return readOne(ctx, conn, query, args...)
}

// GetOne reads the first row inside the transaction.
func (t *Tx) GetOne(ctx context.Context, query string, args ...any) (gooq.Record, error) {
	return readOne(ctx, t.sql, query, args...)
}

// GetValue reads the first column of the first row. No rows returns nil.
func (d *DB) GetValue(ctx context.Context, query string, args ...any) (any, error) {
	conn, err := d.readDB()
	if err != nil {
		return nil, err
	}
	return readValue(ctx, conn, query, args...)
}

// GetValue reads the first column of the first row inside the transaction.
func (t *Tx) GetValue(ctx context.Context, query string, args ...any) (any, error) {
	return readValue(ctx, t.sql, query, args...)
}

// GetScan writes the result into dest.
func (d *DB) GetScan(ctx context.Context, dest any, query string, args ...any) error {
	conn, err := d.readDB()
	if err != nil {
		return err
	}
	return scanDest(ctx, conn, dest, query, args...)
}

// GetScan writes the result into dest inside the transaction.
func (t *Tx) GetScan(ctx context.Context, dest any, query string, args ...any) error {
	return scanDest(ctx, t.sql, dest, query, args...)
}

// Tables returns user table names in the current schema.
func (d *DB) Tables(ctx context.Context) ([]string, error) {
	conn, err := d.writeDB()
	if err != nil {
		return nil, err
	}
	return d.kind.tables(ctx, conn)
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
	return d.kind.columns(ctx, conn, table)
}
