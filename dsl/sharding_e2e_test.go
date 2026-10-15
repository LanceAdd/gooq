package dsl

import (
	"context"
	"strings"
	"testing"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
	_ "github.com/lanceadd/gooq/drivers/sqlite"
)

func TestShardingSQLiteCRUD(t *testing.T) {
	ctx := context.Background()
	const group = "sharding-e2e"
	if err := db.SetConfig(group, db.Config{Link: "sqlite::@file(:memory:)", MaxOpenConns: 1}); err != nil {
		t.Fatal(err)
	}
	defer db.RemoveConfig(group)
	database, err := db.Instance(group)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`CREATE TABLE user_00 (id INTEGER, name TEXT)`,
		`CREATE TABLE user_01 (id INTEGER, name TEXT)`,
	} {
		if _, err = database.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	u := newShardUserTable(&gooq.TableMeta{
		TableName: "user",
		Sharding:  shardUserRule{prefix: "user_", shards: 2},
		Fields:    shardUserFields,
	}, "")

	insert := Insert(u).Columns(u.Id, u.Name).Values(int64(0), "ada").ShardBy(int64(0)).DB(group)
	if _, err = insert.Exec(ctx); err != nil {
		t.Fatal(err)
	}
	var name string
	query := Select(u.Name).From(u).Where(u.Id.Eq(int64(0))).ShardBy(int64(0)).DB(group)
	if err = query.Value(ctx, &name); err != nil {
		t.Fatal(err)
	}
	if name != "ada" {
		t.Fatalf("name %q", name)
	}
	if got := countRows(t, database, "user_01"); got != 0 {
		t.Fatalf("user_01 rows %d", got)
	}

	update := Update(u).Set(u.Name, "ada2").Where(u.Id.Eq(int64(0))).ShardBy(int64(0)).DB(group)
	if _, err = update.Exec(ctx); err != nil {
		t.Fatal(err)
	}
	query = Select(u.Name).From(u).Where(u.Id.Eq(int64(0))).ShardBy(int64(0)).DB(group)
	if err = query.Value(ctx, &name); err != nil {
		t.Fatal(err)
	}
	if name != "ada2" {
		t.Fatalf("name %q", name)
	}

	del := Delete(u).Where(u.Id.Eq(int64(0))).ShardBy(int64(0)).DB(group)
	if _, err = del.Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, database, "user_00") + countRows(t, database, "user_01"); got != 0 {
		t.Fatalf("rows left %d", got)
	}

	before := countRows(t, database, "user_00") + countRows(t, database, "user_01")
	missing := Insert(u).Columns(u.Id, u.Name).Values(int64(0), "nope").DB(group)
	if _, err = missing.Exec(ctx); err == nil || !strings.Contains(err.Error(), "ShardBy is required") {
		t.Fatalf("err %v", err)
	}
	if got := countRows(t, database, "user_00") + countRows(t, database, "user_01"); got != before {
		t.Fatalf("rows %d", got)
	}

	fixed := u.Physical("user_01")
	physical := Insert(fixed).Columns(fixed.Id, fixed.Name).Values(int64(1), "bob").DB(group)
	if _, err = physical.Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, database, "user_01"); got != 1 {
		t.Fatalf("user_01 rows %d", got)
	}
	if got := countRows(t, database, "user_00"); got != 0 {
		t.Fatalf("user_00 rows %d", got)
	}
}

func countRows(t *testing.T, database *db.DB, table string) int64 {
	t.Helper()
	var n int64
	if err := database.Value(context.Background(), &n, "SELECT COUNT(*) FROM "+table); err != nil {
		t.Fatal(err)
	}
	return n
}
