package dsl

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
	_ "github.com/lanceadd/gooq/drivers/sqlite"
)

func TestValueColumnAndScanBoundary(t *testing.T) {
	ctx := context.Background()
	var when time.Time
	var user struct{ Name string }
	if err := db.CheckValue(&user); err == nil {
		t.Fatal("Value accepted a row struct")
	}
	if err := db.CheckValue(&when); err != nil {
		t.Fatal(err)
	}
	var times []time.Time
	if err := db.CheckColumn(&times); err != nil {
		t.Fatal(err)
	}

	group := "cell"
	path := filepath.Join(t.TempDir(), "cell.db")
	err := db.SetConfig(group, db.Config{Link: "sqlite::@file(" + filepath.ToSlash(path) + ")"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.RemoveConfig(group); err != nil {
			t.Error(err)
		}
	})
	database, err := db.Instance(group)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(ctx, `
CREATE TABLE user (
    id INTEGER PRIMARY KEY,
    name TEXT,
    age INTEGER,
    status TEXT,
    created_at TEXT,
    deleted_at TEXT
)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(ctx, `INSERT INTO user(id, name, age, created_at) VALUES (1, 'john', 20, '2026-10-10 08:30:00'), (2, 'amy', 21, NULL)`)
	if err != nil {
		t.Fatal(err)
	}

	if err = Select(testUser.CreatedAt).From(testUser).Where(testUser.Id.Eq(1)).Unscoped().DB(group).Value(ctx, &when); err != nil {
		t.Fatal(err)
	}
	if when.Format("2006-01-02 15:04:05") != "2026-10-10 08:30:00" {
		t.Fatalf("value %s", when)
	}

	var names []string
	if err = Select(testUser.Name).From(testUser).Unscoped().Order(testUser.Id.Asc()).DB(group).Column(ctx, &names); err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "john" || names[1] != "amy" {
		t.Fatalf("names %v", names)
	}

	row, err := Select(testUser.CreatedAt, testUser.Name).From(testUser).Where(testUser.Id.Eq(1)).Unscoped().DB(group).Row(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.Value(row, testUser.CreatedAt)
	if err != nil || got.Format("2006-01-02 15:04:05") != "2026-10-10 08:30:00" {
		t.Fatalf("record value %s err=%v", got, err)
	}
	if db.Cell(row, "name").String() != "john" {
		t.Fatal("cell name")
	}

	err = Select(testUser.CreatedAt).From(testUser).Where(testUser.Id.Eq(9)).Unscoped().DB(group).Value(ctx, &when)
	if err != sql.ErrNoRows {
		t.Fatalf("missing value err=%v", err)
	}

	var returned time.Time
	err = Insert(testUser).Columns(testUser.Id, testUser.Name, testUser.CreatedAt).
		Values(3, "new", "2026-10-11 09:00:00").
		Returning(testUser.CreatedAt).DB(group).Value(ctx, &returned)
	if err != nil {
		t.Fatal(err)
	}
	if returned.Format("2006-01-02 15:04:05") != "2026-10-11 09:00:00" {
		t.Fatalf("returning %s", returned)
	}

	var before int64
	if before, err = Select(testUser.Id).From(testUser).Unscoped().DB(group).Count(ctx); err != nil {
		t.Fatal(err)
	}
	var name string
	err = Insert(testUser).Columns(testUser.Id, testUser.Name).
		Values(30, "nope").
		Returning(testUser.Id, testUser.Name).DB(group).Value(ctx, &name)
	if err == nil || !strings.Contains(err.Error(), "exactly one column") {
		t.Fatalf("returning value err=%v", err)
	}
	var after int64
	if after, err = Select(testUser.Id).From(testUser).Unscoped().DB(group).Count(ctx); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("rejected returning inserted a row: %d -> %d", before, after)
	}

	err = Insert(testUser).Columns(testUser.Id, testUser.Name).
		Values(31, "raw").
		Returning(gooq.Raw("id, name")).DB(group).Value(ctx, &name)
	if err == nil || !strings.Contains(err.Error(), "expected 1") {
		t.Fatalf("raw returning err=%v", err)
	}

	names = nil
	err = Insert(testUser).Columns(testUser.Id, testUser.Name).
		Values(32, "first").
		Values(33, "second").
		Returning(testUser.Name).DB(group).Value(ctx, &name)
	if err != nil || name != "first" {
		t.Fatalf("first returning %q err=%v", name, err)
	}
	err = Insert(testUser).Columns(testUser.Id, testUser.Name).
		Values(34, "a").
		Values(35, "b").
		Returning(testUser.Name).DB(group).Column(ctx, &names)
	if err != nil || len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Fatalf("returning column %v err=%v", names, err)
	}
}
