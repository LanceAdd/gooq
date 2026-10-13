//go:build dm

package dm

import (
	"context"
	"errors"
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

// suiteRow 是批量 ValuesOf 用的最小写入记录。
type suiteRow []gooq.Assignment

func (r suiteRow) Assignments() []gooq.Assignment { return r }

// TestDMSuiteLive 覆盖 dsl 执行链在达梦上的行为：软删、批量、事务、函数、分页、外连接拒绝。
func TestDMSuiteLive(t *testing.T) {
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
	schema := "GOOQ_S_" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:10]
	if _, err = database.Exec(ctx, `CREATE SCHEMA "`+schema+`" AUTHORIZATION SYSDBA`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec(context.Background(), `DROP SCHEMA "`+schema+`" CASCADE`)
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

	base, extra := link, ""
	if i := strings.Index(base, "?"); i >= 0 {
		base, extra = base[:i], base[i:]
	}
	schemaLink := base[:strings.LastIndex(base, "/")+1] + schema + extra
	if err = db.SetConfig(db.DefaultGroupName, db.Config{Link: schemaLink, Timezone: "Asia/Shanghai"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.RemoveConfig(db.DefaultGroupName) })
	instance, err := db.Instance()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("crud", func(t *testing.T) {
		if _, err = view.Exec(ctx, `CREATE TABLE ACCOUNT (
			ID INT PRIMARY KEY,
			NAME VARCHAR(64),
			STATUS VARCHAR(32),
			DELETED_AT DATETIME NULL
		)`); err != nil {
			t.Fatal(err)
		}
		account := newSuiteTable("ACCOUNT", "ID", "NAME", "STATUS", "DELETED_AT")

		if _, err = dsl.Insert(account).
			Columns(account.Field("ID"), account.Field("NAME"), account.Field("STATUS")).
			Values(1, "ann", "old").Values(2, "bob", "old").Values(3, "cat", "vip").
			Exec(ctx); err != nil {
			t.Fatal(err)
		}

		var names []string
		if err = dsl.Select(account.Field("NAME")).From(account).
			OrderAsc(account.Field("ID")).Column(ctx, &names); err != nil {
			t.Fatal(err)
		}
		if len(names) != 3 || names[0] != "ann" || names[2] != "cat" {
			t.Fatalf("names = %#v", names)
		}

		var count int64
		if count, err = dsl.Select(account.Field("ID")).From(account).Count(ctx); err != nil || count != 3 {
			t.Fatalf("count = %d, %v", count, err)
		}
		var found bool
		if found, err = dsl.Select(account.Field("ID")).From(account).
			Where(account.Field("NAME").Eq("bob")).Exists(ctx); err != nil || !found {
			t.Fatalf("exists = %v, %v", found, err)
		}

		var name string
		if err = dsl.Select(account.Field("NAME")).From(account).
			Where(account.Field("ID").Eq(1)).Value(ctx, &name); err != nil || name != "ann" {
			t.Fatalf("value = %q, %v", name, err)
		}

		row, err := dsl.Select(account.Field("NAME"), account.Field("STATUS")).From(account).
			Where(account.Field("ID").Eq(2)).Row(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var scanned struct {
			Name   string `orm:"NAME"`
			Status string `orm:"STATUS"`
		}
		if err = row.Struct(&scanned); err != nil {
			t.Fatal(err)
		}
		if scanned.Name != "bob" || scanned.Status != "old" {
			t.Fatalf("scanned = %#v", scanned)
		}

		if _, err = dsl.Update(account).Set(account.Field("STATUS"), "vip").
			Where(account.Field("ID").Eq(2)).Exec(ctx); err != nil {
			t.Fatal(err)
		}

		// 软删：DELETE 改写 UPDATE DELETED_AT。
		if _, err = dsl.Delete(account).Where(account.Field("ID").Eq(3)).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if count, err = dsl.Select(account.Field("ID")).From(account).Count(ctx); err != nil || count != 2 {
			t.Fatalf("count after soft delete = %d, %v", count, err)
		}
		if count, err = dsl.Select(account.Field("ID")).From(account).Unscoped().Count(ctx); err != nil || count != 3 {
			t.Fatalf("unscoped count = %d, %v", count, err)
		}

		// 分页：Page 渲染 LIMIT/OFFSET，只偏移渲染 OFFSET n ROWS。
		if err = dsl.Select(account.Field("NAME")).From(account).
			OrderAsc(account.Field("ID")).Page(2, 1).Column(ctx, &names); err != nil {
			t.Fatal(err)
		}
		if len(names) != 1 || names[0] != "bob" {
			t.Fatalf("page names = %#v", names)
		}
		if err = dsl.Select(account.Field("NAME")).From(account).
			OrderAsc(account.Field("ID")).Offset(1).Column(ctx, &names); err != nil {
			t.Fatal(err)
		}
		if len(names) != 1 || names[0] != "bob" {
			t.Fatalf("offset names = %#v", names)
		}

		if _, err = dsl.Delete(account).Unscoped().Where(account.Field("ID").Eq(1)).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if count, err = dsl.Select(account.Field("ID")).From(account).Unscoped().Count(ctx); err != nil || count != 2 {
			t.Fatalf("count after hard delete = %d, %v", count, err)
		}
	})

	t.Run("batch", func(t *testing.T) {
		if _, err = view.Exec(ctx, `CREATE TABLE BATCH (ID INT PRIMARY KEY, NAME VARCHAR(64))`); err != nil {
			t.Fatal(err)
		}
		batch := newSuiteTable("BATCH", "ID", "NAME")
		result, err := dsl.Insert(batch).ValuesOf(
			suiteRow{{Column: "ID", Value: 1}, {Column: "NAME", Value: "a"}},
			suiteRow{{Column: "ID", Value: 2}, {Column: "NAME", Value: "b"}},
		).Batch(1).Exec(ctx)
		if err != nil {
			t.Fatal(err)
		}
		affected, err := result.RowsAffected()
		if err != nil || affected != 2 {
			t.Fatalf("batch insert affected = %d, %v", affected, err)
		}

		result, err = dsl.Update(batch).ValuesOf(
			suiteRow{{Column: "ID", Value: 1}, {Column: "NAME", Value: "x"}},
			suiteRow{{Column: "ID", Value: 2}, {Column: "NAME", Value: "y"}},
		).Where(batch.Field("ID").Gt(1)).Exec(ctx)
		if err != nil {
			t.Fatal(err)
		}
		affected, err = result.RowsAffected()
		if err != nil || affected != 1 {
			t.Fatalf("batch update affected = %d, %v", affected, err)
		}
		var names []string
		if err = dsl.Select(batch.Field("NAME")).From(batch).
			OrderAsc(batch.Field("ID")).Column(ctx, &names); err != nil {
			t.Fatal(err)
		}
		if len(names) != 2 || names[0] != "a" || names[1] != "y" {
			t.Fatalf("names after batch update = %#v", names)
		}
	})

	t.Run("transaction", func(t *testing.T) {
		if _, err = view.Exec(ctx, `CREATE TABLE TXITEM (ID INT PRIMARY KEY, NAME VARCHAR(64))`); err != nil {
			t.Fatal(err)
		}
		txitem := newSuiteTable("TXITEM", "ID", "NAME")

		err = instance.Transaction(ctx, func(ctx context.Context, tx *db.Tx) error {
			if _, err := dsl.Insert(txitem).Columns(txitem.Field("ID"), txitem.Field("NAME")).
				Values(1, "rolled").Exec(ctx); err != nil {
				return err
			}
			return errors.New("abort")
		})
		if err == nil {
			t.Fatal("expected transaction error")
		}
		var count int64
		if count, err = dsl.Select(txitem.Field("ID")).From(txitem).Count(ctx); err != nil || count != 0 {
			t.Fatalf("rolled back rows = %d, %v", count, err)
		}

		if err = instance.Transaction(ctx, func(ctx context.Context, tx *db.Tx) error {
			if _, err := dsl.Insert(txitem).Columns(txitem.Field("ID"), txitem.Field("NAME")).
				Values(4, "outer").Exec(ctx); err != nil {
				return err
			}
			err := tx.Transaction(ctx, func(ctx context.Context, tx *db.Tx) error {
				if _, err := dsl.Insert(txitem).Columns(txitem.Field("ID"), txitem.Field("NAME")).
					Values(5, "inner").Exec(ctx); err != nil {
					return err
				}
				return errors.New("nested abort")
			})
			if err == nil {
				t.Fatal("expected nested error")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if count, err = dsl.Select(txitem.Field("ID")).From(txitem).Count(ctx); err != nil || count != 1 {
			t.Fatalf("rows after savepoint rollback = %d, %v", count, err)
		}
	})

	t.Run("time binding", func(t *testing.T) {
		if _, err = view.Exec(ctx, `CREATE TABLE TIMED (ID INT PRIMARY KEY, AT DATETIME NULL)`); err != nil {
			t.Fatal(err)
		}
		at := time.Date(2026, 10, 13, 9, 30, 0, 0, time.Local)
		if _, err = view.Exec(ctx, `INSERT INTO TIMED VALUES (1, ?), (2, NULL)`, at); err != nil {
			t.Fatal(err)
		}
		timed := newSuiteTable("TIMED", "ID", "AT")
		var got []*time.Time
		if err = dsl.Select(timed.Field("AT")).From(timed).
			OrderAsc(timed.Field("ID")).Column(ctx, &got); err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0] == nil || got[1] != nil {
			t.Fatalf("times = %#v", got)
		}
		if got[0].Format("2006-01-02 15:04:05") != at.Format("2006-01-02 15:04:05") {
			t.Fatalf("time roundtrip = %v, want %v", got[0], at)
		}
	})

	t.Run("functions", func(t *testing.T) {
		if _, err = view.Exec(ctx, `CREATE TABLE WORDS (ID INT PRIMARY KEY, NAME VARCHAR(64), SCORE DOUBLE, CREATED_AT DATETIME)`); err != nil {
			t.Fatal(err)
		}
		created := time.Date(2026, 10, 13, 8, 0, 0, 0, time.Local)
		if _, err = view.Exec(ctx, `INSERT INTO WORDS VALUES (1, 'a', 1.235, ?), (2, NULL, 2, ?)`, created, created); err != nil {
			t.Fatal(err)
		}
		words := newSuiteTable("WORDS", "ID", "NAME", "SCORE", "CREATED_AT")

		queries := []gooq.Expression{
			fn.IfNull(words.Field("NAME"), gooq.Str("unknown")),
			fn.CurDate(),
			fn.DateAdd(words.Field("CREATED_AT"), 1, "DAY"),
			fn.DateDiff(fn.Now(), words.Field("CREATED_AT")),
			fn.DateFormat(words.Field("CREATED_AT"), "%Y-%m-%d"),
			fn.Round(words.Field("SCORE"), 2),
			fn.GroupConcat(fn.GroupConcatOptions{Field: words.Field("NAME"), Separator: "|"}),
		}
		for _, expr := range queries {
			sqlText, args, err := dsl.Select(expr).From(words).ToSql(gooq.DialectDM)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = view.GetValue(ctx, sqlText, args...); err != nil {
				t.Fatalf("%s: %v", sqlText, err)
			}
		}
	})

	t.Run("outer join rejected", func(t *testing.T) {
		x := newSuiteTable("ACCOUNT", "ID", "STATUS")
		y := newSuiteTable("BATCH", "ID", "NAME")
		if _, _, err = dsl.Update(x).Set(x.Field("STATUS"), "vip").
			LeftJoin(y).On(y.Field("ID").EqExpr(x.Field("ID"))).
			ToSql(gooq.DialectDM); err == nil {
			t.Fatal("outer join update rendered on dm")
		}
		if _, _, err = dsl.Delete(x).LeftJoin(y).On(y.Field("ID").EqExpr(x.Field("ID"))).
			ToSql(gooq.DialectDM); err == nil {
			t.Fatal("outer join delete rendered on dm")
		}
	})
}

type suiteTable struct {
	*gooq.TableBase
}

func newSuiteTable(name string, columns ...string) *suiteTable {
	metas := make([]gooq.FieldMeta, len(columns))
	for i, column := range columns {
		metas[i] = gooq.FieldMeta{ColumnName: column, LocalType: gooq.LocalTypeString}
		if column == "ID" {
			metas[i].Primary = true
			metas[i].LocalType = gooq.LocalTypeInt
		}
		if column == "DELETED_AT" {
			metas[i].SoftDelete = true
			metas[i].LocalType = gooq.LocalTypeDatetime
		}
		if column == "AT" || column == "CREATED_AT" {
			metas[i].LocalType = gooq.LocalTypeDatetime
		}
	}
	return &suiteTable{
		TableBase: gooq.NewTableBase(&gooq.TableMeta{TableName: name, Fields: metas}),
	}
}
