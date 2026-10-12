package db

import (
	"context"
	"errors"
	"path/filepath"
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
}
