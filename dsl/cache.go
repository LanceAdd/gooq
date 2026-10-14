package dsl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
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

// CacheCodec 编码缓存值。整个进程共用一个实现，未设置时使用 JSON。
// Unmarshal 解进具体类型。UnmarshalRecord 解进行记录，JSON 实现使用 UseNumber。
type CacheCodec interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, dest any) error
	UnmarshalRecord(data []byte, dest any) error
}

var cacheCodec CacheCodec

// SetCacheCodec 设置缓存值的编码方式。nil 恢复为 JSON。
func SetCacheCodec(codec CacheCodec) {
	cacheAdapterMu.Lock()
	defer cacheAdapterMu.Unlock()
	cacheCodec = codec
}

// GetCacheCodec 返回当前编码方式。未设置时是 JSON。
func GetCacheCodec() CacheCodec {
	cacheAdapterMu.RLock()
	defer cacheAdapterMu.RUnlock()
	if cacheCodec == nil {
		return jsonCacheCodec{}
	}
	return cacheCodec
}

type jsonCacheCodec struct{}

func (jsonCacheCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (jsonCacheCodec) Unmarshal(data []byte, dest any) error {
	return json.Unmarshal(data, dest)
}

func (jsonCacheCodec) UnmarshalRecord(data []byte, dest any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode(dest)
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

// cacheKey 基于实际执行的 SQL 计算 key。组名隔离不同配置组。kind 区分结果形状，
// 同 SQL 下 Row/Rows/Count 的 JSON 结构不同，混用同一 key 会反序列化冲突。
func (b *SelectBuilder) cacheKey(kind, sql string, args []any) (string, error) {
	if b.cacheOption != nil && b.cacheOption.Name != "" {
		return b.cacheOption.Name, nil
	}
	return "gooq:query:" + b.cacheGroup() + ":" + kind + ":" + hashQuery(sql, args), nil
}

// compositeCacheKey 是 PageCache 的 key。kind 区分数据 field 的形状：rows 是 db.Result，column 是列切片。
// 总数 SQL 与当前页 SQL 连同参数一起哈希，count 与 rows 存在这一把 key 的两个 field 里。
// 选择列、条件、排序或页码不同则 key 不同。
func (b *SelectBuilder) compositeCacheKey(dialect gooq.Dialect, kind string) (string, error) {
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
	payload := encodeQuery(countSQL, countArgs) + encodeQuery(rowsSQL, rowsArgs)
	return "gooq:multi-query:" + b.cacheGroup() + ":" + kind + ":" + hashString(payload), nil
}

// cacheGroup 返回这次执行使用的配置组。已绑定的 DB 或事务优先。
// 省略 DB() 时用当前默认组，不把空字符串写进 key。
func (b *SelectBuilder) cacheGroup() string {
	switch typed := b.executor.(type) {
	case *db.DB:
		if group := typed.Group(); group != "" {
			return group
		}
	case *db.Tx:
		if database := typed.DB(); database != nil {
			if group := database.Group(); group != "" {
				return group
			}
		}
	}
	if b.groupSet && b.group != "" {
		return b.group
	}
	return db.GetDefaultGroup()
}

// encodeQuery 用长度前缀拼出 SQL 和参数，避免两段文本粘连后发生碰撞。
func encodeQuery(sql string, args []any) string {
	encodedArgs := encodeArgs(args)
	return strconv.Itoa(len(sql)) + ":" + sql + strconv.Itoa(len(encodedArgs)) + ":" + encodedArgs
}

// encodeArgs 按长度前缀编码每个参数。int64(1) 与 "1" 得到不同的 key，
// 参数文本里的逗号也不会和相邻参数粘成同一段。
func encodeArgs(args []any) string {
	var b strings.Builder
	for _, arg := range args {
		typeName := fmt.Sprintf("%T", arg)
		text := fmt.Sprintf("%v", arg)
		b.WriteString(strconv.Itoa(len(typeName)))
		b.WriteByte(':')
		b.WriteString(typeName)
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(len(text)))
		b.WriteByte(':')
		b.WriteString(text)
	}
	return b.String()
}

func hashQuery(sql string, args []any) string {
	return hashString(encodeQuery(sql, args))
}

func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
