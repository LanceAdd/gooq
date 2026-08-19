// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件实现缓存适配器接口（CacheAdapter + HashCacheAdapter）与注入机制。
// gooq 不内置任何缓存承载实现：由使用者自行实现并注入（gcache/redis/自写 map 均可）。
package gooq

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
)

// CacheAdapter 是缓存后端适配器（键值语义，[]byte 承载，序列化归 gooq 负责）。
type CacheAdapter interface {
	// Get 获取缓存值；未命中返回 ok=false。
	Get(ctx context.Context, key string) (value []byte, ok bool, err error)
	// Set 写入缓存值，ttl 为过期时长。
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	// Remove 删除单个缓存键。
	Remove(ctx context.Context, key string) error
	// Removes 批量删除缓存键。
	Removes(ctx context.Context, keys []string) error
}

// HashCacheAdapter 是哈希结构缓存适配器（分页缓存：count 与各页 data 同 key 同生命周期）。
type HashCacheAdapter interface {
	// HGet 获取哈希字段；未命中返回 ok=false。
	HGet(ctx context.Context, key, field string) (value []byte, ok bool, err error)
	// HSet 写入哈希字段；ttl 语义为刷新整个 key 的过期时间（redis field 无独立 TTL）。
	HSet(ctx context.Context, key, field string, value []byte, ttl time.Duration) error
	// HDel 删除哈希字段。
	HDel(ctx context.Context, key string, fields ...string) error
}

// cacheAdapter 是全局缓存适配器（默认 nil：未配置时缓存禁用）。
var cacheAdapter CacheAdapter

// hashCacheAdapter 是全局哈希缓存适配器（默认 nil：未配置时分页缓存禁用）。
var hashCacheAdapter HashCacheAdapter

// cacheAdapterMu 保护全局适配器替换。
var cacheAdapterMu sync.RWMutex

// SetCacheAdapter 注入全局缓存适配器（键值）。
func SetCacheAdapter(adapter CacheAdapter) {
	cacheAdapterMu.Lock()
	defer cacheAdapterMu.Unlock()
	cacheAdapter = adapter
}

// GetCacheAdapter 返回全局缓存适配器；未配置时返回 nil（调用方应跳过缓存）。
func GetCacheAdapter() CacheAdapter {
	cacheAdapterMu.RLock()
	defer cacheAdapterMu.RUnlock()
	return cacheAdapter
}

// SetHashCacheAdapter 注入全局哈希缓存适配器（分页缓存）。
func SetHashCacheAdapter(adapter HashCacheAdapter) {
	cacheAdapterMu.Lock()
	defer cacheAdapterMu.Unlock()
	hashCacheAdapter = adapter
}

// GetHashCacheAdapter 返回全局哈希缓存适配器；未配置时返回 nil（调用方应禁用分页缓存）。
func GetHashCacheAdapter() HashCacheAdapter {
	cacheAdapterMu.RLock()
	defer cacheAdapterMu.RUnlock()
	return hashCacheAdapter
}

// CacheOption 是查询缓存配置。
type CacheOption struct {
	// Duration 是缓存过期时长；0 表示不过期（由承载实现决定）。
	Duration time.Duration
	// Name 是自定义缓存键（默认使用渲染 SQL 作为键）。
	Name string
}

// cacheKey 生成查询缓存键（优先 Name，否则渲染 SQL + 参数哈希）。
// 参数值必须参与哈希：不同参数（如 age > 18 与 age > 30）渲染 SQL 相同但结果不同。
func (b *SelectBuilder) cacheKey() (string, error) {
	if b.cacheOption != nil && b.cacheOption.Name != "" {
		return b.cacheOption.Name, nil
	}
	sql, args, err := b.ToSql(DialectMySQL)
	if err != nil {
		return "", err
	}
	return "gooq:sql:" + hashQuery(sql, args), nil
}

// pageCacheKey 生成分页缓存键：字段/表/join/条件/分组/排序 + 参数值 的联合哈希。
// count 与各页 data 共用（同一 builder 两次查询的渲染要素一致）；
// 排除 LIMIT/OFFSET（count 与 data 的固有差异，不影响结果语义归属）。
func (b *SelectBuilder) pageCacheKey() (string, error) {
	rc := newRenderContext(b.ctx, b.db, DialectMySQL)
	b.registerAliases(rc)
	var sb strings.Builder
	sb.WriteString(b.from.TableName())
	for _, j := range b.joins {
		sb.WriteString("|j:" + j.table.TableName())
		for _, c := range j.on {
			sql, args := rc.render(c)
			sb.WriteString(":" + sql + fmt.Sprintf("%v", args))
		}
	}
	// WHERE 条件（含软删自动条件）与参数必须参与哈希：不同条件/不同参数的查询不得共享缓存。
	sb.WriteString("|w:" + b.renderWhere(rc) + fmt.Sprintf("%v", rc.args))
	for _, g := range b.groupBy {
		sql, args := rc.render(g)
		sb.WriteString("|g:" + sql + fmt.Sprintf("%v", args))
	}
	for _, h := range b.having {
		sql, args := rc.render(h)
		sb.WriteString("|h:" + sql + fmt.Sprintf("%v", args))
	}
	for _, o := range b.orderBy {
		sql, args := o.render(rc)
		sb.WriteString("|o:" + sql + fmt.Sprintf("%v", args))
	}
	for _, f := range b.fields {
		sql, args := rc.render(f)
		sb.WriteString("|f:" + sql + fmt.Sprintf("%v", args))
	}
	// where 渲染的参数并入 rc.args，统一写入 key。
	sb.WriteString("|args:" + fmt.Sprintf("%v", rc.args))
	return "gooq:page:" + hashString(sb.String()), nil
}

// hashQuery 将 SQL 与参数联合哈希（参数值必须参与，避免同 SQL 不同参数串缓存）。
func hashQuery(sql string, args []any) string {
	return hashString(sql + fmt.Sprintf("%v", args))
}

// hashString 返回字符串的 FNV-32a 哈希（分页缓存键用）。
func hashString(s string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return strconv.FormatUint(uint64(h.Sum32()), 16)
}

// marshalResult 序列化查询结果（JSON，[]byte 承载）。
func marshalResult(result Result) ([]byte, error) {
	return json.Marshal(result)
}

// unmarshalResult 反序列化查询结果（数字类型统一还原为 float64，原型可接受）。
func unmarshalResult(value []byte) (Result, error) {
	var list []map[string]any
	if err := json.Unmarshal(value, &list); err != nil {
		return nil, err
	}
	result := make(Result, len(list))
	for i, m := range list {
		record := make(Record, len(m))
		for k, v := range m {
			record[k] = gvar.New(v)
		}
		result[i] = record
	}
	return result, nil
}
