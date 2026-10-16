package db

import (
	"context"
	"database/sql"
	"fmt"
)

// Propagation 决定 Transaction 如何加入 ctx 里已有的事务。
type Propagation string

const (
	// PropagationNested 在 ctx 已有事务时开启保存点，否则开启事务。这是默认值。
	PropagationNested Propagation = "nested"
	// PropagationRequired 加入当前事务。没有事务时开启一个。
	PropagationRequired Propagation = "required"
	// PropagationSupports 加入当前事务。没有事务时以 tx == nil 执行。
	PropagationSupports Propagation = "supports"
	// PropagationRequiresNew 挂起当前事务，另开一个。
	PropagationRequiresNew Propagation = "requires_new"
	// PropagationNotSupported 以 tx == nil 执行，并挂起当前事务。
	PropagationNotSupported Propagation = "not_supported"
	// PropagationMandatory 加入当前事务。没有事务时返回错误。
	PropagationMandatory Propagation = "mandatory"
	// PropagationNever 以 tx == nil 执行。已有事务时返回错误。
	PropagationNever Propagation = "never"
)

// TxOptions 控制隔离级别、只读和传播行为。
type TxOptions struct {
	Propagation Propagation
	Isolation   sql.IsolationLevel
	ReadOnly    bool
}

type txCtxKey struct {
	group string
}

// Begin 在主库连接上开启事务。
func (d *DB) Begin(ctx context.Context) (*Tx, error) {
	return d.BeginTx(ctx, TxOptions{})
}

// BeginTx 在主库连接上按 opts 开启事务。
// Isolation 和 ReadOnly 传给 database/sql。Propagation 由 Transaction 使用，BeginTx 不看它。
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

// Transaction 在事务里执行 fn。fn 返回 nil 时提交，返回错误或 panic 时回滚。
// 默认传播行为是 nested。fn 收到的 context 携带这个事务，同一组上的后续查询会加入它。
func (d *DB) Transaction(ctx context.Context, fn func(ctx context.Context, tx *Tx) error) error {
	return d.TransactionWithOptions(ctx, TxOptions{}, fn)
}

// TransactionWithOptions 按给定的传播行为和事务选项执行 fn。
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
			rbErr := rollbackFully(tx)
			err = fmt.Errorf("gooq: transaction panic: %v", recovered)
			if rbErr != nil {
				err = fmt.Errorf("%w; rollback: %v", err, rbErr)
			}
		}
	}()
	err = fn(ctx, tx)
	return finishTx(tx, err)
}

// Transaction 在当前事务里开启保存点。fn 返回 nil 时释放保存点，返回错误时回滚到该保存点。
// panic 时先回滚到该保存点并恢复层级，再把 panic 抛给外层。
func (t *Tx) Transaction(ctx context.Context, fn func(ctx context.Context, tx *Tx) error) (err error) {
	if t == nil {
		return fmt.Errorf("gooq: transaction is nil")
	}
	if ctx == nil {
		ctx = t.ctx
	}
	if err = t.beginNested(ctx); err != nil {
		return err
	}
	ctx = WithTx(ctx, t)
	defer func() {
		if recovered := recover(); recovered != nil {
			rbErr := t.Rollback()
			if rbErr != nil {
				panic(fmt.Errorf("gooq: transaction panic: %v; rollback: %v", recovered, rbErr))
			}
			panic(recovered)
		}
	}()
	err = fn(ctx, t)
	return finishTx(t, err)
}

// Commit 提交事务。处于嵌套保存点时释放当前保存点，不结束外层事务。
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

// Rollback 回滚事务。处于嵌套保存点时回滚到当前保存点，不结束外层事务。
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

// SavePoint 在事务内建立具名保存点。名字按方言加引号。
func (t *Tx) SavePoint(ctx context.Context, name string) error {
	quoted, err := quoteIdent(t.dialect, name)
	if err != nil {
		return err
	}
	_, err = t.sql.ExecContext(ctx, "SAVEPOINT "+quoted)
	return err
}

// RollbackTo 回滚到具名保存点。名字按方言加引号。
func (t *Tx) RollbackTo(ctx context.Context, name string) error {
	quoted, err := quoteIdent(t.dialect, name)
	if err != nil {
		return err
	}
	_, err = t.sql.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+quoted)
	return err
}

// IsClosed 报告外层事务是否已由 Commit 或 Rollback 结束。嵌套保存点不会把它标成已关闭。接收者为 nil 时返回 true。
func (t *Tx) IsClosed() bool {
	return t == nil || t.closed
}

// DB 返回开启这个事务的数据库。接收者为 nil 时返回 nil。
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

// rollbackFully 退掉尚未释放的保存点，再回滚外层事务。
func rollbackFully(tx *Tx) error {
	if tx == nil {
		return nil
	}
	var first error
	for tx.depth > 0 {
		if err := tx.Rollback(); err != nil && first == nil {
			first = err
		}
	}
	if err := tx.Rollback(); err != nil && first == nil {
		return err
	}
	return first
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

// WithTx 按数据库组把 tx 放进 ctx。tx 或它的数据库为空时原样返回 ctx。
func WithTx(ctx context.Context, tx *Tx) context.Context {
	if tx == nil || tx.db == nil {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, txCtxKey{group: tx.db.group}, tx)
}

// WithoutTx 从 ctx 里清掉该组的事务。
func WithoutTx(ctx context.Context, group string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, txCtxKey{group: group}, (*Tx)(nil))
}

// TxFromContext 返回该组上仍打开的事务。没有、已关闭或底层事务为空时返回 nil。
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
