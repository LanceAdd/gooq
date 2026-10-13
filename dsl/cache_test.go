// 本文件为缓存 key 生成逻辑单元测试（内部函数 cacheKey/pageCacheKey）。
// 表对象用最小 gooq.TableBase 构造：key 逻辑与表来源无关，测试聚焦哈希区分度。
package dsl

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/test/gtest"
	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

// cacheTestUser 是缓存 key 测试用最小表对象。
var cacheTestUser = gooq.NewTableBase(&gooq.TableMeta{
	TableName: "user",
	Fields: []gooq.FieldMeta{
		{ColumnName: "id", LocalType: gooq.LocalTypeInt64, Primary: true},
		{ColumnName: "name", LocalType: gooq.LocalTypeString},
		{ColumnName: "level", LocalType: gooq.LocalTypeInt},
		{ColumnName: "deleted_at", LocalType: gooq.LocalTypeDatetime, SoftDelete: true},
	},
})

// TestDsl_CacheKey 验证单查询缓存 key：不同参数不同 key，自定义 Name 优先。
func TestDsl_CacheKey(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		userId := gooq.NewField[int64]("user", "id")
		userLevel := gooq.NewField[int]("user", "level")

		b1 := SelectFrom(cacheTestUser).Where(userLevel.Eq(1)).Cache(CacheOption{})
		b2 := SelectFrom(cacheTestUser).Where(userLevel.Eq(2)).Cache(CacheOption{})
		sql1, args1, err := b1.ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		sql2, args2, err := b2.ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		k1, err := b1.cacheKey("scan", sql1, args1)
		t.AssertNil(err)
		k2, err := b2.cacheKey("scan", sql2, args2)
		t.AssertNil(err)
		t.AssertNE(k1, k2) // 同 SQL 不同参数：key 不同。

		b3 := SelectFrom(cacheTestUser).Where(userLevel.Eq(1)).Cache(CacheOption{Name: "users:admins"})
		k3, err := b3.cacheKey("scan", sql1, args1)
		t.AssertNil(err)
		t.Assert(k3, "users:admins") // Name 优先。

		b4 := SelectFrom(cacheTestUser).Where(userLevel.Eq(1)).Cache(CacheOption{})
		k4, err := b4.cacheKey("scan", sql1, args1)
		t.AssertNil(err)
		t.Assert(k1, k4) // 同参数同 SQL：key 相同（可命中）。

		// 方言参与 key：MySQL 与 PG 渲染不同，key 不同。
		sqlPg, argsPg, err := b1.ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		k5, err := b1.cacheKey("scan", sqlPg, argsPg)
		t.AssertNil(err)
		t.AssertNE(k1, k5)

		kInt, err := b1.cacheKey("value", "SELECT ?", []any{int64(1)})
		t.AssertNil(err)
		kText, err := b1.cacheKey("value", "SELECT ?", []any{"1"})
		t.AssertNil(err)
		t.AssertNE(kInt, kText)
		t.Assert(kInt, "gooq:query:value:"+hashString(encodeQuery("SELECT ?", []any{int64(1)})))
		t.Assert(len(kInt), len("gooq:query:value:")+64)

		glued, err := b1.cacheKey("value", "SELECT ?", []any{"a,int:1"})
		t.AssertNil(err)
		split, err := b1.cacheKey("value", "SELECT ?", []any{"a", 1})
		t.AssertNil(err)
		t.AssertNE(glued, split)

		sqlOnly, err := b1.cacheKey("value", "SELECT ?int:1", nil)
		t.AssertNil(err)
		withArg, err := b1.cacheKey("value", "SELECT ?", []any{1})
		t.AssertNil(err)
		t.AssertNE(sqlOnly, withArg)
		_ = userId
	})
}

// TestDsl_CompositeCacheKey 验证 PageCache key：总数 SQL 与当前页 SQL 一起哈希。
func TestDsl_CompositeCacheKey(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		userId := gooq.NewField[int64]("user", "id")
		userName := gooq.NewField[string]("user", "name")
		userLevel := gooq.NewField[int]("user", "level")

		// 同一条件同一排序：不同页（LIMIT/OFFSET 差异）key 不同。
		b1 := SelectFrom(cacheTestUser).Where(userLevel.Eq(1)).Order(userId.Desc()).Page(1, 10)
		b2 := SelectFrom(cacheTestUser).Where(userLevel.Eq(1)).Order(userId.Desc()).Page(2, 10)
		k1, err := b1.compositeCacheKey(gooq.DialectMySQL, "rows")
		t.AssertNil(err)
		k2, err := b2.compositeCacheKey(gooq.DialectMySQL, "rows")
		t.AssertNil(err)
		t.AssertNE(k1, k2)

		// 不同参数：key 不同。
		b3 := SelectFrom(cacheTestUser).Where(userLevel.Eq(2)).Order(userId.Desc()).Page(1, 10)
		k3, err := b3.compositeCacheKey(gooq.DialectMySQL, "rows")
		t.AssertNil(err)
		t.AssertNE(k1, k3)

		// 不同排序：key 不同。
		b4 := SelectFrom(cacheTestUser).Where(userLevel.Eq(1)).Order(userName.Asc()).Page(1, 10)
		k4, err := b4.compositeCacheKey(gooq.DialectMySQL, "rows")
		t.AssertNil(err)
		t.AssertNE(k1, k4)

		// 不同选择列：key 不同。同一条查询重复计算则 key 相同。
		b5 := Select(userId, userName).From(cacheTestUser).Where(userLevel.Eq(1)).Order(userId.Desc()).Page(1, 10)
		k5, err := b5.compositeCacheKey(gooq.DialectMySQL, "rows")
		t.AssertNil(err)
		t.AssertNE(k1, k5)
		b6 := Select(userId, userName).From(cacheTestUser).Where(userLevel.Eq(1)).Order(userId.Desc()).Page(1, 10)
		k6, err := b6.compositeCacheKey(gooq.DialectMySQL, "rows")
		t.AssertNil(err)
		t.Assert(k5, k6)

		// 方言改变 SQL 文本，key 不同。自定义 Name 优先。
		kPg, err := b1.compositeCacheKey(gooq.DialectPgsql, "rows")
		t.AssertNil(err)
		t.AssertNE(k1, kPg)
		named := SelectFrom(cacheTestUser).PageCache(CacheOption{Name: "page:users"})
		kNamed, err := named.compositeCacheKey(gooq.DialectMySQL, "rows")
		t.AssertNil(err)
		t.Assert(kNamed, "page:users")

		kCol, err := b1.compositeCacheKey(gooq.DialectMySQL, "column")
		t.AssertNil(err)
		t.AssertNE(k1, kCol)
		t.Assert(strings.HasPrefix(k1, "gooq:multi-query:rows:"), true)
		t.Assert(strings.HasPrefix(kCol, "gooq:multi-query:column:"), true)
		t.Assert(len(k1), len("gooq:multi-query:rows:")+64)
	})
}

func TestCacheRecordKeepsLargeInt(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		const id int64 = 9007199254740993
		idField := gooq.NewField[int64]("user", "id")
		raw, err := json.Marshal(db.Record{"id": id})
		t.AssertNil(err)
		var row db.Record
		t.AssertNil(unmarshalUseNumber(raw, &row))
		got, err := db.Value(row, idField)
		t.AssertNil(err)
		t.Assert(got, id)

		list, err := json.Marshal(db.Result{{"id": id}})
		t.AssertNil(err)
		var rows db.Result
		t.AssertNil(unmarshalUseNumber(list, &rows))
		t.Assert(len(rows), 1)
		got, err = db.Value(rows[0], idField)
		t.AssertNil(err)
		t.Assert(got, id)
	})
}
