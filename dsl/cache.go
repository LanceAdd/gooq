package dsl

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lanceadd/gooq"
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

// compositeCacheKey 是 PageCache 的 key。总数 SQL 与当前页 SQL 连同参数一起哈希，
// count 与 rows 存在这一把 key 的两个 field 里。选择列、条件、排序或页码不同则 key 不同。
func (b *SelectBuilder) compositeCacheKey(dialect gooq.Dialect) (string, error) {
	if b.pageCacheOpt != nil && b.pageCacheOpt.Name != "" {
		return b.pageCacheOpt.Name, nil
	}
	countSQL, countArgs, err := b.countSQL(dialect)
	if err != nil {
		return "", err
	}
	rowsSQL, rowsArgs, err := b.ToSql(dialect)
	if err != nil {
		return "", err
	}
	return "gooq:composite:" + hashString(encodeQuery(countSQL, countArgs)+encodeQuery(rowsSQL, rowsArgs)), nil
}

// encodeQuery 用长度前缀拼出 SQL 和参数，避免两段文本粘连后发生碰撞。
func encodeQuery(sql string, args []any) string {
	encodedArgs := encodeArgs(args)
	return strconv.Itoa(len(sql)) + ":" + sql + strconv.Itoa(len(encodedArgs)) + ":" + encodedArgs
}

// encodeArgs 按「类型名 + 稳定文本」编码每个参数。int64(1) 与 "1" 得到不同的 key。
func encodeArgs(args []any) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = fmt.Sprintf("%T:%v", arg, arg)
	}
	return strings.Join(parts, ",")
}

func hashQuery(sql string, args []any) string {
	return hashString(sql + encodeArgs(args))
}

func hashString(s string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return strconv.FormatUint(uint64(h.Sum32()), 16)
}
