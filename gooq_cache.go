package gooq

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CacheAdapter interface {
	Get(ctx context.Context, key string) (value []byte, ok bool, err error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Remove(ctx context.Context, key string) error
	Removes(ctx context.Context, keys []string) error
}

type HashCacheAdapter interface {
	HGet(ctx context.Context, key, field string) (value []byte, ok bool, err error)
	HGetAll(ctx context.Context, key string) (map[string][]byte, error)
	HSet(ctx context.Context, key, field string, value []byte, ttl time.Duration) error
	HDel(ctx context.Context, key string, fields ...string) error
}

var cacheAdapter CacheAdapter

var hashCacheAdapter HashCacheAdapter

var cacheAdapterMu sync.RWMutex

func SetCacheAdapter(adapter CacheAdapter) {
	cacheAdapterMu.Lock()
	defer cacheAdapterMu.Unlock()
	cacheAdapter = adapter
}

func GetCacheAdapter() CacheAdapter {
	cacheAdapterMu.RLock()
	defer cacheAdapterMu.RUnlock()
	return cacheAdapter
}

func SetHashCacheAdapter(adapter HashCacheAdapter) {
	cacheAdapterMu.Lock()
	defer cacheAdapterMu.Unlock()
	hashCacheAdapter = adapter
}

func GetHashCacheAdapter() HashCacheAdapter {
	cacheAdapterMu.RLock()
	defer cacheAdapterMu.RUnlock()
	return hashCacheAdapter
}

type CacheOption struct {
	Duration   time.Duration
	Name       string
	RowsField  string // 复合缓存 rows 的 hash field（空默认 "rows"）。
	CountField string // 复合缓存 count 的 hash field（空默认 "count"）。
	Force      bool   // count=0 时仍缓存（默认 false 不缓存空结果）。
}

// valid 判断 option 是否真实有效（Duration 或 Name 任一非零）。
func (o CacheOption) valid() bool {
	return o.Duration > 0 || o.Name != ""
}

// cacheKey 基于实际执行的 SQL 计算 key；kind 区分结果形状（同 SQL 下 Row/Rows/Count 的
// JSON 结构不同，混用同一 key 会反序列化冲突）。
func (b *SelectBuilder) cacheKey(kind, sql string, args []any) (string, error) {
	if b.cacheOption != nil && b.cacheOption.Name != "" {
		return b.cacheOption.Name, nil
	}
	return "gooq:sql:" + kind + ":" + hashQuery(sql, args), nil
}

// compositeCacheKey 复合查询（RowsAndCount）缓存 key：公共条件 + limit/offset。
// count 与 rows 子查询共享此 key（hash field 区分），故不含 fields（两者 fields 不同）。
func (b *SelectBuilder) compositeCacheKey(dialect Dialect) (string, error) {
	if b.pageCacheOpt != nil && b.pageCacheOpt.Name != "" {
		return b.pageCacheOpt.Name, nil
	}
	rc := newRenderContext(dialect)
	var sb strings.Builder
	sb.WriteString(b.from.TableName())
	for _, j := range b.joins {
		sb.WriteString("|j:" + j.table.TableName())
		for _, c := range j.on {
			sql, args := rc.render(c)
			sb.WriteString(":" + sql + fmt.Sprintf("%v", args))
		}
	}
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
	sb.WriteString("|args:" + fmt.Sprintf("%v", rc.args))
	sb.WriteString(fmt.Sprintf("|limit:%d|offset:%d", b.limit, b.offset))
	return "gooq:composite:" + hashString(sb.String()), nil
}

func hashQuery(sql string, args []any) string {
	return hashString(sql + fmt.Sprintf("%v", args))
}

func hashString(s string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return strconv.FormatUint(uint64(h.Sum32()), 16)
}
