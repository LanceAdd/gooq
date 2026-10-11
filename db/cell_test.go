package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gtime"
)

func TestValueAndColumn(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cell.db")
	database, err := Open(Config{Link: "sqlite::@file(" + filepath.ToSlash(path) + ")"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	_, err = database.Exec(ctx, `
CREATE TABLE item (
    id INTEGER PRIMARY KEY,
    name TEXT,
    created_at TEXT
)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(ctx, `INSERT INTO item(id, name, created_at) VALUES (1, 'john', '2026-10-10 08:30:00'), (2, NULL, NULL)`)
	if err != nil {
		t.Fatal(err)
	}

	var when time.Time
	if err = database.Value(ctx, &when, `SELECT created_at FROM item WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if when.Format("2006-01-02 15:04:05") != "2026-10-10 08:30:00" {
		t.Fatalf("created_at %s", when)
	}

	var gt gtime.Time
	if err = database.Value(ctx, &gt, `SELECT created_at FROM item WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if gt.Format("Y-m-d H:i:s") != "2026-10-10 08:30:00" {
		t.Fatalf("gtime %s", gt.String())
	}

	var ns sql.NullString
	if err = database.Value(ctx, &ns, `SELECT name FROM item WHERE id = 2`); err != nil || ns.Valid {
		t.Fatalf("null name %+v err=%v", ns, err)
	}
	var nt sql.NullTime
	if err = database.Value(ctx, &nt, `SELECT created_at FROM item WHERE id = 1`); err != nil || !nt.Valid {
		t.Fatalf("null time %+v err=%v", nt, err)
	}

	when = time.Now()
	if err = database.Value(ctx, &when, `SELECT created_at FROM item WHERE id = 2`); err != nil || !when.IsZero() {
		t.Fatalf("NULL time err=%v value=%v", err, when)
	}
	err = database.Value(ctx, &when, `SELECT created_at FROM item WHERE id = 9`)
	if err != sql.ErrNoRows {
		t.Fatalf("missing row err=%v", err)
	}

	var names []sql.NullString
	if err = database.Column(ctx, &names, `SELECT name FROM item ORDER BY id`); err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0].String != "john" || !names[0].Valid || names[1].Valid {
		t.Fatalf("names %+v", names)
	}

	var times []time.Time
	if err = database.Column(ctx, &times, `SELECT created_at FROM item ORDER BY id`); err != nil {
		t.Fatal(err)
	}
	if len(times) != 2 || times[0].IsZero() || !times[1].IsZero() {
		t.Fatalf("times %v", times)
	}

	err = database.GetScan(ctx, &when, `SELECT created_at FROM item WHERE id = 1`)
	if err == nil || !strings.Contains(err.Error(), "use Value") {
		t.Fatalf("GetScan time err=%v", err)
	}

	var id int
	if err = database.GetScan(ctx, &id, `SELECT id FROM item WHERE id = 1`); err != nil || id != 1 {
		t.Fatalf("scan int %d err=%v", id, err)
	}

	err = database.Transaction(ctx, func(ctx context.Context, tx *Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO item(id, name) VALUES (3, 'tx')`); err != nil {
			return err
		}
		var got string
		if err := tx.Value(ctx, &got, `SELECT name FROM item WHERE id = 3`); err != nil {
			return err
		}
		if got != "tx" {
			t.Fatalf("tx value %q", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
