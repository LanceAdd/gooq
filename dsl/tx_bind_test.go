package dsl

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"

	_ "github.com/lanceadd/gooq/drivers/sqlite"
)

func TestExplicitTxBindsOpenDatabase(t *testing.T) {
	dir := t.TempDir()
	defaultLink := "sqlite::@file(" + filepath.ToSlash(filepath.Join(dir, "default.db")) + ")"
	openLink := "sqlite::@file(" + filepath.ToSlash(filepath.Join(dir, "open.db")) + ")"
	if err := db.SetConfig(db.DefaultGroupName, db.Config{Link: defaultLink}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.RemoveConfig(db.DefaultGroupName) })
	registered, err := db.Instance()
	if err != nil {
		t.Fatal(err)
	}
	opened, err := db.Open(db.Config{Link: openLink})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = opened.Close() })
	ctx := context.Background()
	for _, database := range []*db.DB{registered, opened} {
		if _, err = database.Exec(ctx, `CREATE TABLE item (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
			t.Fatal(err)
		}
	}
	item := gooq.NewTableBase(&gooq.TableMeta{
		TableName: "item",
		Fields: []gooq.FieldMeta{
			{ColumnName: "id", LocalType: gooq.LocalTypeInt64, Primary: true},
			{ColumnName: "name", LocalType: gooq.LocalTypeString},
		},
	})
	id := gooq.NewFieldAt[int64](item, "id")
	name := gooq.NewFieldAt[string](item, "name")

	err = opened.Transaction(ctx, func(txctx context.Context, tx *db.Tx) error {
		if _, err = Insert(item).Columns(id, name).Values(int64(1), "from-ctx").Exec(txctx); err != nil {
			return err
		}
		if _, err = Insert(item).Columns(id, name).Values(int64(1), "from-tx").Tx(tx).Exec(txctx); err != nil {
			return err
		}
		return errors.New("rollback open")
	})
	if err == nil || err.Error() != "rollback open" {
		t.Fatal(err)
	}
	var registeredCount, openCount int64
	if err = registered.Value(ctx, &registeredCount, `SELECT COUNT(*) FROM item`); err != nil {
		t.Fatal(err)
	}
	if err = opened.Value(ctx, &openCount, `SELECT COUNT(*) FROM item`); err != nil {
		t.Fatal(err)
	}
	if registeredCount != 1 || openCount != 0 {
		t.Fatalf("registered=%d open=%d", registeredCount, openCount)
	}

	sqlText, _, err := Update(item).Set(name, "x").Tx(nil).ToSql(gooq.DialectSQLite)
	if err != nil || sqlText == "" {
		t.Fatalf("Tx(nil) sql %q err %v", sqlText, err)
	}
	err = opened.Transaction(ctx, func(txctx context.Context, tx *db.Tx) error {
		sqlText, _, err = Update(item).Set(name, "x").Tx(tx).DB("other").ToSql(gooq.DialectSQLite)
		if err == nil || sqlText != "" || !strings.Contains(err.Error(), "does not match DB") {
			return errors.New("ToSql accepted a mismatched group")
		}
		if _, err = Update(item).Set(name, "x").Tx(tx).DB("other").Exec(txctx); err == nil || !strings.Contains(err.Error(), "does not match DB") {
			return errors.New("Exec accepted a mismatched group")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
