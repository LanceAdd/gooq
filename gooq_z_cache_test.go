// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件为缓存 key 生成逻辑单元测试（内部函数 cacheKey/pageCacheKey）。
// 表对象用最小 TableBase 构造：key 逻辑与表来源无关，测试聚焦哈希区分度。
package gooq

import (
	"testing"

	"github.com/gogf/gf/v2/test/gtest"
)

// cacheTestUser 是缓存 key 测试用最小表对象。
var cacheTestUser = NewTableBase(&TableMeta{
	TableName: "user",
	Fields: []FieldMeta{
		{ColumnName: "id", LocalType: LocalTypeInt64, Primary: true},
		{ColumnName: "name", LocalType: LocalTypeString},
		{ColumnName: "level", LocalType: LocalTypeInt},
		{ColumnName: "deleted_at", LocalType: LocalTypeDatetime, SoftDelete: true},
	},
})

// TestDsl_CacheKey 验证单查询缓存 key：不同参数不同 key，自定义 Name 优先。
func TestDsl_CacheKey(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		userID := NewField[int64]("user", "id")
		userLevel := NewField[int]("user", "level")

		b1 := SelectFrom(cacheTestUser).Where(userLevel.Eq(1)).Cache(CacheOption{})
		b2 := SelectFrom(cacheTestUser).Where(userLevel.Eq(2)).Cache(CacheOption{})
		k1, err := b1.cacheKey(DialectMySQL)
		t.AssertNil(err)
		k2, err := b2.cacheKey(DialectMySQL)
		t.AssertNil(err)
		t.AssertNE(k1, k2) // 同 SQL 不同参数：key 不同。

		b3 := SelectFrom(cacheTestUser).Where(userLevel.Eq(1)).Cache(CacheOption{Name: "users:admins"})
		k3, err := b3.cacheKey(DialectMySQL)
		t.AssertNil(err)
		t.Assert(k3, "users:admins") // Name 优先。

		b4 := SelectFrom(cacheTestUser).Where(userLevel.Eq(1)).Cache(CacheOption{})
		k4, err := b4.cacheKey(DialectMySQL)
		t.AssertNil(err)
		t.Assert(k1, k4) // 同参数同 SQL：key 相同（可命中）。

		// 方言参与 key：MySQL 与 PG 渲染不同，key 不同。
		k5, err := b1.cacheKey(DialectPgsql)
		t.AssertNil(err)
		t.AssertNE(k1, k5)
		_ = userID
	})
}

// TestDsl_PageCacheKey 验证分页缓存 key：参数、排序、字段均参与哈希。
func TestDsl_PageCacheKey(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		userID := NewField[int64]("user", "id")
		userName := NewField[string]("user", "name")
		userLevel := NewField[int]("user", "level")

		// 同一条件同一排序：key 相同（count 与 data 共享）。
		b1 := SelectFrom(cacheTestUser).Where(userLevel.Eq(1)).Order(userID.Desc()).
			PageCache(CacheOption{}).Page(1, 10)
		b2 := SelectFrom(cacheTestUser).Where(userLevel.Eq(1)).Order(userID.Desc()).
			PageCache(CacheOption{}).Page(2, 10)
		k1, err := b1.pageCacheKey(DialectMySQL)
		t.AssertNil(err)
		k2, err := b2.pageCacheKey(DialectMySQL)
		t.AssertNil(err)
		t.Assert(k1, k2) // 不同页（LIMIT/OFFSET 差异）同 key。

		// 不同参数：key 不同。
		b3 := SelectFrom(cacheTestUser).Where(userLevel.Eq(2)).Order(userID.Desc()).
			PageCache(CacheOption{}).Page(1, 10)
		k3, err := b3.pageCacheKey(DialectMySQL)
		t.AssertNil(err)
		t.AssertNE(k1, k3)

		// 不同排序：key 不同。
		b4 := SelectFrom(cacheTestUser).Where(userLevel.Eq(1)).Order(userName.Asc()).
			PageCache(CacheOption{}).Page(1, 10)
		k4, err := b4.pageCacheKey(DialectMySQL)
		t.AssertNil(err)
		t.AssertNE(k1, k4)

		// 不同字段：key 不同。
		b5 := Select(userID, userName).From(cacheTestUser).Where(userLevel.Eq(1)).Order(userID.Desc()).
			PageCache(CacheOption{}).Page(1, 10)
		k5, err := b5.pageCacheKey(DialectMySQL)
		t.AssertNil(err)
		t.AssertNE(k1, k5)
	})
}
