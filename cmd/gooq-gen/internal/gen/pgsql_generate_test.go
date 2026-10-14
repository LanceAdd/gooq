//go:build pgsql

package gen

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/text/gstr"
	"github.com/google/uuid"

	"github.com/lanceadd/gooq/db"
	_ "github.com/lanceadd/gooq/drivers/pgsql"
)

func TestGenerateFromPostgres(t *testing.T) {
	link := os.Getenv("GOOQ_PG_LINK")
	if link == "" {
		t.Skip("GOOQ_PG_LINK is not set")
	}
	ctx := context.Background()
	database, err := db.Open(db.Config{Link: link})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	schema := "gooq_gen_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	if _, err = database.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer database.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	statements := []string{
		`CREATE TABLE sample (
			id int PRIMARY KEY,
			amount numeric(20,2),
			tags text[],
			span interval,
			ids uuid[]
		)`,
		`CREATE UNIQUE INDEX sample_amount ON sample (amount)`,
		`CREATE UNIQUE INDEX sample_partial ON sample (span) WHERE span IS NOT NULL`,
	}
	view, err := database.Schema(schema)
	if err != nil {
		t.Fatal(err)
	}
	defer view.Close()
	for _, statement := range statements {
		if _, err = view.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	genLink := link
	joiner := "?"
	if strings.Contains(link, "?") {
		joiner = "&"
	}
	genLink += joiner + "options=" + url.QueryEscape("-csearch_path="+schema)
	out := gfile.Temp("gooq-gen-pgsql", schema)
	if err = os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	defer gfile.Remove(out)
	Generate(ctx, Input{Link: genLink, Path: out})
	content := gfile.GetContents(filepath.Join(out, "table", "sample.go"))
	if !gstr.Contains(content, `LocalType: gooq.LocalTypeDecimal`) || !gstr.Contains(content, "gooq.Field[string]") {
		t.Fatalf("numeric was not generated as string:\n%s", content)
	}
	if !gstr.Contains(content, "gooq.Field[[]string]") || !gstr.Contains(content, `LocalType: gooq.LocalTypeString`) {
		t.Fatalf("array or interval type missing:\n%s", content)
	}
	if !gstr.Contains(content, "[]uuid.UUID") || !gstr.Contains(content, `"github.com/google/uuid"`) {
		t.Fatalf("uuid slice import missing:\n%s", content)
	}
	if !gstr.Contains(content, `ColumnName: "amount"`) || !gstr.Contains(content, "Unique: true") {
		t.Fatalf("unique amount missing:\n%s", content)
	}
	if gstr.Contains(content, `ColumnName: "id", LocalType: gooq.LocalTypeInt, Primary: true, Unique: true`) {
		t.Fatalf("primary key was marked unique:\n%s", content)
	}
}
