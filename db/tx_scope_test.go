package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func openFileDB(t *testing.T, path string) *DB {
	t.Helper()
	database, err := Open(Config{Link: "sqlite::@file(" + filepath.ToSlash(path) + ")"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	return database
}

func createEvents(t *testing.T, database *DB) {
	t.Helper()
	if _, err := database.Exec(context.Background(), `CREATE TABLE events (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
}

func countEvents(t *testing.T, database *DB) int64 {
	t.Helper()
	var n int64
	if err := database.Value(context.Background(), &n, `SELECT COUNT(*) FROM events`); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestOpenTransactionsStayOnTheirPool(t *testing.T) {
	dir := t.TempDir()
	a := openFileDB(t, filepath.Join(dir, "a.db"))
	b := openFileDB(t, filepath.Join(dir, "b.db"))
	createEvents(t, a)
	createEvents(t, b)
	ctx := context.Background()

	err := a.Transaction(ctx, func(txctx context.Context, tx *Tx) error {
		if _, err := b.Exec(txctx, `INSERT INTO events(id) VALUES (1)`); err != nil {
			return err
		}
		return errors.New("rollback a")
	})
	if err == nil || err.Error() != "rollback a" {
		t.Fatal(err)
	}
	if countEvents(t, a) != 0 || countEvents(t, b) != 1 {
		t.Fatalf("a=%d b=%d", countEvents(t, a), countEvents(t, b))
	}

	err = a.Transaction(ctx, func(txctx context.Context, tx *Tx) error {
		joined := WithTx(context.Background(), tx)
		if _, err := a.Exec(joined, `INSERT INTO events(id) VALUES (2)`); err != nil {
			return err
		}
		if _, err := b.Exec(joined, `INSERT INTO events(id) VALUES (3)`); err != nil {
			return err
		}
		return errors.New("rollback a")
	})
	if err == nil {
		t.Fatal("expected rollback")
	}
	if countEvents(t, a) != 0 || countEvents(t, b) != 2 {
		t.Fatalf("after WithTx a=%d b=%d", countEvents(t, a), countEvents(t, b))
	}
}

func TestNestedSavepointKeepsOuterTx(t *testing.T) {
	database := openFileDB(t, filepath.Join(t.TempDir(), "nested.db"))
	createEvents(t, database)
	err := database.Transaction(context.Background(), func(ctx context.Context, tx *Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO events(id) VALUES (1)`); err != nil {
			return err
		}
		err := tx.Transaction(ctx, func(inner context.Context, innerTx *Tx) error {
			if database.TxFromContext(inner) != tx || innerTx != tx {
				return errors.New("nested ctx lost the outer transaction")
			}
			if _, err := tx.Exec(inner, `INSERT INTO events(id) VALUES (2)`); err != nil {
				return err
			}
			return errors.New("rollback savepoint")
		})
		if err == nil {
			return errors.New("savepoint rollback was swallowed")
		}
		if database.TxFromContext(ctx) != tx || tx.IsClosed() {
			return errors.New("outer transaction was closed")
		}
		_, err = tx.Exec(ctx, `INSERT INTO events(id) VALUES (3)`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if countEvents(t, database) != 2 {
		t.Fatalf("count %d", countEvents(t, database))
	}
}

func TestRequiresNewRestoresOuterTx(t *testing.T) {
	database := openFileDB(t, filepath.Join(t.TempDir(), "new.db"))
	createEvents(t, database)
	err := database.Transaction(context.Background(), func(ctx context.Context, tx *Tx) error {
		err := database.TransactionWithOptions(ctx, TxOptions{Propagation: PropagationRequiresNew}, func(inner context.Context, tx2 *Tx) error {
			if tx2 == nil || tx2 == tx || database.TxFromContext(inner) != tx2 {
				return errors.New("inner context does not carry the new transaction")
			}
			if _, err := database.Exec(inner, `INSERT INTO events(id) VALUES (9)`); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return err
		}
		if database.TxFromContext(ctx) != tx || tx.IsClosed() {
			return errors.New("outer transaction was not restored")
		}
		return errors.New("rollback outer")
	})
	if err == nil || err.Error() != "rollback outer" {
		t.Fatal(err)
	}
	if countEvents(t, database) != 1 {
		t.Fatalf("inner commit count %d", countEvents(t, database))
	}
}

func TestNotSupportedUsesOwnConnection(t *testing.T) {
	dir := t.TempDir()
	a := openFileDB(t, filepath.Join(dir, "a.db"))
	b := openFileDB(t, filepath.Join(dir, "b.db"))
	createEvents(t, a)
	createEvents(t, b)
	err := b.Transaction(context.Background(), func(bctx context.Context, btx *Tx) error {
		return a.Transaction(bctx, func(actx context.Context, atx *Tx) error {
			err := a.TransactionWithOptions(actx, TxOptions{Propagation: PropagationNotSupported}, func(inner context.Context, tx *Tx) error {
				if tx != nil || a.TxFromContext(inner) != nil {
					return errors.New("not supported still saw this pool's transaction")
				}
				if b.TxFromContext(inner) != btx {
					return errors.New("other pool's transaction was removed")
				}
				_, err := a.Exec(inner, `INSERT INTO events(id) VALUES (7)`)
				return err
			})
			if err != nil {
				return err
			}
			if a.TxFromContext(actx) != atx {
				return errors.New("outer transaction was not restored")
			}
			return errors.New("rollback a")
		})
	})
	if err == nil || err.Error() != "rollback a" {
		t.Fatal(err)
	}
	if countEvents(t, a) != 1 || countEvents(t, b) != 0 {
		t.Fatalf("a=%d b=%d", countEvents(t, a), countEvents(t, b))
	}
}

func TestSQLiteSchemaAndMasterJoinTransaction(t *testing.T) {
	database := openFileDB(t, filepath.Join(t.TempDir(), "schema.db"))
	createEvents(t, database)
	view, err := database.Schema("main")
	if err != nil {
		t.Fatal(err)
	}
	err = database.Transaction(context.Background(), func(ctx context.Context, tx *Tx) error {
		if _, err := view.Exec(ctx, `INSERT INTO events(id) VALUES (1)`); err != nil {
			return err
		}
		if _, err := view.Master().Exec(ctx, `INSERT INTO events(id) VALUES (2)`); err != nil {
			return err
		}
		return errors.New("rollback")
	})
	if err == nil {
		t.Fatal("expected rollback")
	}
	if countEvents(t, database) != 0 {
		t.Fatalf("shared schema wrote outside the transaction, count %d", countEvents(t, database))
	}
}

func TestRegisteredGroupsDoNotShareTransactions(t *testing.T) {
	dir := t.TempDir()
	ordersLink := "sqlite::@file(" + filepath.ToSlash(filepath.Join(dir, "orders.db")) + ")"
	reportLink := "sqlite::@file(" + filepath.ToSlash(filepath.Join(dir, "report.db")) + ")"
	if err := SetConfig("orders", Config{Link: ordersLink}); err != nil {
		t.Fatal(err)
	}
	if err := SetConfig("report", Config{Link: reportLink}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = RemoveConfig("orders")
		_ = RemoveConfig("report")
	})
	orders, err := Instance("orders")
	if err != nil {
		t.Fatal(err)
	}
	report, err := Instance("report")
	if err != nil {
		t.Fatal(err)
	}
	createEvents(t, orders)
	createEvents(t, report)
	err = orders.Transaction(context.Background(), func(ctx context.Context, tx *Tx) error {
		if _, err := report.Exec(ctx, `INSERT INTO events(id) VALUES (1)`); err != nil {
			return err
		}
		return errors.New("rollback orders")
	})
	if err == nil || err.Error() != "rollback orders" {
		t.Fatal(err)
	}
	if countEvents(t, orders) != 0 || countEvents(t, report) != 1 {
		t.Fatalf("orders=%d report=%d", countEvents(t, orders), countEvents(t, report))
	}
}

func TestConcurrentDefaultGroupTransactionsAreDistinct(t *testing.T) {
	path := filepath.Join(t.TempDir(), "default.db")
	link := "sqlite::@file(" + filepath.ToSlash(path) + ")"
	if err := SetConfig(DefaultGroupName, Config{Link: link}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RemoveConfig(DefaultGroupName) })
	database, err := Instance()
	if err != nil {
		t.Fatal(err)
	}
	createEvents(t, database)

	type result struct {
		tx  *Tx
		err error
	}
	started := make(chan result, 2)
	releaseSecond := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondDone := make(chan struct{})
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		err := database.Transaction(context.Background(), func(ctx context.Context, tx *Tx) error {
			if database.TxFromContext(ctx) != tx {
				return errors.New("first ctx lost its transaction")
			}
			started <- result{tx: tx}
			<-releaseFirst
			if _, err := tx.Exec(ctx, `INSERT INTO events(id) VALUES (1)`); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			started <- result{err: err}
		}
	}()
	go func() {
		defer close(secondDone)
		err := database.Transaction(context.Background(), func(ctx context.Context, tx *Tx) error {
			if database.TxFromContext(ctx) != tx {
				return errors.New("second ctx lost its transaction")
			}
			started <- result{tx: tx}
			<-releaseSecond
			return nil
		})
		if err != nil {
			started <- result{err: err}
		}
	}()

	first := <-started
	second := <-started
	if first.err != nil || second.err != nil {
		close(releaseSecond)
		close(releaseFirst)
		<-secondDone
		<-firstDone
		t.Fatalf("first %v second %v", first.err, second.err)
	}
	close(releaseSecond)
	<-secondDone
	close(releaseFirst)
	<-firstDone
	if first.tx == nil || second.tx == nil || first.tx == second.tx {
		t.Fatalf("transactions were not distinct: %p %p", first.tx, second.tx)
	}
	if countEvents(t, database) != 1 {
		t.Fatalf("count %d", countEvents(t, database))
	}

	err = database.Transaction(context.Background(), func(ctx context.Context, tx *Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO events(id) VALUES (2)`); err != nil {
			return err
		}
		return errors.New("rollback")
	})
	if err == nil {
		t.Fatal("expected rollback")
	}
	if countEvents(t, database) != 1 {
		t.Fatalf("rollback changed other rows, count %d", countEvents(t, database))
	}
}
