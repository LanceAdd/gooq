package dsl

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
	"github.com/lanceadd/gooq/fn"

	_ "github.com/lanceadd/gooq/drivers/sqlite"
)

func TestSQLiteOffsetAndNowExecute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "offset.db")
	database, err := db.Open(db.Config{Link: "sqlite::@file(" + filepath.ToSlash(path) + ")"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()
	if _, err = database.Exec(ctx, `CREATE TABLE item (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err = database.Exec(ctx, `INSERT INTO item(id, name) VALUES (1, 'a'), (2, 'b')`); err != nil {
		t.Fatal(err)
	}
	if err = db.SetConfig("sqlite-offset", db.Config{Link: "sqlite::@file(" + filepath.ToSlash(path) + ")"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.RemoveConfig("sqlite-offset") })
	item := gooq.NewTableBase(&gooq.TableMeta{TableName: "item", Fields: []gooq.FieldMeta{{ColumnName: "id", LocalType: gooq.LocalTypeInt64, Primary: true}}})
	id := gooq.NewFieldAt[int64](item, "id")
	var got int64
	err = Select(id).From(item).OrderAsc(id).Offset(1).DB("sqlite-offset").Value(ctx, &got)
	if err != nil || got != 2 {
		t.Fatalf("offset value %d %v", got, err)
	}
	sqlText, args, err := Select(fn.Now()).From(item).DB("sqlite-offset").ToSql(gooq.DialectSQLite)
	if err != nil || !strings.Contains(sqlText, "CURRENT_TIMESTAMP") {
		t.Fatalf("now sql %s %v", sqlText, err)
	}
	value, err := database.GetValue(ctx, sqlText, args...)
	now, _ := value.(string)
	if err != nil || now == "" {
		t.Fatalf("now %#v %v", value, err)
	}
	sqlText, _, err = Select(fn.Median(id)).From(item).ToSql(gooq.DialectSQLite)
	if err == nil || sqlText != "" {
		t.Fatalf("median sql %q err %v", sqlText, err)
	}
}

func TestGroupConcatOrderArgsStayOutOfSQLiteWhere(t *testing.T) {
	sqlText, args, err := Select(fn.GroupConcat(fn.GroupConcatOptions{
		Field:   testUser.Name,
		OrderBy: []gooq.Expression{testUser.Name.Eq("a")},
	})).From(testUser).Where(testUser.Id.Eq(int64(1))).ToSql(gooq.DialectSQLite)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sqlText, "ORDER BY") {
		t.Fatalf("sqlite kept order: %s", sqlText)
	}
	if len(args) != 1 || args[0] != int64(1) {
		t.Fatalf("args %#v", args)
	}
	sqlText, args, err = Select(fn.GroupConcat(fn.GroupConcatOptions{
		Field:   testUser.Name,
		OrderBy: []gooq.Expression{testUser.Name.Eq("a")},
	})).From(testUser).Where(testUser.Id.Eq(int64(1))).ToSql(gooq.DialectMySQL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sqlText, "ORDER BY") || len(args) != 2 || args[0] != "a" || args[1] != int64(1) {
		t.Fatalf("mysql sql %s args %#v", sqlText, args)
	}
}
