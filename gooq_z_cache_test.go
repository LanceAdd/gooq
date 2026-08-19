// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件为缓存适配器（CacheAdapter/HashCacheAdapter）的单元测试。
// 测试使用最小 map 实现（用户自写实现的典型形态），验证接口契约。
package gooq

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gogf/gf/v2/test/gtest"
)

// mapCacheAdapter 是最小键值实现（测试用）。
type mapCacheAdapter struct {
	mu sync.Mutex
	m  map[string][]byte
}

// Get 实现 CacheAdapter 接口。
func (a *mapCacheAdapter) Get(ctx context.Context, key string) ([]byte, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	value, ok := a.m[key]
	return value, ok, nil
}

// Set 实现 CacheAdapter 接口。
func (a *mapCacheAdapter) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.m[key] = value
	return nil
}

// Remove 实现 CacheAdapter 接口。
func (a *mapCacheAdapter) Remove(ctx context.Context, key string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.m, key)
	return nil
}

// Removes 实现 CacheAdapter 接口。
func (a *mapCacheAdapter) Removes(ctx context.Context, keys []string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, key := range keys {
		delete(a.m, key)
	}
	return nil
}

// mapHashCacheAdapter 是最小哈希实现（map 容器模拟，测试用）。
type mapHashCacheAdapter struct {
	mu sync.Mutex
	m  map[string]map[string][]byte
}

// HGet 实现 HashCacheAdapter 接口。
func (a *mapHashCacheAdapter) HGet(ctx context.Context, key, field string) ([]byte, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	container, ok := a.m[key]
	if !ok {
		return nil, false, nil
	}
	value, ok := container[field]
	return value, ok, nil
}

// HSet 实现 HashCacheAdapter 接口（ttl 语义由实现决定，测试实现忽略过期）。
func (a *mapHashCacheAdapter) HSet(ctx context.Context, key, field string, value []byte, ttl time.Duration) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.m[key] == nil {
		a.m[key] = make(map[string][]byte)
	}
	a.m[key][field] = value
	return nil
}

// HDel 实现 HashCacheAdapter 接口。
func (a *mapHashCacheAdapter) HDel(ctx context.Context, key string, fields ...string) error {
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

// TestCacheAdapter_Basic 验证键值接口的 Get/Set/Remove/Removes 契约。
func TestCacheAdapter_Basic(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		ctx := context.Background()
		adapter := &mapCacheAdapter{m: make(map[string][]byte)}

		// 未命中。
		_, ok, err := adapter.Get(ctx, "not-exist")
		t.AssertNil(err)
		t.Assert(ok, false)

		// Set + Get。
		err = adapter.Set(ctx, "k1", []byte("v1"), time.Minute)
		t.AssertNil(err)
		value, ok, err := adapter.Get(ctx, "k1")
		t.AssertNil(err)
		t.Assert(ok, true)
		t.Assert(string(value), "v1")

		// Removes 批量删除。
		t.AssertNil(adapter.Set(ctx, "k2", []byte("v2"), time.Minute))
		t.AssertNil(adapter.Removes(ctx, []string{"k1", "k2"}))
		_, ok, err = adapter.Get(ctx, "k1")
		t.AssertNil(err)
		t.Assert(ok, false)
		_, ok, err = adapter.Get(ctx, "k2")
		t.AssertNil(err)
		t.Assert(ok, false)
	})
}

// TestHashCacheAdapter_Basic 验证哈希接口的 HGet/HSet/HDel 契约（同 key 多字段）。
func TestHashCacheAdapter_Basic(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		ctx := context.Background()
		adapter := &mapHashCacheAdapter{m: make(map[string]map[string][]byte)}

		// 未命中。
		_, ok, err := adapter.HGet(ctx, "page:user", "count")
		t.AssertNil(err)
		t.Assert(ok, false)

		// HSet + HGet（count 与各页 data 同 key 同生命周期）。
		t.AssertNil(adapter.HSet(ctx, "page:user", "count", []byte("100"), time.Minute))
		t.AssertNil(adapter.HSet(ctx, "page:user", "data:1:10", []byte("data1"), time.Minute))
		count, ok, err := adapter.HGet(ctx, "page:user", "count")
		t.AssertNil(err)
		t.Assert(ok, true)
		t.Assert(string(count), "100")
		data, ok, err := adapter.HGet(ctx, "page:user", "data:1:10")
		t.AssertNil(err)
		t.Assert(ok, true)
		t.Assert(string(data), "data1")

		// HDel 单字段：count 删除后 data 仍在。
		t.AssertNil(adapter.HDel(ctx, "page:user", "count"))
		_, ok, err = adapter.HGet(ctx, "page:user", "count")
		t.AssertNil(err)
		t.Assert(ok, false)
		_, ok, err = adapter.HGet(ctx, "page:user", "data:1:10")
		t.AssertNil(err)
		t.Assert(ok, true)
	})
}

// TestDsl_PageCacheKey 验证分页缓存 key 的区分度：参数、排序、字段均参与哈希。
func TestDsl_PageCacheKey(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		// 同一条件同一排序：key 相同（count 与 data 共享）。
		b1 := SelectFrom(User).Where(User.Age.Gt(18)).Order(User.ID.Desc()).
			PageCache(CacheOption{}).Page(1, 10)
		b2 := SelectFrom(User).Where(User.Age.Gt(18)).Order(User.ID.Desc()).
			PageCache(CacheOption{}).Page(2, 10)
		k1, err := b1.pageCacheKey()
		t.AssertNil(err)
		k2, err := b2.pageCacheKey()
		t.AssertNil(err)
		t.Assert(k1, k2) // 不同页（LIMIT/OFFSET 差异）同 key。

		// 不同参数：key 不同（避免 age>18 与 age>30 串缓存）。
		b3 := SelectFrom(User).Where(User.Age.Gt(30)).Order(User.ID.Desc()).
			PageCache(CacheOption{}).Page(1, 10)
		k3, err := b3.pageCacheKey()
		t.AssertNil(err)
		t.AssertNE(k1, k3)

		// 不同排序：key 不同（避免排序变化串页）。
		b4 := SelectFrom(User).Where(User.Age.Gt(18)).Order(User.Name.Asc()).
			PageCache(CacheOption{}).Page(1, 10)
		k4, err := b4.pageCacheKey()
		t.AssertNil(err)
		t.AssertNE(k1, k4)

		// 不同字段：key 不同。
		b5 := Select(User.ID, User.Name).From(User).Where(User.Age.Gt(18)).Order(User.ID.Desc()).
			PageCache(CacheOption{}).Page(1, 10)
		k5, err := b5.pageCacheKey()
		t.AssertNil(err)
		t.AssertNE(k1, k5)
	})
}

// TestDsl_CacheKey 验证单查询缓存 key：不同参数不同 key，自定义 Name 优先。
func TestDsl_CacheKey(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		b1 := SelectFrom(User).Where(User.Age.Gt(18)).Cache(CacheOption{})
		b2 := SelectFrom(User).Where(User.Age.Gt(30)).Cache(CacheOption{})
		k1, err := b1.cacheKey()
		t.AssertNil(err)
		k2, err := b2.cacheKey()
		t.AssertNil(err)
		t.AssertNE(k1, k2) // 同 SQL 不同参数：key 不同。

		b3 := SelectFrom(User).Where(User.Age.Gt(18)).Cache(CacheOption{Name: "users:adults"})
		k3, err := b3.cacheKey()
		t.AssertNil(err)
		t.Assert(k3, "users:adults") // Name 优先。
	})
}

// TestHashCacheAdapter_Concurrent 验证并发写入安全（实现需自带锁）。
func TestHashCacheAdapter_Concurrent(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		ctx := context.Background()
		adapter := &mapHashCacheAdapter{m: make(map[string]map[string][]byte)}
		done := make(chan struct{})
		for i := 0; i < 20; i++ {
			go func(n int) {
				field := string(rune('a' + n))
				for j := 0; j < 50; j++ {
					_ = adapter.HSet(ctx, "hash", field, []byte{byte(n)}, time.Minute)
				}
				done <- struct{}{}
			}(i)
		}
		for i := 0; i < 20; i++ {
			<-done
		}
		value, ok, err := adapter.HGet(ctx, "hash", "a")
		t.AssertNil(err)
		t.Assert(ok, true)
		t.Assert(len(value), 1)
	})
}
