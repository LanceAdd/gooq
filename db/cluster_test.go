package db

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

func TestMasterSlaveRouting(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	masterPath := filepath.Join(dir, "master.db")
	slavePath := filepath.Join(dir, "slave.db")
	master, err := Open(Config{Link: "sqlite::@file(" + masterPath + ")"})
	if err != nil {
		t.Fatal(err)
	}
	slave, err := Open(Config{Link: "sqlite::@file(" + slavePath + ")"})
	if err != nil {
		t.Fatal(err)
	}
	for _, database := range []*DB{master, slave} {
		_, err = database.Exec(ctx, `CREATE TABLE marker (src TEXT)`)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = master.Exec(ctx, `INSERT INTO marker(src) VALUES ('master')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = slave.Exec(ctx, `INSERT INTO marker(src) VALUES ('slave')`)
	if err != nil {
		t.Fatal(err)
	}
	if err = master.Close(); err != nil {
		t.Fatal(err)
	}
	if err = slave.Close(); err != nil {
		t.Fatal(err)
	}

	const group = "rw-test"
	err = SetConfig(group,
		Config{Link: "sqlite::@file(" + masterPath + ")", Role: RoleMaster},
		Config{Link: "sqlite::@file(" + slavePath + ")", Role: RoleSlave},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := RemoveConfig(group); err != nil {
			t.Error(err)
		}
	})
	database, err := Instance(group)
	if err != nil {
		t.Fatal(err)
	}
	var src string
	if err = database.Value(ctx, &src, `SELECT src FROM marker`); err != nil {
		t.Fatal(err)
	}
	if src != "slave" {
		t.Fatalf("read src %q, want slave", src)
	}
	if err = database.Master().Value(ctx, &src, `SELECT src FROM marker`); err != nil {
		t.Fatal(err)
	}
	if src != "master" {
		t.Fatalf("master src %q, want master", src)
	}
	if _, err = database.Exec(ctx, `INSERT INTO marker(src) VALUES ('written')`); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err = database.Master().Value(ctx, &count, `SELECT COUNT(*) FROM marker WHERE src = 'written'`); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("master written rows %d", count)
	}
	if err = database.Value(ctx, &count, `SELECT COUNT(*) FROM marker WHERE src = 'written'`); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("slave written rows %d, want 0", count)
	}
}

func TestTransactionPropagation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "tx.db")
	const group = "tx-test"
	err := SetConfig(group, Config{Link: "sqlite::@file(" + path + ")"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := RemoveConfig(group); err != nil {
			t.Error(err)
		}
	})
	database, err := Instance(group)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(ctx, `CREATE TABLE item (id INTEGER PRIMARY KEY, name TEXT)`)
	if err != nil {
		t.Fatal(err)
	}

	err = database.Transaction(ctx, func(ctx context.Context, tx *Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO item(id, name) VALUES (1, 'keep')`); err != nil {
			return err
		}
		nestedErr := tx.Transaction(ctx, func(ctx context.Context, tx *Tx) error {
			if _, err := tx.Exec(ctx, `INSERT INTO item(id, name) VALUES (2, 'drop')`); err != nil {
				return err
			}
			return errors.New("rollback nested")
		})
		if nestedErr == nil || nestedErr.Error() != "rollback nested" {
			return errors.New("nested error was not returned")
		}
		if err := tx.SavePoint(ctx, "mark"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO item(id, name) VALUES (3, 'marked')`); err != nil {
			return err
		}
		return tx.RollbackTo(ctx, "mark")
	})
	if err != nil {
		t.Fatal(err)
	}
	var count int64
	if err = database.Value(ctx, &count, `SELECT COUNT(*) FROM item`); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("committed rows %d, want 1", count)
	}

	err = database.Transaction(ctx, func(ctx context.Context, tx *Tx) error {
		return database.TransactionWithOptions(ctx, TxOptions{Propagation: PropagationNever}, func(context.Context, *Tx) error {
			return nil
		})
	})
	if err == nil {
		t.Fatal("expected never propagation to fail inside a transaction")
	}

	err = database.Transaction(ctx, func(ctx context.Context, tx *Tx) error {
		return database.TransactionWithOptions(ctx, TxOptions{Propagation: PropagationRequiresNew}, func(inner context.Context, tx2 *Tx) error {
			if tx2 == nil || tx2 == tx {
				return errors.New("requires_new did not start a different transaction")
			}
			if TxFromContext(inner, group) == tx {
				return errors.New("inner context still carries the outer transaction")
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	err = database.Transaction(ctx, func(ctx context.Context, tx *Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO item(id, name) VALUES (4, 'outer')`); err != nil {
			return err
		}
		defer func() { _ = recover() }()
		_ = tx.Transaction(ctx, func(ctx context.Context, tx *Tx) error {
			if _, err := tx.Exec(ctx, `INSERT INTO item(id, name) VALUES (5, 'boom')`); err != nil {
				return err
			}
			panic("boom")
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = database.Value(ctx, &count, `SELECT COUNT(*) FROM item WHERE id = 4`); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("outer row after recovered nested panic = %d", count)
	}
	if err = database.Value(ctx, &count, `SELECT COUNT(*) FROM item WHERE id = 5`); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("nested panic row survived = %d", count)
	}

	err = database.Transaction(ctx, func(ctx context.Context, tx *Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO item(id, name) VALUES (6, 'lost')`); err != nil {
			return err
		}
		return tx.Transaction(ctx, func(context.Context, *Tx) error {
			panic("lost")
		})
	})
	if err == nil {
		t.Fatal("expected nested panic to fail the outer transaction")
	}
	if err = database.Value(ctx, &count, `SELECT COUNT(*) FROM item WHERE id = 6`); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("unrecovered nested panic committed rows = %d", count)
	}
	if _, err = database.Exec(ctx, `INSERT INTO item(id, name) VALUES (7, 'after')`); err != nil {
		t.Fatal(err)
	}

	err = database.Transaction(ctx, func(ctx context.Context, tx *Tx) error {
		if _, err := database.Exec(ctx, `INSERT INTO item(id, name) VALUES (8, 'ctx')`); err != nil {
			return err
		}
		var seen int64
		if err := database.Value(ctx, &seen, `SELECT COUNT(*) FROM item WHERE id = 8`); err != nil {
			return err
		}
		if seen != 1 {
			return errors.New("ctx query did not see the transaction write")
		}
		return errors.New("rollback ctx write")
	})
	if err == nil {
		t.Fatal("expected ctx write to roll back")
	}
	if err = database.Value(ctx, &count, `SELECT COUNT(*) FROM item WHERE id = 8`); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("ctx exec survived rollback, count = %d", count)
	}
}

func TestGetValueUsesFirstDuplicateColumn(t *testing.T) {
	ctx := context.Background()
	database, err := Open(Config{Link: "sqlite::@file(" + filepath.Join(t.TempDir(), "cols.db") + ")"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	var value int64
	if err = database.Value(ctx, &value, `SELECT 1 AS a, 2 AS a`); err != nil {
		t.Fatal(err)
	}
	if value != 1 {
		t.Fatalf("first column = %d", value)
	}
	record, err := database.GetOne(ctx, `SELECT 1 AS a, 2 AS a`)
	if err != nil {
		t.Fatal(err)
	}
	if record["a"] != int64(2) {
		t.Fatalf("record duplicate = %#v", record["a"])
	}
	if err = database.ValueExact(ctx, &value, `SELECT 1 AS a, 2 AS a`); err == nil {
		t.Fatal("expected exact value to reject two columns")
	}
	var values []int64
	if err = database.ColumnExact(ctx, &values, `SELECT 1 AS a, 2 AS a`); err == nil {
		t.Fatal("expected exact column to reject two columns")
	}
	if err = database.ValueExact(ctx, &value, `SELECT 1 WHERE 0`); err != sql.ErrNoRows {
		t.Fatalf("missing exact value err=%v", err)
	}
	if err = database.ValueExact(ctx, &value, `SELECT 1 AS a, 2 AS b WHERE 0`); err == nil {
		t.Fatal("expected exact value to reject two columns when no row is returned")
	}
}

func TestInstanceRemoveDoesNotRepublish(t *testing.T) {
	path := filepath.Join(t.TempDir(), "race.db")
	const group = "race-config"
	link := "sqlite::@file(" + path + ")"
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_, _ = Instance(group)
				}
			}
		}()
	}
	for i := 0; i < 40; i++ {
		if err := SetConfig(group, Config{Link: link}); err != nil {
			close(stop)
			wg.Wait()
			t.Fatal(err)
		}
		if err := RemoveConfig(group); err != nil {
			close(stop)
			wg.Wait()
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
	if database := groupInstances.Get(group); database != nil {
		_ = database.Close()
		t.Fatal("instance remained cached after RemoveConfig")
	}
}
