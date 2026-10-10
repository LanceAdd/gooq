package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lanceadd/gooq"
)

// Propagation decides how Transaction joins a transaction already stored in ctx.
type Propagation string

const (
	// PropagationNested starts a savepoint when ctx already has a transaction.
	// Otherwise it starts a transaction. This is the default.
	PropagationNested Propagation = "nested"
	// PropagationRequired joins the current transaction, or starts one.
	PropagationRequired Propagation = "required"
	// PropagationSupports joins the current transaction, or runs with tx == nil.
	PropagationSupports Propagation = "supports"
	// PropagationRequiresNew suspends the current transaction and starts another.
	PropagationRequiresNew Propagation = "requires_new"
	// PropagationNotSupported runs with tx == nil and suspends any current transaction.
	PropagationNotSupported Propagation = "not_supported"
	// PropagationMandatory joins the current transaction, or returns an error.
	PropagationMandatory Propagation = "mandatory"
	// PropagationNever runs with tx == nil, or returns an error when a transaction exists.
	PropagationNever Propagation = "never"
)

// TxOptions controls isolation, read-only mode, and propagation.
type TxOptions struct {
	Propagation Propagation
	Isolation   sql.IsolationLevel
	ReadOnly    bool
}

type txCtxKey struct {
	group string
}

// Begin starts a transaction on a master connection.
func (d *DB) Begin(ctx context.Context) (*Tx, error) {
	return d.BeginTx(ctx, TxOptions{})
}

// BeginTx starts a transaction on a master connection using opts.
// Isolation and ReadOnly are passed to database/sql. Propagation is used by Transaction.
func (d *DB) BeginTx(ctx context.Context, opts TxOptions) (*Tx, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	conn, err := d.writeDB()
	if err != nil {
		return nil, err
	}
	sqlTx, err := conn.BeginTx(ctx, &sql.TxOptions{
		Isolation: opts.Isolation,
		ReadOnly:  opts.ReadOnly,
	})
	if err != nil {
		return nil, err
	}
	return &Tx{sql: sqlTx, db: d, dialect: d.dialect, ctx: ctx}, nil
}

// Transaction runs fn inside a transaction and commits when fn returns nil.
// The default propagation is nested. fn receives a context that carries the transaction,
// so later queries on the same group join it.
func (d *DB) Transaction(ctx context.Context, fn func(ctx context.Context, tx *Tx) error) error {
	return d.TransactionWithOptions(ctx, TxOptions{}, fn)
}

// TransactionWithOptions runs fn with the given propagation and transaction options.
func (d *DB) TransactionWithOptions(ctx context.Context, opts TxOptions, fn func(ctx context.Context, tx *Tx) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Propagation == "" {
		opts.Propagation = PropagationNested
	}
	current := TxFromContext(ctx, d.group)
	switch opts.Propagation {
	case PropagationRequired:
		if current != nil {
			return fn(ctx, current)
		}
		return d.runNew(ctx, opts, fn)
	case PropagationSupports:
		if current == nil {
			return fn(ctx, nil)
		}
		return fn(ctx, current)
	case PropagationMandatory:
		if current == nil {
			return fmt.Errorf("gooq: propagation mandatory requires an existing transaction")
		}
		return fn(ctx, current)
	case PropagationRequiresNew:
		return d.runNew(WithoutTx(ctx, d.group), opts, fn)
	case PropagationNotSupported:
		return fn(WithoutTx(ctx, d.group), nil)
	case PropagationNever:
		if current != nil {
			return fmt.Errorf("gooq: propagation never cannot run inside a transaction")
		}
		return fn(ctx, nil)
	case PropagationNested:
		if current != nil {
			return current.Transaction(ctx, fn)
		}
		return d.runNew(ctx, opts, fn)
	default:
		return fmt.Errorf("gooq: unsupported propagation %q", opts.Propagation)
	}
}

func (d *DB) runNew(ctx context.Context, opts TxOptions, fn func(ctx context.Context, tx *Tx) error) (err error) {
	tx, err := d.BeginTx(ctx, opts)
	if err != nil {
		return err
	}
	ctx = WithTx(ctx, tx)
	defer func() {
		if recovered := recover(); recovered != nil {
			rbErr := tx.Rollback()
			err = fmt.Errorf("gooq: transaction panic: %v", recovered)
			if rbErr != nil {
				err = fmt.Errorf("%w; rollback: %v", err, rbErr)
			}
		}
	}()
	err = fn(ctx, tx)
	return finishTx(tx, err)
}

// Transaction starts a savepoint when called on an open transaction.
func (t *Tx) Transaction(ctx context.Context, fn func(ctx context.Context, tx *Tx) error) error {
	if t == nil {
		return fmt.Errorf("gooq: transaction is nil")
	}
	if ctx == nil {
		ctx = t.ctx
	}
	if err := t.beginNested(ctx); err != nil {
		return err
	}
	ctx = WithTx(ctx, t)
	err := fn(ctx, t)
	return finishTx(t, err)
}

// Commit commits the transaction, or releases the current savepoint when nested.
func (t *Tx) Commit() error {
	if t.depth > 0 {
		t.depth--
		_, err := t.sql.ExecContext(t.context(), "RELEASE SAVEPOINT "+savepointName(t.depth))
		return err
	}
	err := t.sql.Commit()
	if err == nil {
		t.closed = true
	}
	return err
}

// Rollback rolls the transaction back, or rolls back to the current savepoint when nested.
func (t *Tx) Rollback() error {
	if t.depth > 0 {
		t.depth--
		_, err := t.sql.ExecContext(t.context(), "ROLLBACK TO SAVEPOINT "+savepointName(t.depth))
		return err
	}
	err := t.sql.Rollback()
	if err == nil {
		t.closed = true
	}
	return err
}

// SavePoint marks a named savepoint inside the transaction.
func (t *Tx) SavePoint(ctx context.Context, name string) error {
	quoted, err := quoteIdent(t.dialect, name)
	if err != nil {
		return err
	}
	_, err = t.sql.ExecContext(ctx, "SAVEPOINT "+quoted)
	return err
}

// RollbackTo rolls the transaction back to a named savepoint.
func (t *Tx) RollbackTo(ctx context.Context, name string) error {
	quoted, err := quoteIdent(t.dialect, name)
	if err != nil {
		return err
	}
	_, err = t.sql.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+quoted)
	return err
}

// IsClosed reports whether Commit or Rollback has finished the outer transaction.
func (t *Tx) IsClosed() bool {
	return t == nil || t.closed
}

// DB returns the database that started the transaction.
func (t *Tx) DB() *DB {
	if t == nil {
		return nil
	}
	return t.db
}

func (t *Tx) beginNested(ctx context.Context) error {
	name := savepointName(t.depth)
	if _, err := t.sql.ExecContext(ctx, "SAVEPOINT "+name); err != nil {
		return err
	}
	t.depth++
	return nil
}

func (t *Tx) context() context.Context {
	if t.ctx != nil {
		return t.ctx
	}
	return context.Background()
}

func savepointName(depth int) string {
	return fmt.Sprintf("gooq_sp_%d", depth)
}

func finishTx(tx *Tx, err error) error {
	if tx == nil {
		return err
	}
	if err != nil {
		rbErr := tx.Rollback()
		if rbErr != nil {
			return fmt.Errorf("%w; rollback: %v", err, rbErr)
		}
		return err
	}
	return tx.Commit()
}

// WithTx stores tx in ctx under its database group.
func WithTx(ctx context.Context, tx *Tx) context.Context {
	if tx == nil || tx.db == nil {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, txCtxKey{group: tx.db.group}, tx)
}

// WithoutTx removes the transaction for group from ctx.
func WithoutTx(ctx context.Context, group string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, txCtxKey{group: group}, (*Tx)(nil))
}

// TxFromContext returns the open transaction stored for group.
func TxFromContext(ctx context.Context, group string) *Tx {
	if ctx == nil {
		return nil
	}
	tx, _ := ctx.Value(txCtxKey{group: group}).(*Tx)
	if tx == nil || tx.IsClosed() || tx.sql == nil {
		return nil
	}
	return tx
}

func quoteIdent(dialect gooq.Dialect, name string) (string, error) {
	if err := validIdent(name); err != nil {
		return "", err
	}
	if dialect == gooq.DialectMySQL {
		return "`" + name + "`", nil
	}
	return `"` + name + `"`, nil
}
