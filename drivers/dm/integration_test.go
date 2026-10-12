//go:build dm

package dm

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
	"github.com/lanceadd/gooq/dsl"
)

func TestDMLive(t *testing.T) {
	link := os.Getenv("GOOQ_DM_LINK")
	if link == "" {
		t.Skip("GOOQ_DM_LINK is not set")
	}
	ctx := context.Background()
	database, err := db.Open(db.Config{Link: link, Timezone: "Asia/Shanghai"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	const table = "GOOQ_DM_IT"
	_, _ = database.Exec(ctx, "DROP TABLE IF EXISTS "+table)
	t.Cleanup(func() {
		_, _ = database.Exec(context.Background(), "DROP TABLE IF EXISTS "+table)
	})
	if _, err = database.Exec(ctx, `
CREATE TABLE GOOQ_DM_IT (
  ID BIGINT PRIMARY KEY,
  NAME VARCHAR(32) NOT NULL,
  ATTR_INDEX INT,
  CREATED_AT TIMESTAMP
)`); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 3, 2, 3, 4, 5, 0, time.UTC)
	if _, err = database.Exec(ctx, `INSERT INTO GOOQ_DM_IT (ID, NAME, CREATED_AT) VALUES (?, ?, ?)`, 1, "ada", created); err != nil {
		t.Fatal(err)
	}
	got, err := database.GetValue(ctx, `SELECT NAME FROM GOOQ_DM_IT WHERE ID = ?`, 1)
	if err != nil {
		t.Fatal(err)
	}
	if textOf(got) != "ada" {
		t.Fatalf("name = %#v", got)
	}
	tables, err := database.Tables(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !containsFold(tables, table) {
		t.Fatalf("tables = %v", tables)
	}
	columns, err := database.TableFields(ctx, table)
	if err != nil {
		t.Fatal(err)
	}
	if len(columns) != 4 {
		t.Fatalf("columns = %+v", columns)
	}
	byName := map[string]db.Column{}
	for _, column := range columns {
		byName[strings.ToUpper(column.Name)] = column
	}
	id := byName["ID"]
	if !id.Primary || id.LocalType != gooq.LocalTypeInt64 {
		t.Fatalf("id = %+v", id)
	}
	name := byName["NAME"]
	if name.Nullable || name.LocalType != gooq.LocalTypeString {
		t.Fatalf("name = %+v", name)
	}
	if byName["ATTR_INDEX"].LocalType != gooq.LocalTypeInt {
		t.Fatalf("index = %+v", byName["ATTR_INDEX"])
	}
	if byName["CREATED_AT"].LocalType != gooq.LocalTypeDatetime {
		t.Fatalf("created = %+v", byName["CREATED_AT"])
	}
	sqlText, args, err := dsl.Insert(liveTable()).
		Columns(liveTable().Field("ID"), liveTable().Field("NAME")).
		Values(2, "bea").
		OnConflictKey(liveTable().Field("ID")).
		DoUpdate(liveTable().Field("NAME"), "bea").
		ToSql(gooq.DialectDM)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Exec(ctx, sqlText, args...); err != nil {
		t.Fatalf("merge %s: %v", sqlText, err)
	}
	if database.Dialect() != gooq.DialectDM {
		t.Fatalf("dialect = %s", database.Dialect())
	}
}

func textOf(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return ""
	}
}

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}

type liveDMTable struct {
	*gooq.TableBase
}

func liveTable() *liveDMTable {
	return &liveDMTable{TableBase: gooq.NewTableBase(&gooq.TableMeta{TableName: "GOOQ_DM_IT"})}
}

func (t *liveDMTable) Field(name string) gooq.Field[any] {
	return gooq.NewFieldAt[any](t.TableBase, name)
}
