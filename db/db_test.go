package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestParseLink(t *testing.T) {
	parsed, err := parseLink("mysql:root:pass@tcp(127.0.0.1:3306)/app?charset=utf8")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Type != "mysql" || parsed.User != "root" || parsed.Name != "app" || parsed.Port != "3306" {
		t.Fatalf("parsed mysql link: %+v", parsed)
	}
	parsed, err = parseLink("sqlite::@file(:memory:)")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Protocol != "file" || parsed.Name != ":memory:" {
		t.Fatalf("parsed sqlite link: %+v", parsed)
	}
}

func TestSQLiteScanAndSchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "gooq.db")
	database, err := Open(Config{Link: "sqlite::@file(" + path + ")"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	_, err = database.Exec(ctx, `
CREATE TABLE user (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    active INTEGER NOT NULL,
    created_at DATETIME,
    meta TEXT
)`)
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 10, 10, 8, 30, 0, 0, time.Local)
	_, err = database.Exec(ctx, `INSERT INTO user(name, active, created_at, meta) VALUES (?, ?, ?, ?)`,
		"john", 1, created.Format("2006-01-02 15:04:05"), `{"vip":true}`)
	if err != nil {
		t.Fatal(err)
	}

	var user struct {
		Id        int64
		Name      string
		Active    bool
		CreatedAt time.Time
		Meta      string
	}
	if err = database.GetScan(ctx, &user, `SELECT id, name, active, created_at, meta FROM user`); err != nil {
		t.Fatal(err)
	}
	if user.Id != 1 || user.Name != "john" || !user.Active || user.Meta != `{"vip":true}` {
		t.Fatalf("scanned user: %+v", user)
	}
	if user.CreatedAt.Format("2006-01-02 15:04:05") != created.Format("2006-01-02 15:04:05") {
		t.Fatalf("created_at %s, want %s", user.CreatedAt, created)
	}

	columns, err := database.TableFields(ctx, "user")
	if err != nil {
		t.Fatal(err)
	}
	if len(columns) != 5 || !columns[0].Primary || !columns[0].AutoIncrement || columns[0].LocalType != "int" {
		t.Fatalf("columns: %+v", columns)
	}
	if columns[3].LocalType != "datetime" || columns[4].LocalType != "json" && columns[4].LocalType != "string" {
		t.Fatalf("time/json columns: %+v", columns[3:])
	}
}
