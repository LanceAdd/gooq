package db

import (
	"context"
	"database/sql"
	"fmt"
)

// Stmt 是绑定到一个连接池或事务的预编译语句。
type Stmt struct {
	stmt  *sql.Stmt
	db    *DB
	query string
}

// Prepare 预编译语句。master 为 true 时使用主库。
// ctx 里已有同一组的事务时，改在该事务上预编译。否则走读库。
func (d *DB) Prepare(ctx context.Context, query string, master ...bool) (*Stmt, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if tx := TxFromContext(ctx, d.group); tx != nil {
		return tx.Prepare(ctx, query)
	}
	useMaster := len(master) > 0 && master[0]
	var stmt *sql.Stmt
	err := d.observe(ctx, query, nil, func(ctx context.Context) error {
		var conn *sql.DB
		var err error
		if useMaster {
			conn, err = d.writeDB()
		} else {
			conn, err = d.readDB()
		}
		if err != nil {
			return err
		}
		stmt, err = conn.PrepareContext(ctx, query)
		return err
	})
	if err != nil {
		return nil, err
	}
	if stmt == nil {
		return &Stmt{db: d, query: query}, nil
	}
	return &Stmt{stmt: stmt, db: d, query: query}, nil
}

// Prepare 在事务上预编译语句。
func (t *Tx) Prepare(ctx context.Context, query string) (*Stmt, error) {
	var stmt *sql.Stmt
	err := t.db.observe(ctx, query, nil, func(ctx context.Context) error {
		var err error
		stmt, err = t.sql.PrepareContext(ctx, query)
		return err
	})
	if err != nil {
		return nil, err
	}
	if stmt == nil {
		return &Stmt{db: t.db, query: query}, nil
	}
	return &Stmt{stmt: stmt, db: t.db, query: query}, nil
}

// Exec 执行预编译语句。语句为空时返回空结果，不访问数据库。
func (s *Stmt) Exec(ctx context.Context, args ...any) (sql.Result, error) {
	if s.stmt == nil {
		return dryResult{}, nil
	}
	var result sql.Result = dryResult{}
	err := s.db.observe(ctx, s.query, args, func(ctx context.Context) error {
		var err error
		result, err = s.stmt.ExecContext(ctx, s.db.bindArgs(args)...)
		return err
	})
	return result, err
}

// Query 执行预编译查询。调用方负责关闭 rows。语句为空时返回 nil, nil。
func (s *Stmt) Query(ctx context.Context, args ...any) (*sql.Rows, error) {
	if s.stmt == nil || (s.db != nil && s.db.dryRun != nil && s.db.dryRun.Val()) {
		if s.db != nil {
			_ = s.db.observe(ctx, s.query, args, func(context.Context) error { return nil })
		}
		return nil, fmt.Errorf("gooq: Query is not available in dry run")
	}
	var rows *sql.Rows
	err := s.db.observe(ctx, s.query, args, func(ctx context.Context) error {
		var err error
		rows, err = s.stmt.QueryContext(ctx, s.db.bindArgs(args)...)
		return err
	})
	return rows, err
}

// Close 关闭预编译语句。接收者或语句为空时什么都不做。
func (s *Stmt) Close() error {
	if s == nil || s.stmt == nil {
		return nil
	}
	return s.stmt.Close()
}
