//go:build gaussdb

package gaussdb

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

// gsRow 是批量 ValuesOf 用的最小写入记录。
type gsRow []gooq.Assignment

func (r gsRow) Assignments() []gooq.Assignment { return r }

func TestGaussDBLive(t *testing.T) {
	link := os.Getenv("GOOQ_GAUSSDB_LINK")
	if link == "" {
		t.Skip("GOOQ_GAUSSDB_LINK is not set")
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

	dropTables := func() {
		for _, name := range []string{"gooq_it_acc", "gooq_it_item", "gooq_it_batch", "gooq_it_tx", "gooq_it_words"} {
			_, _ = database.Exec(context.Background(), "DROP TABLE IF EXISTS "+name)
		}
	}
	dropTables()
	t.Cleanup(dropTables)

	// dsl 终结方法按配置组解析连接：用默认组直连。
	if err = db.SetConfig(db.DefaultGroupName, db.Config{Link: link}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.RemoveConfig(db.DefaultGroupName) })
	instance, err := db.Instance()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("schema", func(t *testing.T) {
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
		defer view.Close()
		got, err := view.GetValue(ctx, "SELECT current_schema()")
		if err != nil {
			t.Fatal(err)
		}
		if got != schema {
			t.Fatalf("current_schema = %v", got)
		}
	})

	t.Run("create table like", func(t *testing.T) {
		schema := "gooq_like_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
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
		defer view.Close()

		template, target := "gooq_like_template", "gooq_like_target"
		if _, err = view.Exec(ctx, `CREATE TABLE `+template+` (
			id bigint PRIMARY KEY,
			name varchar(64) NOT NULL DEFAULT 'x',
			code varchar(16) UNIQUE)`); err != nil {
			t.Fatal(err)
		}
		if _, err = view.Exec(ctx, `INSERT INTO `+template+` (id, name, code) VALUES (1, 'a', 'c')`); err != nil {
			t.Fatal(err)
		}
		if err = view.CreateTableLike(ctx, template, target); err != nil {
			t.Fatal(err)
		}
		if err = view.CreateTableLike(ctx, template, target); err != nil {
			t.Fatalf("repeat: %v", err)
		}
		var count int64
		if err = view.Value(ctx, &count, "SELECT COUNT(*) FROM "+target); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("clone copied %d rows", count)
		}
		if _, err = view.Exec(ctx, `INSERT INTO `+target+` (id) VALUES (2)`); err != nil {
			t.Fatal(err)
		}
		var name string
		if err = view.Value(ctx, &name, "SELECT name FROM "+target+" WHERE id = 2"); err != nil {
			t.Fatal(err)
		}
		if name != "x" {
			t.Fatalf("default = %q", name)
		}
		columns, err := view.TableFields(ctx, target)
		if err != nil {
			t.Fatal(err)
		}
		if len(columns) != 3 || !columns[0].Primary || columns[1].Nullable || !columns[2].Unique {
			t.Fatalf("columns = %+v", columns)
		}
	})

	t.Run("crud", func(t *testing.T) {
		if _, err = database.Exec(ctx, `CREATE TABLE gooq_it_acc (
			id INT PRIMARY KEY,
			name TEXT,
			status TEXT,
			deleted_at TIMESTAMP NULL
		)`); err != nil {
			t.Fatal(err)
		}
		account := newGSTable("gooq_it_acc", "id", "name", "status", "deleted_at")

		if _, err = dsl.Insert(account).
			Columns(account.Field("id"), account.Field("name"), account.Field("status")).
			Values(1, "ann", "old").Values(2, "bob", "old").Values(3, "cat", "vip").
			Exec(ctx); err != nil {
			t.Fatal(err)
		}

		var names []string
		if err = dsl.Select(account.Field("name")).From(account).
			OrderAsc(account.Field("id")).Column(ctx, &names); err != nil {
			t.Fatal(err)
		}
		if len(names) != 3 || names[0] != "ann" || names[2] != "cat" {
			t.Fatalf("names = %#v", names)
		}

		var count int64
		if count, err = dsl.Select(account.Field("id")).From(account).Count(ctx); err != nil || count != 3 {
			t.Fatalf("count = %d, %v", count, err)
		}
		exists, err := dsl.Select(account.Field("id")).From(account).
			Where(account.Field("name").Eq("bob")).Exists(ctx)
		if err != nil || !exists {
			t.Fatalf("exists = %v, %v", exists, err)
		}

		var name string
		if err = dsl.Select(account.Field("name")).From(account).
			Where(account.Field("id").Eq(1)).Value(ctx, &name); err != nil || name != "ann" {
			t.Fatalf("value = %q, %v", name, err)
		}

		row, err := dsl.Select(account.Field("name"), account.Field("status")).From(account).
			Where(account.Field("id").Eq(2)).Row(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var scanned struct {
			Name   string `orm:"name"`
			Status string `orm:"status"`
		}
		if err = row.Struct(&scanned); err != nil {
			t.Fatal(err)
		}
		if scanned.Name != "bob" || scanned.Status != "old" {
			t.Fatalf("scanned = %#v", scanned)
		}

		if _, err = dsl.Update(account).Set(account.Field("status"), "vip").
			Where(account.Field("id").Eq(2)).Exec(ctx); err != nil {
			t.Fatal(err)
		}

		// 软删：DELETE 改写 UPDATE deleted_at。
		if _, err = dsl.Delete(account).Where(account.Field("id").Eq(3)).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if count, err = dsl.Select(account.Field("id")).From(account).Count(ctx); err != nil || count != 2 {
			t.Fatalf("count after soft delete = %d, %v", count, err)
		}
		if count, err = dsl.Select(account.Field("id")).From(account).Unscoped().Count(ctx); err != nil || count != 3 {
			t.Fatalf("unscoped count = %d, %v", count, err)
		}

		// 分页在 PG 系方言上是 LIMIT/OFFSET。
		if err = dsl.Select(account.Field("name")).From(account).
			OrderAsc(account.Field("id")).Page(2, 1).Column(ctx, &names); err != nil {
			t.Fatal(err)
		}
		if len(names) != 1 || names[0] != "bob" {
			t.Fatalf("page names = %#v", names)
		}
		if err = dsl.Select(account.Field("name")).From(account).
			OrderAsc(account.Field("id")).Offset(1).Column(ctx, &names); err != nil {
			t.Fatal(err)
		}
		if len(names) != 1 || names[0] != "bob" {
			t.Fatalf("offset names = %#v", names)
		}

		if _, err = dsl.Delete(account).Unscoped().Where(account.Field("id").Eq(1)).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if count, err = dsl.Select(account.Field("id")).From(account).Unscoped().Count(ctx); err != nil || count != 2 {
			t.Fatalf("count after hard delete = %d, %v", count, err)
		}
	})

	t.Run("upsert and returning", func(t *testing.T) {
		if _, err = database.Exec(ctx, `CREATE TABLE gooq_it_item (id INT PRIMARY KEY, name TEXT)`); err != nil {
			t.Fatal(err)
		}
		if _, err = database.Exec(ctx, `INSERT INTO gooq_it_item VALUES (1, 'old'), (2, 'old')`); err != nil {
			t.Fatal(err)
		}
		item := newGSTable("gooq_it_item", "id", "name")

		sqlText, args, err := dsl.Insert(item).Columns(item.Field("id"), item.Field("name")).
			Values(1, "a").
			OnConflictKey(item.Field("id")).
			DoUpdate(item.Field("name"), gooq.Excluded(item.Field("name"))).
			ToSql(gooq.DialectGaussDB)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sqlText, "ON DUPLICATE KEY UPDATE") || !strings.Contains(sqlText, `VALUES("name")`) {
			t.Fatalf("upsert sql = %s", sqlText)
		}
		if _, err = database.Exec(ctx, sqlText, args...); err != nil {
			t.Fatal(err)
		}
		var name string
		if err = database.Value(ctx, &name, `SELECT name FROM gooq_it_item WHERE id = 1`); err != nil || name != "a" {
			t.Fatalf("upsert update = %q, %v", name, err)
		}

		// 未匹配时插入。
		if _, err = dsl.Insert(item).Columns(item.Field("id"), item.Field("name")).
			Values(3, "c").OnConflictKey(item.Field("id")).
			DoUpdate(item.Field("name"), gooq.Excluded(item.Field("name"))).
			Exec(ctx); err != nil {
			t.Fatal(err)
		}
		var rows []string
		if err = database.Column(ctx, &rows, `SELECT name FROM gooq_it_item ORDER BY id`); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 3 || rows[0] != "a" || rows[1] != "old" || rows[2] != "c" {
			t.Fatalf("rows after upsert = %#v", rows)
		}

		// DoNothing 在 GaussDB 上没有对应子句，渲染期返回错误。
		if _, _, err = dsl.Insert(item).Columns(item.Field("id"), item.Field("name")).
			Values(9, "x").DoNothing().ToSql(gooq.DialectGaussDB); err == nil {
			t.Fatal("do nothing rendered on gaussdb")
		}

		// RETURNING：插入后直接取回。
		var id int64
		if err = dsl.Insert(item).Columns(item.Field("id"), item.Field("name")).
			Values(4, "d").Returning(item.Field("id")).Value(ctx, &id); err != nil || id != 4 {
			t.Fatalf("returning id = %d, %v", id, err)
		}
	})

	t.Run("batch", func(t *testing.T) {
		if _, err = database.Exec(ctx, `CREATE TABLE gooq_it_batch (id INT PRIMARY KEY, name TEXT)`); err != nil {
			t.Fatal(err)
		}
		batch := newGSTable("gooq_it_batch", "id", "name")
		result, err := dsl.Insert(batch).ValuesOf(
			gsRow{{Column: "id", Value: 1}, {Column: "name", Value: "a"}},
			gsRow{{Column: "id", Value: 2}, {Column: "name", Value: "b"}},
		).Batch(1).Exec(ctx)
		if err != nil {
			t.Fatal(err)
		}
		affected, err := result.RowsAffected()
		if err != nil || affected != 2 {
			t.Fatalf("batch insert affected = %d, %v", affected, err)
		}

		result, err = dsl.Update(batch).ValuesOf(
			gsRow{{Column: "id", Value: 1}, {Column: "name", Value: "x"}},
			gsRow{{Column: "id", Value: 2}, {Column: "name", Value: "y"}},
		).Where(batch.Field("id").Gt(1)).Exec(ctx)
		if err != nil {
			t.Fatal(err)
		}
		affected, err = result.RowsAffected()
		if err != nil || affected != 1 {
			t.Fatalf("batch update affected = %d, %v", affected, err)
		}
		var names []string
		if err = dsl.Select(batch.Field("name")).From(batch).
			OrderAsc(batch.Field("id")).Column(ctx, &names); err != nil {
			t.Fatal(err)
		}
		if len(names) != 2 || names[0] != "a" || names[1] != "y" {
			t.Fatalf("names after batch update = %#v", names)
		}
	})

	t.Run("transaction", func(t *testing.T) {
		if _, err = database.Exec(ctx, `CREATE TABLE gooq_it_tx (id INT PRIMARY KEY, name TEXT)`); err != nil {
			t.Fatal(err)
		}
		txitem := newGSTable("gooq_it_tx", "id", "name")

		err = instance.Transaction(ctx, func(ctx context.Context, tx *db.Tx) error {
			if _, err := dsl.Insert(txitem).Columns(txitem.Field("id"), txitem.Field("name")).
				Values(1, "rolled").Exec(ctx); err != nil {
				return err
			}
			return errors.New("abort")
		})
		if err == nil {
			t.Fatal("expected transaction error")
		}
		var count int64
		if count, err = dsl.Select(txitem.Field("id")).From(txitem).Count(ctx); err != nil || count != 0 {
			t.Fatalf("rolled back rows = %d, %v", count, err)
		}

		if err = instance.Transaction(ctx, func(ctx context.Context, tx *db.Tx) error {
			if _, err := dsl.Insert(txitem).Columns(txitem.Field("id"), txitem.Field("name")).
				Values(4, "outer").Exec(ctx); err != nil {
				return err
			}
			err := tx.Transaction(ctx, func(ctx context.Context, tx *db.Tx) error {
				if _, err := dsl.Insert(txitem).Columns(txitem.Field("id"), txitem.Field("name")).
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
		if count, err = dsl.Select(txitem.Field("id")).From(txitem).Count(ctx); err != nil || count != 1 {
			t.Fatalf("rows after savepoint rollback = %d, %v", count, err)
		}
	})

	t.Run("functions", func(t *testing.T) {
		if _, err = database.Exec(ctx, `CREATE TABLE gooq_it_words (id INT PRIMARY KEY, name TEXT, score DOUBLE PRECISION, created_at TIMESTAMP)`); err != nil {
			t.Fatal(err)
		}
		created := time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)
		if _, err = database.Exec(ctx, `INSERT INTO gooq_it_words VALUES (1, 'a', 1.235, $1), (2, NULL, 2, $1)`, created); err != nil {
			t.Fatal(err)
		}
		words := newGSTable("gooq_it_words", "id", "name", "score", "created_at")

		queries := []gooq.Expression{
			fn.IfNull(words.Field("name"), gooq.Str("unknown")),
			fn.CurDate(),
			fn.DateDiff(words.Field("created_at"), fn.Now()),
			fn.DateFormat(words.Field("created_at"), "%Y-%m-%d"),
			fn.Round(words.Field("score"), 2),
			fn.GroupConcat(fn.GroupConcatOptions{Field: words.Field("name"), Separator: "|"}),
		}
		for _, expr := range queries {
			sqlText, args, err := dsl.Select(expr).From(words).ToSql(gooq.DialectGaussDB)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = database.GetValue(ctx, sqlText, args...); err != nil {
				t.Fatalf("%s: %v", sqlText, err)
			}
		}

		sqlText, args, err := dsl.Select(fn.DateAdd(words.Field("created_at"), 1, "DAY")).
			From(words).ToSql(gooq.DialectGaussDB)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(sqlText, "make_interval") || !strings.Contains(sqlText, "INTERVAL '1 DAY'") {
			t.Fatalf("date add sql = %s", sqlText)
		}
		if _, err = database.GetValue(ctx, sqlText, args...); err != nil {
			t.Fatalf("date add: %v", err)
		}
	})

	t.Run("outer join rejected", func(t *testing.T) {
		x := newGSTable("gooq_it_item", "id", "name")
		y := newGSTable("gooq_it_batch", "id", "name")
		if _, _, err = dsl.Update(x).Set(x.Field("name"), "vip").
			LeftJoin(y).On(y.Field("id").EqExpr(x.Field("id"))).
			ToSql(gooq.DialectGaussDB); err == nil || !strings.Contains(err.Error(), "gaussdb") {
			t.Fatalf("outer join update on gaussdb: %v", err)
		}
		if _, _, err = dsl.Delete(x).LeftJoin(y).On(y.Field("id").EqExpr(x.Field("id"))).
			ToSql(gooq.DialectGaussDB); err == nil || !strings.Contains(err.Error(), "gaussdb") {
			t.Fatalf("outer join delete on gaussdb: %v", err)
		}
	})
}

type gsTable struct {
	*gooq.TableBase
}

func newGSTable(name string, columns ...string) *gsTable {
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
	return &gsTable{
		TableBase: gooq.NewTableBase(&gooq.TableMeta{TableName: name, Fields: metas}),
	}
}
