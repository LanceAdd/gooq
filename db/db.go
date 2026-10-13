// Package db 打开 MySQL、PostgreSQL 和 SQLite 连接，并执行 gooq 已经渲染好的 SQL。
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

// Role 选择节点属于主库还是从库。
type Role string

const (
	// RoleMaster 承担写入。没有从库时，读取也走主库。
	RoleMaster Role = "master"
	// RoleSlave 承担读取，除非这次操作强制走主库。
	RoleSlave Role = "slave"
)

// Route 把后续操作固定到一个连接池。
type Route int

const (
	// RouteAuto 有从库时读取走从库，写入走主库。
	RouteAuto Route = iota
	// RouteMaster 把操作发到主库。
	RouteMaster
	// RouteSlave 把操作发到从库。没有从库时用主库。
	RouteSlave
)

// Config 是一个连接节点。Role 为空时视为主库。Weight 小于等于 0 时按 1 计。
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

// DB 是一组共用同一方言的主库和从库连接池。
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

// Tx 是在主库连接上开启的事务。
type Tx struct {
	sql     *sql.Tx
	db      *DB
	dialect gooq.Dialect
	ctx     context.Context
	depth   int
	closed  bool
}

// Queryer 是连接上用来读行和执行目录语句的部分。
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Open 解析 GoFrame 风格的链接，并探测数据库是否可连。
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
			if opened.sql != nil {
				_ = opened.sql.Close()
			}
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
	dsn, err = configureSession(drv, dsn, cfg)
	if err != nil {
		return node{}, nil, err
	}
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

func configureSession(drv Driver, dsn string, cfg Config) (string, error) {
	if configurer, ok := drv.(SessionConfigurer); ok {
		return configurer.ConfigureSession(dsn, cfg)
	}
	return drv.ApplySession(dsn, cfg), nil
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

// Close 关闭全部主库和从库连接池。共享视图什么都不做。
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

// Group 返回配置组名。空字符串表示这个 DB 来自 Open。
func (d *DB) Group() string {
	return d.group
}

// Master 返回把后续操作都发到主库的视图。
func (d *DB) Master() *DB {
	return d.WithRoute(RouteMaster)
}

// Slave 返回把后续操作发到从库的视图。没有从库时仍落到主库。
func (d *DB) Slave() *DB {
	return d.WithRoute(RouteSlave)
}

// WithRoute 返回使用 route 的视图，底层连接池与原对象共享。
// 关闭视图不会关闭这些连接池，要关闭原来的 DB。
// route 已经相同时返回 d 本身。
func (d *DB) WithRoute(route Route) *DB {
	if d == nil || d.route == route {
		return d
	}
	copied := *d
	copied.route = route
	// schema 视图自己拥有 initSQL 打开的池。路由视图继续拥有这些池，
	// Schema(name).Master() 仍能被 Close 关掉。原库的路由视图保持 shared。
	if d.shared || d.schema == "" {
		copied.shared = true
	}
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

// Dialect 返回这条连接渲染 SQL 所用的方言。
func (d *DB) Dialect() gooq.Dialect {
	return d.dialect
}

// Dialect 返回事务所属连接的方言。
func (t *Tx) Dialect() gooq.Dialect {
	return t.dialect
}

// Query 执行查询并返回原始行。调用方负责关闭 rows。有从库时走从库。
func (d *DB) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if d != nil && d.dryRun != nil && d.dryRun.Val() {
		_ = d.observe(ctx, query, args, func(context.Context) error { return nil })
		return nil, fmt.Errorf("gooq: Query is not available in dry run")
	}
	var rows *sql.Rows
	err := d.observe(ctx, query, args, func(ctx context.Context) error {
		conn, err := d.executor(ctx, false)
		if err != nil {
			return err
		}
		rows, err = conn.QueryContext(ctx, query, d.bindArgs(args)...)
		return err
	})
	return rows, err
}

// Exec 在主库上执行写入语句。
func (d *DB) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	var result sql.Result = dryResult{}
	err := d.observe(ctx, query, args, func(ctx context.Context) error {
		conn, err := d.executor(ctx, true)
		if err != nil {
			return err
		}
		result, err = conn.ExecContext(ctx, query, d.bindArgs(args)...)
		return err
	})
	return result, err
}

// Query 在事务内执行查询并返回原始行。调用方负责关闭 rows。
func (t *Tx) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	var rows *sql.Rows
	err := t.db.observe(ctx, query, args, func(ctx context.Context) error {
		var err error
		rows, err = t.sql.QueryContext(ctx, query, t.db.bindArgs(args)...)
		return err
	})
	return rows, err
}

// Exec 在事务内执行语句。
func (t *Tx) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	var result sql.Result = dryResult{}
	err := t.db.observe(ctx, query, args, func(ctx context.Context) error {
		var err error
		result, err = t.sql.ExecContext(ctx, query, t.db.bindArgs(args)...)
		return err
	})
	return result, err
}

// GetAll 读取全部行。没有行时返回 nil。
func (d *DB) GetAll(ctx context.Context, query string, args ...any) (Result, error) {
	var result Result
	err := d.observe(ctx, query, args, func(ctx context.Context) error {
		conn, err := d.executor(ctx, false)
		if err != nil {
			return err
		}
		result, err = readAll(ctx, conn, d.driver, query, d.bindArgs(args)...)
		return err
	})
	return result, err
}

// GetAll 在事务内读取全部行。没有行时返回 nil。
func (t *Tx) GetAll(ctx context.Context, query string, args ...any) (Result, error) {
	var result Result
	err := t.db.observe(ctx, query, args, func(ctx context.Context) error {
		var err error
		result, err = readAll(ctx, t.sql, t.db.driver, query, t.db.bindArgs(args)...)
		return err
	})
	return result, err
}

// GetOne 读取第一行。没有行时返回 nil。
func (d *DB) GetOne(ctx context.Context, query string, args ...any) (Record, error) {
	var record Record
	err := d.observe(ctx, query, args, func(ctx context.Context) error {
		conn, err := d.executor(ctx, false)
		if err != nil {
			return err
		}
		record, err = readOne(ctx, conn, d.driver, query, d.bindArgs(args)...)
		return err
	})
	return record, err
}

// GetOne 在事务内读取第一行。没有行时返回 nil。
func (t *Tx) GetOne(ctx context.Context, query string, args ...any) (Record, error) {
	var record Record
	err := t.db.observe(ctx, query, args, func(ctx context.Context) error {
		var err error
		record, err = readOne(ctx, t.sql, t.db.driver, query, t.db.bindArgs(args)...)
		return err
	})
	return record, err
}

// GetValue 读取第一行的第一列。没有行或值为 NULL 时返回 nil。
func (d *DB) GetValue(ctx context.Context, query string, args ...any) (any, error) {
	var value any
	err := d.observe(ctx, query, args, func(ctx context.Context) error {
		conn, err := d.executor(ctx, false)
		if err != nil {
			return err
		}
		value, err = readValue(ctx, conn, d.driver, query, d.bindArgs(args)...)
		return err
	})
	return value, err
}

// GetValue 在事务内读取第一行的第一列。没有行或值为 NULL 时返回 nil。
func (t *Tx) GetValue(ctx context.Context, query string, args ...any) (any, error) {
	var value any
	err := t.db.observe(ctx, query, args, func(ctx context.Context) error {
		var err error
		value, err = readValue(ctx, t.sql, t.db.driver, query, t.db.bindArgs(args)...)
		return err
	})
	return value, err
}

// Value 把第一行的第一列写入 dest。
// dest 是指向一个单元格的非空指针，例如 *time.Time、*sql.NullString 或 *int。
// 没有行时返回 sql.ErrNoRows。NULL 写成零值。行结构体用 GetOne 再 Record.Struct。
func (d *DB) Value(ctx context.Context, dest any, query string, args ...any) error {
	if err := CheckValue(dest); err != nil {
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
	return Assign(dest, value)
}

// Value 在事务内把第一行的第一列写入 dest。约束与 DB.Value 相同。
func (t *Tx) Value(ctx context.Context, dest any, query string, args ...any) error {
	if err := CheckValue(dest); err != nil {
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
	return Assign(dest, value)
}

// Column 把每一行的第一列写入 dest。
// dest 是单元格切片的指针。没有行时写成空切片。
func (d *DB) Column(ctx context.Context, dest any, query string, args ...any) error {
	if err := CheckColumn(dest); err != nil {
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

// Column 在事务内把每一行的第一列写入 dest。没有行时写成空切片。
func (t *Tx) Column(ctx context.Context, dest any, query string, args ...any) error {
	if err := CheckColumn(dest); err != nil {
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

func (d *DB) executor(ctx context.Context, write bool) (Queryer, error) {
	if tx := TxFromContext(ctx, d.group); tx != nil {
		return tx.sql, nil
	}
	if write {
		return d.writeDB()
	}
	return d.readDB()
}

func (d *DB) scalar(ctx context.Context, query string, args ...any) (any, bool, error) {
	var (
		value any
		found bool
	)
	err := d.observe(ctx, query, args, func(ctx context.Context) error {
		conn, err := d.executor(ctx, false)
		if err != nil {
			return err
		}
		value, found, err = readScalar(ctx, conn, d.driver, query, d.bindArgs(args)...)
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
		value, found, err = readScalar(ctx, t.sql, t.db.driver, query, t.db.bindArgs(args)...)
		return err
	})
	return value, found, err
}

func (d *DB) column(ctx context.Context, query string, args ...any) ([]any, error) {
	var values []any
	err := d.observe(ctx, query, args, func(ctx context.Context) error {
		conn, err := d.executor(ctx, false)
		if err != nil {
			return err
		}
		values, err = readColumn(ctx, conn, d.driver, query, d.bindArgs(args)...)
		return err
	})
	return values, err
}

func (t *Tx) column(ctx context.Context, query string, args ...any) ([]any, error) {
	var values []any
	err := t.db.observe(ctx, query, args, func(ctx context.Context) error {
		var err error
		values, err = readColumn(ctx, t.sql, t.db.driver, query, t.db.bindArgs(args)...)
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
		if err := Assign(elem.Addr().Interface(), value); err != nil {
			return err
		}
		out = reflect.Append(out, elem)
	}
	slice.Set(out)
	return nil
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
