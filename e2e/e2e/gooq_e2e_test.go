// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件为 gooq 端到端冒烟测试：连真实 MySQL 验证 DSL 构建 → gdb 执行的完整链路。
// 运行前提：docker 启动 MySQL（见本目录 README），然后 cd 本目录执行 go test ./...
package e2e

import (
	"context"
	"sync"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/mysql/v2"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/database/gooq"
	"github.com/gogf/gf/v2/test/gtest"
)

// UserEntity 是对应用户表的实体（生产环境由 gen dao 生成）。
type UserEntity struct {
	Id        int64      `json:"id"        orm:"id"`
	Name      string     `json:"name"      orm:"name"`
	Age       int        `json:"age"       orm:"age"`
	Status    string     `json:"status"    orm:"status"`
	DeletedAt *time.Time `json:"deletedAt" orm:"deleted_at"`
	CreatedAt *time.Time `json:"createdAt" orm:"created_at"`
}

// init 注册 MySQL 配置（端口 3307，见 docker-compose 或 README）。
func init() {
	err := gdb.AddConfigNode("default", gdb.ConfigNode{
		Host: "127.0.0.1",
		Port: "3307",
		User: "root",
		Pass: "root123",
		Name: "gooq_test",
		Type: "mysql",
	})
	if err != nil {
		panic(err)
	}
}

// createTable 创建测试表（幂等）。
func createTable(t *gtest.T, ctx context.Context) {
	db, err := gdb.Instance()
	t.AssertNil(err)
	_, err = db.Exec(ctx, `
CREATE TABLE IF NOT EXISTS user (
    id         BIGINT PRIMARY KEY AUTO_INCREMENT,
    name       VARCHAR(32)  NOT NULL,
    age        INT          NOT NULL,
    status     VARCHAR(16)  NOT NULL DEFAULT 'active',
    deleted_at DATETIME,
    created_at DATETIME
)`)
	t.AssertNil(err)
	_, err = db.Exec(ctx, `TRUNCATE TABLE user`)
	t.AssertNil(err)
}

// TestE2E_CRUD 验证 Insert/Select/Scan/Update/Delete 完整链路（含软删除）。
func TestE2E_CRUD(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		ctx := context.Background()
		createTable(t, ctx)

		// Insert：DSL 构建 → gdb 执行。
		result, err := gooq.Insert(gooq.User, map[string]any{
			"name":   "john",
			"age":    18,
			"status": "active",
		}).Ctx(ctx).Insert()
		t.AssertNil(err)
		rows, err := result.RowsAffected()
		t.AssertNil(err)
		t.Assert(rows, 1)

		// Select + Scan：类型化条件 → 实体映射。
		var users []UserEntity
		err = gooq.SelectFrom(gooq.User).Ctx(ctx).
			Where(gooq.User.Name.Eq("john")).
			Scan(&users)
		t.AssertNil(err)
		t.Assert(len(users), 1)
		t.Assert(users[0].Name, "john")
		t.Assert(users[0].Age, 18)

		// All：动态消费（map 形态）。
		records, err := gooq.SelectFrom(gooq.User).Ctx(ctx).
			Where(gooq.User.Age.Gt(10)).
			All()
		t.AssertNil(err)
		t.Assert(len(records), 1)
		t.Assert(records[0]["name"].String(), "john")

		// Update：类型化 Set。
		_, err = gooq.Update(gooq.User).Ctx(ctx).
			Set(gooq.User.Name, "john2").
			Where(gooq.User.ID.Eq(users[0].Id)).
			Update()
		t.AssertNil(err)

		// Delete：软删除（deleted_at 置位 + 查询自动过滤）。
		_, err = gooq.Delete(gooq.User).Ctx(ctx).
			Where(gooq.User.ID.Eq(users[0].Id)).
			Delete()
		t.AssertNil(err)

		// 自动过滤：查不到已删数据。
		records, err = gooq.SelectFrom(gooq.User).Ctx(ctx).All()
		t.AssertNil(err)
		t.Assert(len(records), 0)

		// Unscoped：能查到（deleted_at 已置位）。
		records, err = gooq.SelectFrom(gooq.User).Ctx(ctx).Unscoped().All()
		t.AssertNil(err)
		t.Assert(len(records), 1)
		t.Assert(records[0]["name"].String(), "john2")
	})
}

// TestE2E_PageCount 验证 Count/Page 执行与分页缓存闭环。
func TestE2E_PageCount(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		ctx := context.Background()
		createTable(t, ctx)

		// 批量插入 25 条。
		for i := 1; i <= 25; i++ {
			_, err := gooq.Insert(gooq.User, map[string]any{
				"name":   "user" + itoa(i),
				"age":    i,
				"status": "active",
			}).Ctx(ctx).Insert()
			t.AssertNil(err)
		}

		// Count + Page 无缓存。
		b := gooq.SelectFrom(gooq.User).Ctx(ctx).Where(gooq.User.Age.Gte(1))
		count, err := b.Count()
		t.AssertNil(err)
		t.Assert(count, 25)

		page1, err := b.Page(1, 10).All()
		t.AssertNil(err)
		t.Assert(len(page1), 10)
		page3, err := b.Page(3, 10).All()
		t.AssertNil(err)
		t.Assert(len(page3), 5)

		// 分页缓存闭环：注入哈希适配器后，第二次查询命中缓存（不再查库）。
		adapter := &mapHashAdapter{
			m:    make(map[string]map[string][]byte),
			gets: make(map[string]int),
			hits: make(map[string]int),
		}
		gooq.SetHashCacheAdapter(adapter)
		defer gooq.SetHashCacheAdapter(nil)

		// 第一次：查库并写缓存（HGet 未命中 → 查库 → HSet）。
		b1 := gooq.SelectFrom(gooq.User).Ctx(ctx).Where(gooq.User.Age.Gte(1)).
			PageCache(gooq.CacheOption{Duration: time.Minute})
		count, err = b1.Count()
		t.AssertNil(err)
		t.Assert(count, 25)
		// 第二次：命中缓存（HGet 命中 → 不再查库），返回值仍正确。
		count, err = b1.Count()
		t.AssertNil(err)
		t.Assert(count, 25)
		// 命中验证：第一次未命中（gets=1），第二次命中（gets=2 且 ok=true）。
		t.Assert(adapter.hitCount("count"), 1)
		t.Assert(adapter.missCount("count"), 1)
	})
}

// mapHashAdapter 是最小哈希实现（测试用，带命中计数）。
type mapHashAdapter struct {
	mu   sync.Mutex
	m    map[string]map[string][]byte
	gets map[string]int // field → HGet 总次数。
	hits map[string]int // field → HGet 命中次数。
}

// HGet 实现 HashCacheAdapter 接口。
func (a *mapHashAdapter) HGet(ctx context.Context, key, field string) ([]byte, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.gets[field]++
	container, ok := a.m[key]
	if !ok {
		return nil, false, nil
	}
	value, ok := container[field]
	if ok {
		a.hits[field]++
	}
	return value, ok, nil
}

// hitCount 返回指定 field 的命中次数。
func (a *mapHashAdapter) hitCount(field string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.hits[field]
}

// missCount 返回指定 field 的未命中次数。
func (a *mapHashAdapter) missCount(field string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.gets[field] - a.hits[field]
}

// HSet 实现 HashCacheAdapter 接口。
func (a *mapHashAdapter) HSet(ctx context.Context, key, field string, value []byte, ttl time.Duration) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.m[key] == nil {
		a.m[key] = make(map[string][]byte)
	}
	a.m[key][field] = value
	return nil
}

// HDel 实现 HashCacheAdapter 接口。
func (a *mapHashAdapter) HDel(ctx context.Context, key string, fields ...string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	container, ok := a.m[key]
	if !ok {
		return nil
	}
	for _, field := range fields {
		delete(container, field)
	}
	return nil
}

// itoa 是 int 转字符串的简易实现（避免引入 strconv 的格式化噪音）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits [16]byte
	pos := len(digits)
	for n > 0 {
		pos--
		digits[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(digits[pos:])
}
