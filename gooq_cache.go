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
	Duration time.Duration
	Name     string
}

func (b *SelectBuilder) cacheKey(dialect Dialect) (string, error) {
	if b.cacheOption != nil && b.cacheOption.Name != "" {
		return b.cacheOption.Name, nil
	}
	sql, args, err := b.ToSql(dialect)
	if err != nil {
		return "", err
	}
	return "gooq:sql:" + hashQuery(sql, args), nil
}

func (b *SelectBuilder) pageCacheKey(dialect Dialect) (string, error) {
	rc := newRenderContext(b.ctx, dialect)
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
	for _, f := range b.fields {
		sql, args := rc.render(f)
		sb.WriteString("|f:" + sql + fmt.Sprintf("%v", args))
	}
	sb.WriteString("|args:" + fmt.Sprintf("%v", rc.args))
	return "gooq:page:" + hashString(sb.String()), nil
}

func hashQuery(sql string, args []any) string {
	return hashString(sql + fmt.Sprintf("%v", args))
}

func hashString(s string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return strconv.FormatUint(uint64(h.Sum32()), 16)
}
