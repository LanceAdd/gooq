package db

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenUnregisteredDriver(t *testing.T) {
	_, err := Open(Config{Link: "oracle:u:p@tcp(127.0.0.1:1521)/db"})
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("expected unregistered driver error, got %v", err)
	}
}

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
	parsed, err = parseLink("pgsql:gooq:gooq@tcp([::1]:5432)/gooq")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Host != "::1" || parsed.Port != "5432" || parsed.Name != "gooq" {
		t.Fatalf("parsed ipv6 link: %+v", parsed)
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

func TestSQLiteAutoIncrement(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "autoinc.db")
	database, err := Open(Config{Link: "sqlite::@file(" + filepath.ToSlash(path) + ")"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	tables := []struct {
		name string
		ddl  string
		want map[string]bool
	}{
		{
			name: "integer_pk",
			ddl:  `CREATE TABLE integer_pk (id INTEGER PRIMARY KEY, name TEXT)`,
			want: map[string]bool{"id": true, "name": false},
		},
		{
			name: "integer_pk_lower",
			ddl:  `CREATE TABLE integer_pk_lower (id integer primary key, name TEXT)`,
			want: map[string]bool{"id": true, "name": false},
		},
		{
			name: "composite_pk",
			ddl:  `CREATE TABLE composite_pk (a INTEGER, b INTEGER, PRIMARY KEY(a, b))`,
			want: map[string]bool{"a": false, "b": false},
		},
		{
			name: "int_pk",
			ddl:  `CREATE TABLE int_pk (id INT PRIMARY KEY)`,
			want: map[string]bool{"id": false},
		},
		{
			name: "without_rowid",
			ddl:  `CREATE TABLE without_rowid (id INTEGER PRIMARY KEY, name TEXT) WITHOUT ROWID`,
			want: map[string]bool{"id": false, "name": false},
		},
	}
	for _, table := range tables {
		if _, err = database.Exec(ctx, table.ddl); err != nil {
			t.Fatal(err)
		}
		columns, err := database.TableFields(ctx, table.name)
		if err != nil {
			t.Fatal(err)
		}
		if len(columns) != len(table.want) {
			t.Fatalf("%s columns: %+v", table.name, columns)
		}
		for _, column := range columns {
			want, ok := table.want[column.Name]
			if !ok || column.AutoIncrement != want {
				t.Fatalf("%s column %s auto=%v want=%v ok=%v", table.name, column.Name, column.AutoIncrement, want, ok)
			}
		}
	}
}

func TestRouteViewCloseKeepsPool(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "route.db")
	database, err := Open(Config{Link: "sqlite::@file(" + filepath.ToSlash(path) + ")"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(ctx, `CREATE TABLE item (id INTEGER PRIMARY KEY)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(ctx, `INSERT INTO item(id) VALUES (1)`)
	if err != nil {
		t.Fatal(err)
	}

	master := database.Master()
	slave := database.Slave()
	if master == database || slave == database {
		t.Fatal("route view reused the owner")
	}
	if err = master.Close(); err != nil {
		t.Fatal(err)
	}
	if err = slave.Close(); err != nil {
		t.Fatal(err)
	}
	var id int
	if err = database.GetScan(ctx, &id, `SELECT id FROM item`); err != nil || id != 1 {
		t.Fatalf("owner after view close id=%d err=%v", id, err)
	}
	if err = database.Close(); err != nil {
		t.Fatal(err)
	}
	err = database.GetScan(ctx, &id, `SELECT id FROM item`)
	if err == nil || !strings.Contains(err.Error(), "database is closed") {
		t.Fatalf("owner close err=%v", err)
	}
}
