package dsl

import (
	"context"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

// rowWithCache 单行缓存：db.Record 空结果不缓存（与 count=0 空结果策略一致）。
func (b *SelectBuilder) rowWithCache(
	ctx context.Context, adapter CacheAdapter, dialect gooq.Dialect, sql string, args []any,
) (db.Record, error) {
	key, err := b.cacheKey("row", sql, args)
	if err != nil {
		return nil, err
	}
	codec := GetCacheCodec()
	if payload, ok, err := adapter.Get(ctx, key); err == nil && ok {
		var record db.Record
		if err := codec.UnmarshalRecord(payload, &record); err != nil {
			return nil, err
		}
		return record, nil
	}
	record, err := b.executor.GetOne(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	if record == nil && !b.cacheOption.Force {
		return nil, nil
	}
	if payload, err := codec.Marshal(record); err == nil {
		_ = adapter.Set(ctx, key, payload, b.cacheOption.Duration)
	}
	return record, nil
}

// Row 返回一行；没有行返回 nil, nil。
// 没有 Limit 时本次执行补 LIMIT 1，保留 Offset，查询链不变。
// Limit 大于 1 时返回错误；缓存 key 按这次实际执行的语句计算。
func (b *SelectBuilder) Row(ctx context.Context) (db.Record, error) {
	if b.limit > 1 {
		return nil, errManyRows(b.limit)
	}
	restore, err := b.bind(ctx)
	if err != nil {
		return nil, err
	}
	defer restore()
	dialect := b.dialect()
	sql, args, err := b.oneRowSQL(dialect)
	if err != nil {
		return nil, err
	}
	if b.pageCacheOpt != nil {
		return nil, errPageCache()
	}
	if b.cacheOption != nil {
		if adapter := GetCacheAdapter(); adapter != nil {
			return b.rowWithCache(ctx, adapter, dialect, sql, args)
		}
	}
	record, err := b.executor.GetOne(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, nil
	}
	return record, nil
}

// rowsWithCache 多行缓存：空结果不缓存。
func (b *SelectBuilder) rowsWithCache(
	ctx context.Context, adapter CacheAdapter, dialect gooq.Dialect, sql string, args []any,
) (db.Result, error) {
	key, err := b.cacheKey("rows", sql, args)
	if err != nil {
		return nil, err
	}
	codec := GetCacheCodec()
	if payload, ok, err := adapter.Get(ctx, key); err == nil && ok {
		var rows db.Result
		if err := codec.UnmarshalRecord(payload, &rows); err != nil {
			return nil, err
		}
		return rows, nil
	}
	rows, err := b.rowsValue(ctx, dialect)
	if err != nil {
		return nil, err
	}
	if rows == nil && !b.cacheOption.Force {
		return nil, nil
	}
	if payload, err := codec.Marshal(rows); err == nil {
		_ = adapter.Set(ctx, key, payload, b.cacheOption.Duration)
	}
	return rows, nil
}

func (b *SelectBuilder) Rows(ctx context.Context) (db.Result, error) {
	restore, err := b.bind(ctx)
	if err != nil {
		return nil, err
	}
	defer restore()
	dialect := b.dialect()
	sql, args, err := b.ToSql(dialect)
	if err != nil {
		return nil, err
	}
	if b.pageCacheOpt != nil {
		return nil, errPageCache()
	}
	if b.cacheOption != nil {
		if adapter := GetCacheAdapter(); adapter != nil {
			return b.rowsWithCache(ctx, adapter, dialect, sql, args)
		}
	}
	return b.rowsValue(ctx, dialect)
}

// rowsValue 执行当前查询并返回全部行（Rows/RowsAndCount 共用）。
func (b *SelectBuilder) rowsValue(ctx context.Context, dialect gooq.Dialect) (db.Result, error) {
	sql, args, err := b.ToSql(dialect)
	if err != nil {
		return nil, err
	}
	result, err := b.executor.GetAll(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return result, nil
}
