//go:build mysql

package mysql

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
	"github.com/lanceadd/gooq/dsl"
	"github.com/lanceadd/gooq/fn"
)

// liveRow 是批量 ValuesOf 用的最小写入记录。
type liveRow []gooq.Assignment

func (r liveRow) Assignments() []gooq.Assignment { return r }

func TestMySQL8Live(t *testing.T) {
	link := os.Getenv("GOOQ_MYSQL_LINK")
	if link == "" {
		t.Skip("GOOQ_MYSQL_LINK is not set")
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
	if _, err = database.Exec(ctx, "CREATE DATABASE `"+schema+"`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec(context.Background(), "DROP DATABASE `"+schema+"`")
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

	// dsl 终结方法按配置组解析连接：把测试库注册成默认组。
	base, extra := link, ""
	if i := strings.Index(base, "?"); i >= 0 {
		base, extra = base[:i], base[i:]
	}
	schemaLink := base[:strings.LastIndex(base, "/")+1] + schema + extra
	if err = db.SetConfig(db.DefaultGroupName, db.Config{Link: schemaLink}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.RemoveConfig(db.DefaultGroupName) })
	instance, err := db.Instance()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("schema", func(t *testing.T) {
		got, err := view.GetValue(ctx, "SELECT DATABASE()")
		if err != nil {
			t.Fatal(err)
		}
		if got != schema {
			t.Fatalf("database = %v", got)
		}
		original, err := database.GetValue(ctx, "SELECT DATABASE()")
		if err != nil {
			t.Fatal(err)
		}
		if original == schema {
			t.Fatalf("original database changed to %v", original)
		}
	})

	t.Run("charset", func(t *testing.T) {
		withCharset := link
		joiner := "?"
		if strings.Contains(link, "?") {
			joiner = "&"
		}
		withCharset += joiner + "charset=utf8mb4"
		opened, err := db.Open(db.Config{Link: withCharset})
		if err != nil {
			t.Fatal(err)
		}
		defer opened.Close()
		got, err := opened.GetValue(ctx, "SELECT @@character_set_client")
		if err != nil {
			t.Fatal(err)
		}
		if got != "utf8mb4" {
			t.Fatalf("character_set_client = %v", got)
		}
	})

	t.Run("catalog", func(t *testing.T) {
		statements := []string{
			"CREATE TABLE uniqidx (id INT PRIMARY KEY AUTO_INCREMENT, email VARCHAR(64), a INT, name VARCHAR(64))",
			"CREATE UNIQUE INDEX uniqidx_email ON uniqidx (email)",
			"CREATE TABLE plain (id INT PRIMARY KEY)",
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
		if !set["uniqidx"] || !set["plain"] {
			t.Fatalf("tables = %v", names)
		}
		columns, err := view.TableFields(ctx, "uniqidx")
		if err != nil {
			t.Fatal(err)
		}
		byName := map[string]db.Column{}
		for _, column := range columns {
			byName[column.Name] = column
		}
		if !byName["id"].Primary || !byName["id"].AutoIncrement {
			t.Fatalf("id column = %#v", byName["id"])
		}
		if !byName["email"].Unique || byName["a"].Unique {
			t.Fatalf("unique flags = %#v", byName)
		}
		if !byName["name"].Nullable {
			t.Fatalf("nullable flag = %#v", byName["name"])
		}
	})

	t.Run("crud", func(t *testing.T) {
		if _, err = view.Exec(ctx, `CREATE TABLE account (
			id INT PRIMARY KEY AUTO_INCREMENT,
			name VARCHAR(64),
			status VARCHAR(32),
			deleted_at DATETIME NULL
		)`); err != nil {
			t.Fatal(err)
		}
		account := newLiveTable("account", "id", "name", "status", "deleted_at")

		result, err := dsl.Insert(account).Columns(account.Field("name"), account.Field("status")).
			Values("ann", "old").Exec(ctx)
		if err != nil {
			t.Fatal(err)
		}
		first, err := result.LastInsertId()
		if err != nil || first != 1 {
			t.Fatalf("last insert id = %v, %v", first, err)
		}
		_, err = dsl.Insert(account).Columns(account.Field("name"), account.Field("status")).
			Values("bob", "old").Values("cat", "vip").Exec(ctx)
		if err != nil {
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
		count, err = dsl.Select(account.Field("id")).From(account).Count(ctx)
		if err != nil || count != 3 {
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
		if err = dsl.Select(account.Field("status")).From(account).
			Where(account.Field("id").Eq(2)).Value(ctx, &name); err != nil || name != "vip" {
			t.Fatalf("updated status = %q, %v", name, err)
		}

		// 软删：DELETE 改写成 UPDATE deleted_at；计数默认过滤已删。
		if _, err = dsl.Delete(account).Where(account.Field("id").Eq(3)).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		count, err = dsl.Select(account.Field("id")).From(account).Count(ctx)
		if err != nil || count != 2 {
			t.Fatalf("count after soft delete = %d, %v", count, err)
		}
		count, err = dsl.Select(account.Field("id")).From(account).Unscoped().Count(ctx)
		if err != nil || count != 3 {
			t.Fatalf("unscoped count = %d, %v", count, err)
		}

		// 分页与只用偏移的渲染在 MySQL 上可直接执行。
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

		// 硬删。
		if _, err = dsl.Delete(account).Unscoped().Where(account.Field("id").Eq(1)).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		count, err = dsl.Select(account.Field("id")).From(account).Unscoped().Count(ctx)
		if err != nil || count != 2 {
			t.Fatalf("count after hard delete = %d, %v", count, err)
		}
	})

	t.Run("batch", func(t *testing.T) {
		if _, err = view.Exec(ctx, `CREATE TABLE batch (id INT PRIMARY KEY, name VARCHAR(64))`); err != nil {
			t.Fatal(err)
		}
		batch := newLiveTable("batch", "id", "name")

		// 分段插入：Batch(1) 把两行拆成两条 INSERT。
		result, err := dsl.Insert(batch).ValuesOf(
			liveRow{{Column: "id", Value: 1}, {Column: "name", Value: "a"}},
			liveRow{{Column: "id", Value: 2}, {Column: "name", Value: "b"}},
		).Batch(1).Exec(ctx)
		if err != nil {
			t.Fatal(err)
		}
		affected, err := result.RowsAffected()
		if err != nil || affected != 2 {
			t.Fatalf("batch insert affected = %d, %v", affected, err)
		}

		// 批量更新带上 Where：每条语句都 AND 上条件。
		result, err = dsl.Update(batch).ValuesOf(
			liveRow{{Column: "id", Value: 1}, {Column: "name", Value: "x"}},
			liveRow{{Column: "id", Value: 2}, {Column: "name", Value: "y"}},
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

		// 批量删除（软删表的 deleted_at 在 batch 表上未标记，这里按主键硬删）。
		result, err = dsl.Delete(batch).ValuesOf(
			liveRow{{Column: "id", Value: 1}},
		).Exec(ctx)
		if err != nil {
			t.Fatal(err)
		}
		affected, err = result.RowsAffected()
		if err != nil || affected != 1 {
			t.Fatalf("batch delete affected = %d, %v", affected, err)
		}
	})

	t.Run("upsert", func(t *testing.T) {
		if _, err = view.Exec(ctx, `CREATE TABLE item (id INT PRIMARY KEY, name VARCHAR(64))`); err != nil {
			t.Fatal(err)
		}
		if _, err = view.Exec(ctx, `INSERT INTO item VALUES (1, 'old'), (2, 'old')`); err != nil {
			t.Fatal(err)
		}
		item := newLiveTable("item", "id", "name")

		sqlText, args, err := dsl.Insert(item).Columns(item.Field("id"), item.Field("name")).
			Values(1, "a").Values(2, "b").
			OnConflictKey(item.Field("id")).
			DoUpdate(item.Field("name"), gooq.Excluded(item.Field("name"))).
			ToSql(gooq.DialectMySQL)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sqlText, "ON DUPLICATE KEY UPDATE") || !strings.Contains(sqlText, "VALUES(`name`)") {
			t.Fatalf("upsert sql = %s", sqlText)
		}
		if _, err = view.Exec(ctx, sqlText, args...); err != nil {
			t.Fatal(err)
		}
		var names []string
		if err = view.Column(ctx, &names, `SELECT name FROM item ORDER BY id`); err != nil {
			t.Fatal(err)
		}
		if len(names) != 2 || names[0] != "a" || names[1] != "b" {
			t.Fatalf("names after upsert = %#v", names)
		}

		// DoNothing 渲染 INSERT IGNORE，冲突时保留旧值。
		if _, err = dsl.Insert(item).Columns(item.Field("id"), item.Field("name")).
			Values(1, "ignored").DoNothing().Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if err = view.Column(ctx, &names, `SELECT name FROM item ORDER BY id`); err != nil {
			t.Fatal(err)
		}
		if names[0] != "a" {
			t.Fatalf("insert ignore changed row = %#v", names)
		}

		// 无更新列的 upsert 构造错误在渲染期返回。
		if _, _, err = dsl.Insert(item).Columns(item.Field("id"), item.Field("name")).
			Values(1, "x").OnConflictKey(item.Field("id")).ToSql(gooq.DialectMySQL); err == nil {
			t.Fatal("empty upsert rendered")
		}
	})

	t.Run("joins", func(t *testing.T) {
		statements := []string{
			`CREATE TABLE jaccount (id INT PRIMARY KEY, status VARCHAR(32), deleted_at DATETIME NULL)`,
			`INSERT INTO jaccount VALUES (1, 'old', NULL), (2, 'old', NULL)`,
			`CREATE TABLE flag (id INT PRIMARY KEY)`,
			`INSERT INTO flag VALUES (1)`,
			`CREATE TABLE note (id INT PRIMARY KEY)`,
			`INSERT INTO note VALUES (1), (2)`,
		}
		for _, statement := range statements {
			if _, err = view.Exec(ctx, statement); err != nil {
				t.Fatal(err)
			}
		}
		account := newLiveTable("jaccount", "id", "status", "deleted_at")
		flag := newLiveTable("flag", "id")
		note := newLiveTable("note", "id")
		u := account.As("u")
		f := flag.As("f")

		// 多表更新：MySQL 把 JOIN 写在 SET 前。
		sqlText, args, err := dsl.Update(u).Set(u.Field("status"), "vip").
			InnerJoin(f).On(gooq.Raw("1 = 1")).
			ToSql(gooq.DialectMySQL)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sqlText, "`u` INNER JOIN `flag` AS `f` ON 1 = 1 SET `status` = ?") {
			t.Fatalf("multi update sql = %s", sqlText)
		}
		if _, err = view.Exec(ctx, sqlText, args...); err != nil {
			t.Fatal(err)
		}
		var statuses []string
		if err = view.Column(ctx, &statuses, `SELECT status FROM jaccount ORDER BY id`); err != nil {
			t.Fatal(err)
		}
		if len(statuses) != 2 || statuses[0] != "vip" || statuses[1] != "vip" {
			t.Fatalf("statuses = %#v", statuses)
		}

		// 外连接更新在 MySQL 保留 LEFT JOIN，并能执行。
		sqlText, args, err = dsl.Update(u).Set(u.Field("status"), "left").
			LeftJoin(f).On(f.Field("id").EqExpr(u.Field("id"))).
			ToSql(gooq.DialectMySQL)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sqlText, "LEFT JOIN") {
			t.Fatalf("left update sql = %s", sqlText)
		}
		if _, err = view.Exec(ctx, sqlText, args...); err != nil {
			t.Fatal(err)
		}

		// 带连接的删除：DELETE target FROM ... JOIN。
		target := note.As("u")
		joined := flag.As("f")
		sqlText, args, err = dsl.Delete(target).InnerJoin(joined).On(joined.Field("id").EqExpr(target.Field("id"))).ToSql(gooq.DialectMySQL)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sqlText, "DELETE `u` FROM `note` AS `u` INNER JOIN `flag` AS `f`") {
			t.Fatalf("joined delete sql = %s", sqlText)
		}
		if _, err = view.Exec(ctx, sqlText, args...); err != nil {
			t.Fatal(err)
		}
		var remaining int64
		if err = view.Value(ctx, &remaining, `SELECT COUNT(*) FROM note`); err != nil {
			t.Fatal(err)
		}
		if remaining != 1 {
			t.Fatalf("remaining after joined delete = %d", remaining)
		}

		// 软删表带外连接的删除：MySQL 走 UPDATE de ated_at，保留 LEFT JOIN。
		if _, _, err = dsl.Delete(u).LeftJoin(f).On(f.Field("id").EqExpr(u.Field("id"))).ToSql(gooq.DialectMySQL); err != nil {
			t.Fatalf("soft delete left join should render on mysql: %v", err)
		}
	})

	t.Run("transaction", func(t *testing.T) {
		if _, err = view.Exec(ctx, `CREATE TABLE txitem (id INT PRIMARY KEY, name VARCHAR(64))`); err != nil {
			t.Fatal(err)
		}
		txitem := newLiveTable("txitem", "id", "name")

		// 回滚：dsl 写入走 ctx 事务，fn 返回错误后行不存在。
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

		// 裸 DB 方法在 ctx 事务内同样加入事务。
		err = instance.Transaction(ctx, func(ctx context.Context, tx *db.Tx) error {
			if _, err := instance.Exec(ctx, "INSERT INTO txitem VALUES (2, 'ctx')"); err != nil {
				return err
			}
			return errors.New("abort")
		})
		if err == nil {
			t.Fatal("expected transaction error")
		}
		if count, err = dsl.Select(txitem.Field("id")).From(txitem).Count(ctx); err != nil || count != 0 {
			t.Fatalf("raw exec escaped transaction = %d, %v", count, err)
		}

		// 提交：dsl 在事务内看到自己的写入。
		if err = instance.Transaction(ctx, func(ctx context.Context, tx *db.Tx) error {
			if _, err := dsl.Insert(txitem).Columns(txitem.Field("id"), txitem.Field("name")).
				Values(3, "kept").Exec(ctx); err != nil {
				return err
			}
			n, err := dsl.Select(txitem.Field("id")).From(txitem).Count(ctx)
			if err != nil || n != 1 {
				t.Fatalf("count inside tx = %d, %v", n, err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if count, err = dsl.Select(txitem.Field("id")).From(txitem).Count(ctx); err != nil || count != 1 {
			t.Fatalf("committed rows = %d, %v", count, err)
		}

		// 保存点：嵌套失败只回滚嵌套段。
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
		if count, err = dsl.Select(txitem.Field("id")).From(txitem).Count(ctx); err != nil || count != 2 {
			t.Fatalf("rows after savepoint rollback = %d, %v", count, err)
		}

		// 行锁在事务内可用。
		if err = instance.Transaction(ctx, func(ctx context.Context, tx *db.Tx) error {
			row, err := dsl.Select(txitem.Field("id")).From(txitem).
				Where(txitem.Field("id").Eq(3)).LockForUpdate().Row(ctx)
			if err != nil || row == nil {
				t.Fatalf("lock for update: %v %v", row, err)
			}
			row, err = dsl.Select(txitem.Field("id")).From(txitem).
				Where(txitem.Field("id").Eq(3)).LockInShareMode().Row(ctx)
			if err != nil || row == nil {
				t.Fatalf("lock in share mode: %v %v", row, err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("types", func(t *testing.T) {
		if _, err = view.Exec(ctx, `CREATE TABLE typed (
			id INT PRIMARY KEY,
			created_at DATETIME NULL,
			amount DECIMAL(20,2),
			active TINYINT(1),
			payload JSON
		)`); err != nil {
			t.Fatal(err)
		}
		created := time.Date(2026, 10, 13, 12, 30, 45, 0, time.UTC)
		if _, err = view.Exec(ctx,
			`INSERT INTO typed VALUES (1, ?, 1234567890123456.25, 1, '{"k":1}'), (2, NULL, 0.01, 0, NULL)`,
			created); err != nil {
			t.Fatal(err)
		}
		typed := newLiveTable("typed", "id", "created_at", "amount", "active", "payload")

		var got time.Time
		if err = dsl.Select(typed.Field("created_at")).From(typed).
			Where(typed.Field("id").Eq(1)).Value(ctx, &got); err != nil {
			t.Fatal(err)
		}
		if !got.Equal(created) {
			t.Fatalf("time roundtrip = %v", got)
		}

		var times []*time.Time
		if err = dsl.Select(typed.Field("created_at")).From(typed).
			OrderAsc(typed.Field("id")).Column(ctx, &times); err != nil {
			t.Fatal(err)
		}
		if len(times) != 2 || times[0] == nil || !times[0].Equal(created) || times[1] != nil {
			t.Fatalf("nullable times = %#v", times)
		}

		var amounts []string
		if err = dsl.Select(typed.Field("amount")).From(typed).
			OrderAsc(typed.Field("id")).Column(ctx, &amounts); err != nil {
			t.Fatal(err)
		}
		if len(amounts) != 2 || amounts[0] != "1234567890123456.25" || amounts[1] != "0.01" {
			t.Fatalf("decimal = %#v", amounts)
		}

		var actives []bool
		if err = dsl.Select(typed.Field("active")).From(typed).
			OrderAsc(typed.Field("id")).Column(ctx, &actives); err != nil {
			t.Fatal(err)
		}
		if len(actives) != 2 || !actives[0] || actives[1] {
			t.Fatalf("bool = %#v", actives)
		}

		var payload string
		if err = dsl.Select(typed.Field("payload")).From(typed).
			Where(typed.Field("id").Eq(1)).Value(ctx, &payload); err != nil {
			t.Fatal(err)
		}
		if payload != `{"k": 1}` {
			t.Fatalf("json = %q", payload)
		}
	})

	t.Run("cte and union", func(t *testing.T) {
		if _, err = view.Exec(ctx, `CREATE TABLE numbers (id INT PRIMARY KEY, kind VARCHAR(8))`); err != nil {
			t.Fatal(err)
		}
		if _, err = view.Exec(ctx, `INSERT INTO numbers VALUES (1, 'a'), (2, 'b'), (3, 'a')`); err != nil {
			t.Fatal(err)
		}
		numbers := newLiveTable("numbers", "id", "kind")

		inner := dsl.Select(numbers.Field("id")).From(numbers).Where(numbers.Field("kind").Eq("a"))
		query := dsl.With("adults", inner).Fields(dsl.Cte("adults").Field("id")).From(dsl.Cte("adults")).
			OrderAsc(dsl.Cte("adults").Field("id"))
		var ids []int64
		if err = query.Column(ctx, &ids); err != nil {
			t.Fatal(err)
		}
		if len(ids) != 2 || ids[0] != 1 || ids[1] != 3 {
			t.Fatalf("cte ids = %#v", ids)
		}

		// MySQL 的集合操作全局 ORDER BY 只认结果列名，不接受表限定列（Error 1250），排序在 Go 侧做。
		union := dsl.Select(numbers.Field("id")).From(numbers).Where(numbers.Field("kind").Eq("b")).
			UnionAll(dsl.Select(numbers.Field("id")).From(numbers).Where(numbers.Field("kind").Eq("a")))
		if err = union.Column(ctx, &ids); err != nil {
			t.Fatal(err)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		if len(ids) != 3 || ids[0] != 1 || ids[1] != 2 || ids[2] != 3 {
			t.Fatalf("union ids = %#v", ids)
		}
	})

	t.Run("functions", func(t *testing.T) {
		if _, err = view.Exec(ctx, `CREATE TABLE words (id INT PRIMARY KEY, name VARCHAR(64), score DOUBLE, created_at DATETIME)`); err != nil {
			t.Fatal(err)
		}
		created := time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)
		if _, err = view.Exec(ctx, `INSERT INTO words VALUES (1, 'a', 1.235, ?), (2, NULL, 2, ?)`, created, created); err != nil {
			t.Fatal(err)
		}
		words := newLiveTable("words", "id", "name", "score", "created_at")

		queries := []gooq.Expression{
			fn.IfNull(words.Field("name"), gooq.Str("unknown")),
			fn.CurDate(),
			fn.DateAdd(words.Field("created_at"), 1, "DAY"),
			fn.DateDiff(words.Field("created_at"), fn.Now()),
			fn.DateFormat(words.Field("created_at"), "%Y-%m-%d %H:%i:%s"),
			fn.Round(words.Field("score"), 2),
			fn.GroupConcat(fn.GroupConcatOptions{Field: words.Field("name"), Separator: "a'b"}),
			fn.GroupConcat(fn.GroupConcatOptions{Field: words.Field("name"), Separator: `a\b`}),
			fn.GroupConcat(fn.GroupConcatOptions{
				Field:     words.Field("name"),
				Distinct:  true,
				OrderBy:   []gooq.Expression{words.Field("name").Desc()},
				Separator: "|",
			}),
		}
		for _, expr := range queries {
			sqlText, args, err := dsl.Select(expr).From(words).ToSql(gooq.DialectMySQL)
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
	return &liveTable{
		TableBase: gooq.NewTableBase(&gooq.TableMeta{TableName: name, Fields: metas}),
	}
}
