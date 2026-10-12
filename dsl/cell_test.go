package dsl

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

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
}
