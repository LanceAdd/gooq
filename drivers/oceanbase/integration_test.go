//go:build oceanbase

package oceanbase

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

func TestOceanBaseLive(t *testing.T) {
	link := os.Getenv("GOOQ_OB_LINK")
	if link == "" {
		t.Skip("GOOQ_OB_LINK is not set")
	}
	ctx := context.Background()
	database, err := db.Open(db.Config{Link: link, Charset: "utf8mb4"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	const table = "gooq_ob_it"
	_, _ = database.Exec(ctx, "DROP TABLE IF EXISTS "+table)
	t.Cleanup(func() {
		_, _ = database.Exec(context.Background(), "DROP TABLE IF EXISTS "+table)
	})
	if _, err = database.Exec(ctx, `
CREATE TABLE gooq_ob_it (
  id bigint PRIMARY KEY AUTO_INCREMENT,
  name varchar(32) NOT NULL,
  created_at datetime NULL
)`); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 3, 2, 3, 4, 5, 0, time.UTC)
	if _, err = database.Exec(ctx, "INSERT INTO gooq_ob_it (name, created_at) VALUES (?, ?)", "ada", created); err != nil {
		t.Fatal(err)
	}
	got, err := database.GetValue(ctx, "SELECT name FROM gooq_ob_it WHERE id = ?", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ada" {
		t.Fatalf("name = %#v", got)
	}
	tables, err := database.Tables(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, name := range tables {
		if name == table {
			found = true
		}
	}
	if !found {
		t.Fatalf("tables = %v", tables)
	}
	columns, err := database.TableFields(ctx, table)
	if err != nil {
		t.Fatal(err)
	}
	if len(columns) != 3 || columns[0].Name != "id" || !columns[0].Primary || !columns[0].AutoIncrement {
		t.Fatalf("columns = %+v", columns)
	}
	if columns[1].LocalType != gooq.LocalTypeString || columns[2].LocalType != gooq.LocalTypeDatetime {
		t.Fatalf("types = %+v", columns)
	}
	if database.Dialect() != gooq.DialectOceanBase {
		t.Fatalf("dialect = %s", database.Dialect())
	}
}
