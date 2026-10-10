package db

import (
	"context"
	"database/sql"
)

// Stmt is a prepared statement bound to one pool or transaction.
type Stmt struct {
	stmt  *sql.Stmt
	db    *DB
	query string
}

// Prepare prepares a statement. A true master flag uses a master pool.
// A transaction in ctx prepares on that transaction.
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

// Prepare prepares a statement on the transaction.
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

// Exec executes the prepared statement.
func (s *Stmt) Exec(ctx context.Context, args ...any) (sql.Result, error) {
	if s.stmt == nil {
		return dryResult{}, nil
	}
	var result sql.Result
	err := s.db.observe(ctx, s.query, args, func(ctx context.Context) error {
		var err error
		result, err = s.stmt.ExecContext(ctx, args...)
		return err
	})
	return result, err
}

// Query runs the prepared query. The caller closes the rows.
func (s *Stmt) Query(ctx context.Context, args ...any) (*sql.Rows, error) {
	if s.stmt == nil {
		return nil, nil
	}
	var rows *sql.Rows
	err := s.db.observe(ctx, s.query, args, func(ctx context.Context) error {
		var err error
		rows, err = s.stmt.QueryContext(ctx, args...)
		return err
	})
	return rows, err
}

// Close closes the prepared statement.
func (s *Stmt) Close() error {
	if s == nil || s.stmt == nil {
		return nil
	}
	return s.stmt.Close()
}
