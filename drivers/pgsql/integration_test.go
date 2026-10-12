//go:build pgsql

package pgsql

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
	"github.com/lanceadd/gooq/dsl"
	"github.com/lanceadd/gooq/fn"
)

func TestPostgresLive(t *testing.T) {
	link := os.Getenv("GOOQ_PG_LINK")
	if link == "" {
		t.Skip("GOOQ_PG_LINK is not set")
	}
	ctx := context.Background()
	database, err := db.Open(db.Config{Link: link})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	schema := "gooq_it_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	if _, err = database.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	})
	view, err := database.Schema(schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := view.Close(); err != nil {
			t.Error(err)
		}
	})

	t.Run("schema", func(t *testing.T) {
		got, err := view.GetValue(ctx, "SELECT current_schema()")
		if err != nil {
			t.Fatal(err)
		}
		if got != schema {
			t.Fatalf("current_schema = %v", got)
		}
		original, err := database.GetValue(ctx, "SELECT current_schema()")
		if err != nil {
			t.Fatal(err)
		}
		if original != "public" {
			t.Fatalf("original schema = %v", original)
		}
	})

	t.Run("password with space", func(t *testing.T) {
		role := "gooq_pw_" + strings.TrimPrefix(schema, "gooq_it_")
		if _, err = database.Exec(ctx, `CREATE ROLE `+role+` LOGIN PASSWORD 'a b'`); err != nil {
			t.Skip(err)
		}
		t.Cleanup(func() { _, _ = database.Exec(context.Background(), `DROP ROLE `+role) })
		var (
			host string
			port string
			name string
		)
		if err = database.GetScan(ctx, &host, `SELECT COALESCE(host(inet_server_addr()), '127.0.0.1')`); err != nil {
			t.Fatal(err)
		}
		if err = database.GetScan(ctx, &port, `SELECT inet_server_port()::text`); err != nil || port == "" {
			t.Skip("server port is unavailable")
		}
		if err = database.GetScan(ctx, &name, `SELECT current_database()`); err != nil {
			t.Fatal(err)
		}
		if _, err = database.Exec(ctx, `GRANT CONNECT ON DATABASE `+name+` TO `+role); err != nil {
			t.Skip(err)
		}
		opened, err := db.Open(db.Config{Link: "pgsql:" + role + ":a b@tcp(" + host + ":" + port + ")/" + name})
		if err != nil {
			t.Fatal(err)
		}
		defer opened.Close()
		got, err := opened.GetValue(ctx, "SELECT 1")
		if err != nil {
			t.Fatal(err)
		}
		if got != int64(1) {
			t.Fatalf("select 1 = %v", got)
		}
	})

	t.Run("application_name", func(t *testing.T) {
		appLink := link
		joiner := "?"
		if strings.Contains(link, "?") {
			joiner = "&"
		}
		appLink += joiner + "application_name=gooq_it"
		opened, err := db.Open(db.Config{Link: appLink})
		if err != nil {
			t.Fatal(err)
		}
		defer opened.Close()
		got, err := opened.GetValue(ctx, "SHOW application_name")
		if err != nil {
			t.Fatal(err)
		}
		if got != "gooq_it" {
			t.Fatalf("application_name = %v", got)
		}
	})

	t.Run("catalog", func(t *testing.T) {
		statements := []string{
			`CREATE TABLE parent (id int, name text) PARTITION BY LIST (id)`,
			`CREATE TABLE parent_p1 PARTITION OF parent FOR VALUES IN (1)`,
			`CREATE VIEW parent_v AS SELECT 1 AS id`,
			`CREATE TABLE uniqidx (id int PRIMARY KEY, email text, a int, name text)`,
			`CREATE UNIQUE INDEX uniqidx_email ON uniqidx (email)`,
			`CREATE UNIQUE INDEX uniqidx_pair ON uniqidx (a, name)`,
			`CREATE UNIQUE INDEX uniqidx_partial ON uniqidx (name) WHERE name IS NOT NULL`,
		}
		for _, statement := range statements {
			if _, err = view.Exec(ctx, statement); err != nil {
				t.Fatal(err)
			}
		}
		names, err := view.Tables(ctx)
		if err != nil {
			t.Fatal(err)
		}
		set := map[string]bool{}
		for _, name := range names {
			set[name] = true
		}
		if !set["parent"] || !set["parent_v"] || set["parent_p1"] {
			t.Fatalf("tables = %v", names)
		}
		columns, err := view.TableFields(ctx, "uniqidx")
		if err != nil {
			t.Fatal(err)
		}
		unique := map[string]bool{}
		for _, column := range columns {
			unique[column.Name] = column.Unique
		}
		if !unique["email"] || unique["a"] || unique["name"] || unique["id"] {
			t.Fatalf("unique flags = %#v", columns)
		}
	})

	t.Run("arrays and numeric", func(t *testing.T) {
		statements := []string{
			`CREATE TABLE arrays (
				id int PRIMARY KEY,
				i2 int2[], i4 int4[], i8 int8[],
				tx text[], vc varchar[], ch char(1)[],
				bl bool[], f4 float4[], f8 float8[],
				ba bytea[], uu uuid[]
			)`,
			`CREATE TABLE amounts (id int PRIMARY KEY, amount numeric(20,2))`,
		}
		for _, statement := range statements {
			if _, err = view.Exec(ctx, statement); err != nil {
				t.Fatal(err)
			}
		}
		id := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
		_, err = view.Exec(ctx, `INSERT INTO arrays VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			1, []int{1, 2}, []int{3, 4}, []int64{5, 6},
			[]string{"a", "b"}, []string{"c"}, []string{"d"},
			[]bool{true, false}, []float32{1.5}, []float64{2.5},
			[][]byte{{0x68, 0x69}}, []uuid.UUID{id},
		)
		if err != nil {
			t.Fatal(err)
		}
		var row struct {
			I2 []int       `orm:"i2"`
			I4 []int       `orm:"i4"`
			I8 []int64     `orm:"i8"`
			Tx []string    `orm:"tx"`
			Vc []string    `orm:"vc"`
			Ch []string    `orm:"ch"`
			Bl []bool      `orm:"bl"`
			F4 []float32   `orm:"f4"`
			F8 []float64   `orm:"f8"`
			Ba [][]byte    `orm:"ba"`
			Uu []uuid.UUID `orm:"uu"`
		}
		if err = view.GetScan(ctx, &row, `SELECT i2, i4, i8, tx, vc, ch, bl, f4, f8, ba, uu FROM arrays WHERE id = 1`); err != nil {
			t.Fatal(err)
		}
		if len(row.I4) != 2 || row.I4[0] != 3 || row.I4[1] != 4 {
			t.Fatalf("i4 = %#v", row.I4)
		}
		if len(row.I2) != 2 || row.I2[0] != 1 || len(row.I8) != 2 || row.I8[0] != 5 {
			t.Fatalf("ints = %#v %#v", row.I2, row.I8)
		}
		if len(row.Tx) != 2 || row.Tx[0] != "a" || len(row.Bl) != 2 || !row.Bl[0] || row.Bl[1] {
			t.Fatalf("text/bool = %#v %#v", row.Tx, row.Bl)
		}
		if len(row.F4) != 1 || row.F4[0] != 1.5 || len(row.F8) != 1 || row.F8[0] != 2.5 {
			t.Fatalf("floats = %#v %#v", row.F4, row.F8)
		}
		if len(row.Ba) != 1 || len(row.Ba[0]) != 2 || row.Ba[0][0] != 0x68 || len(row.Uu) != 1 || row.Uu[0] != id {
			t.Fatalf("bytea/uuid = %#v %#v", row.Ba, row.Uu)
		}
		if len(row.Vc) != 1 || row.Vc[0] != "c" || len(row.Ch) != 1 || !strings.HasPrefix(row.Ch[0], "d") {
			t.Fatalf("varchar/char = %#v %#v", row.Vc, row.Ch)
		}
		_, err = view.Exec(ctx, `INSERT INTO amounts VALUES (1, 1234567890123456.25)`)
		if err != nil {
			t.Fatal(err)
		}
		var amount struct {
			Amount string `orm:"amount"`
		}
		if err = view.GetScan(ctx, &amount, `SELECT amount FROM amounts WHERE id = 1`); err != nil {
			t.Fatal(err)
		}
		if amount.Amount != "1234567890123456.25" {
			t.Fatalf("amount = %q", amount.Amount)
		}
	})

	t.Run("upsert and joins", func(t *testing.T) {
		statements := []string{
			`CREATE TABLE item (id int PRIMARY KEY, name text)`,
			`INSERT INTO item VALUES (1, 'old'), (2, 'old')`,
			`CREATE TABLE account (id int PRIMARY KEY, status text, deleted_at timestamp)`,
			`INSERT INTO account VALUES (1, 'old', NULL), (2, 'old', NULL)`,
			`CREATE TABLE role_lj (id int PRIMARY KEY)`,
			`INSERT INTO role_lj VALUES (1)`,
			`CREATE TABLE flag (id int PRIMARY KEY)`,
			`INSERT INTO flag VALUES (1)`,
			`CREATE TABLE note (id int PRIMARY KEY)`,
			`INSERT INTO note VALUES (1), (2)`,
		}
		for _, statement := range statements {
			if _, err = view.Exec(ctx, statement); err != nil {
				t.Fatal(err)
			}
		}
		item := newLiveTable("item", "id", "name")
		sqlText, args, err := dsl.Insert(item).Columns(item.Field("id"), item.Field("name")).
			Values(1, "a").Values(2, "b").
			OnConflictKey(item.Field("id")).
			DoUpdate(item.Field("name"), gooq.Excluded(item.Field("name"))).
			ToSql(gooq.DialectPgsql)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sqlText, `EXCLUDED."name"`) {
			t.Fatalf("upsert sql = %s", sqlText)
		}
		if _, err = view.Exec(ctx, sqlText, args...); err != nil {
			t.Fatal(err)
		}
		var names []string
		if err = view.GetScan(ctx, &names, `SELECT name FROM item ORDER BY id`); err != nil {
			t.Fatal(err)
		}
		if len(names) != 2 || names[0] != "a" || names[1] != "b" {
			t.Fatalf("names = %#v", names)
		}

		account := newLiveTable("account", "id", "status", "deleted_at")
		role := newLiveTable("role_lj", "id")
		flag := newLiveTable("flag", "id")
		u := account.As("u")
		r := role.As("r")
		f := flag.As("f")
		sqlText, args, err = dsl.Update(u).Set(u.Field("status"), "vip").
			InnerJoin(f).On(gooq.Raw("1 = 1")).
			LeftJoin(r).On(r.Field("id").EqExpr(u.Field("id"))).
			ToSql(gooq.DialectPgsql)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sqlText, `FROM (SELECT 1) AS "_gooq"`) || !strings.Contains(sqlText, "LEFT JOIN") {
			t.Fatalf("mixed update sql = %s", sqlText)
		}
		if _, err = view.Exec(ctx, sqlText, args...); err != nil {
			t.Fatal(err)
		}
		var statuses []string
		if err = view.GetScan(ctx, &statuses, `SELECT status FROM account ORDER BY id`); err != nil {
			t.Fatal(err)
		}
		if len(statuses) != 2 || statuses[0] != "vip" || statuses[1] != "vip" {
			t.Fatalf("statuses = %#v", statuses)
		}

		note := newLiveTable("note", "id")
		nu := note.As("u")
		sqlText, args, err = dsl.Delete(nu).Where(nu.Field("id").Eq(2)).ToSql(gooq.DialectPgsql)
		if err != nil {
			t.Fatal(err)
		}
		if sqlText != `DELETE FROM "note" AS "u" WHERE "u"."id" = $1` {
			t.Fatalf("alias delete sql = %s", sqlText)
		}
		if _, err = view.Exec(ctx, sqlText, args...); err != nil {
			t.Fatal(err)
		}

		target := note.As("u")
		joined := flag.As("f")
		sqlText, args, err = dsl.Delete(target).InnerJoin(joined).On(joined.Field("id").EqExpr(target.Field("id"))).ToSql(gooq.DialectPgsql)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sqlText, `USING "flag" AS "f"`) {
			t.Fatalf("using delete sql = %s", sqlText)
		}
		if _, err = view.Exec(ctx, sqlText, args...); err != nil {
			t.Fatal(err)
		}

		sqlText, args, err = dsl.Delete(u).LeftJoin(r).On(r.Field("id").EqExpr(u.Field("id"))).ToSql(gooq.DialectPgsql)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sqlText, "FROM") || !strings.Contains(sqlText, "LEFT JOIN") {
			t.Fatalf("soft delete sql = %s", sqlText)
		}
		if _, err = view.Exec(ctx, sqlText, args...); err != nil {
			t.Fatal(err)
		}
		var deleted int64
		if err = view.GetScan(ctx, &deleted, `SELECT COUNT(*) FROM account WHERE deleted_at IS NOT NULL`); err != nil {
			t.Fatal(err)
		}
		if deleted != 2 {
			t.Fatalf("soft-deleted rows = %d", deleted)
		}
	})

	t.Run("functions", func(t *testing.T) {
		if _, err = view.Exec(ctx, `CREATE TABLE words (id int PRIMARY KEY, name text, score float8, created_at timestamp)`); err != nil {
			t.Fatal(err)
		}
		if _, err = view.Exec(ctx, `INSERT INTO words VALUES (1, 'a', 1.235, $1), (2, NULL, 2, $1)`, time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)); err != nil {
			t.Fatal(err)
		}
		words := newLiveTable("words", "id", "name", "score", "created_at")
		queries := []gooq.Expression{
			fn.IfNull(words.Field("name"), gooq.Str("unknown")),
			fn.CurDate(),
			fn.DateAdd(words.Field("created_at"), 1, "DAY"),
			fn.DateDiff(words.Field("created_at"), fn.Now()),
			fn.Round(words.Field("score"), 2),
			fn.GroupConcat(fn.GroupConcatOptions{Field: words.Field("name"), Separator: "a'b"}),
			fn.GroupConcat(fn.GroupConcatOptions{Field: words.Field("name"), Separator: `a\b`}),
		}
		for _, expr := range queries {
			sqlText, args, err := dsl.Select(expr).From(words).ToSql(gooq.DialectPgsql)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = view.GetValue(ctx, sqlText, args...); err != nil {
				t.Fatalf("%s: %v", sqlText, err)
			}
		}
	})
}

type liveTable struct {
	*gooq.TableBase
	fields map[string]gooq.Field[any]
}

func newLiveTable(name string, columns ...string) *liveTable {
	metas := make([]gooq.FieldMeta, len(columns))
	for i, column := range columns {
		metas[i] = gooq.FieldMeta{ColumnName: column, LocalType: gooq.LocalTypeString}
		if column == "id" {
			metas[i].Primary = true
			metas[i].LocalType = gooq.LocalTypeInt
		}
		if column == "deleted_at" {
			metas[i].SoftDelete = true
			metas[i].LocalType = gooq.LocalTypeDatetime
		}
	}
	table := &liveTable{
		TableBase: gooq.NewTableBase(&gooq.TableMeta{TableName: name, Fields: metas}),
		fields:    map[string]gooq.Field[any]{},
	}
	for _, column := range columns {
		table.fields[column] = gooq.NewFieldAt[any](table.TableBase, column)
	}
	return table
}

func (t *liveTable) Field(column string) gooq.Field[any] { return t.fields[column] }

func (t *liveTable) As(alias string) *liveTable {
	cloned := newLiveTable(t.TableName(), columnNames(t)...)
	cloned.TableBase = cloned.TableBase.As(alias)
	for column := range cloned.fields {
		cloned.fields[column] = gooq.NewFieldAt[any](cloned.TableBase, column)
	}
	return cloned
}

func columnNames(t *liveTable) []string {
	names := make([]string, 0, len(t.fields))
	for name := range t.fields {
		names = append(names, name)
	}
	return names
}
