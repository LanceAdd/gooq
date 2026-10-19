package dsl

import (
	"context"
	"fmt"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

// RowsAndCount 复合查询：count 先行，count=0 短路不查 rows；
// PageCache 开启时走 hash 缓存（count/rows 独立 field，部分命中独立处理）。
func (b *SelectBuilder) RowsAndCount(ctx context.Context) (db.Result, int64, error) {
	restore, err := b.bind(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer restore()
	dialect := b.dialect()
	if b.pageCacheOpt != nil {
		return b.rowsAndCountWithCache(ctx, dialect)
	}
	count, err := b.countValue(ctx, dialect)
	if err != nil {
		return nil, 0, err
	}
	if count == 0 {
		return nil, 0, nil
	}
	rows, err := b.rowsValue(ctx, dialect)
	if err != nil {
		return nil, 0, err
	}
	return rows, count, nil
}

// cacheFieldNames 解析复合缓存 field 名：数据 field 默认 "rows"，count 默认 "count"。
func (b *SelectBuilder) cacheFieldNames() (dataField, countField string) {
	dataField = b.pageCacheOpt.RowsField
	if dataField == "" {
		dataField = "rows"
	}
	countField = b.pageCacheOpt.CountField
	if countField == "" {
		countField = "count"
	}
	return
}

// compositeCacheBegin 复合缓存入口：校验 HashCacheAdapter、计算 key、一次取出整个 hash 记录。
func (b *SelectBuilder) compositeCacheBegin(ctx context.Context, dialect gooq.Dialect, kind string) (HashCacheAdapter, string, map[string][]byte, error) {
	adapter := GetHashCacheAdapter()
	if adapter == nil {
		return nil, "", nil, fmt.Errorf("gooq: PageCache requires HashCacheAdapter, use SetHashCacheAdapter")
	}
	key, err := b.compositeCacheKey(dialect, kind)
	if err != nil {
		return nil, "", nil, err
	}
	fields, err := adapter.HGetAll(ctx, key)
	if err != nil {
		fields = nil
	}
	return adapter, key, fields, nil
}

// resolveCompositeCount 处理复合缓存 count field：命中反序列化；未命中查询回填
// （count=0 且非 Force 不缓存并短路）。short 为 true 时调用方直接返回空结果。
func (b *SelectBuilder) resolveCompositeCount(
	ctx context.Context, dialect gooq.Dialect, adapter HashCacheAdapter, key, countField string, fields map[string][]byte,
) (count int64, short bool, err error) {
	codec := GetCacheCodec()
	if payload, ok := fields[countField]; ok {
		if err := codec.Unmarshal(payload, &count); err != nil {
			return 0, false, err
		}
		return count, count == 0, nil
	}
	count, err = b.countValue(ctx, dialect)
	if err != nil {
		return 0, false, err
	}
	if count == 0 && !b.pageCacheOpt.Force {
		return 0, true, nil
	}
	if payload, err := codec.Marshal(count); err == nil {
		_ = adapter.HSet(ctx, key, countField, payload, b.pageCacheOpt.Duration)
	}
	return count, count == 0, nil
}

func (b *SelectBuilder) rowsAndCountWithCache(ctx context.Context, dialect gooq.Dialect) (db.Result, int64, error) {
	if executorInTx(b.executor) {
		count, err := b.countValue(ctx, dialect)
		if err != nil || count == 0 {
			return nil, count, err
		}
		rows, err := b.rowsValue(ctx, dialect)
		return rows, count, err
	}
	dataField, countField := b.cacheFieldNames()
	adapter, key, fields, err := b.compositeCacheBegin(ctx, dialect, "rows")
	if err != nil {
		return nil, 0, err
	}
	count, short, err := b.resolveCompositeCount(ctx, dialect, adapter, key, countField, fields)
	if err != nil {
		return nil, 0, err
	}
	if short {
		return nil, 0, nil
	}
	codec := GetCacheCodec()
	if payload, ok := fields[dataField]; ok {
		var rows db.Result
		if err := codec.UnmarshalRecord(payload, &rows); err != nil {
			return nil, 0, err
		}
		return rows, count, nil
	}
	rows, err := b.rowsValue(ctx, dialect)
	if err != nil {
		return nil, 0, err
	}
	if rows == nil {
		rows = db.Result{}
	}
	if payload, err := codec.Marshal(rows); err == nil {
		_ = adapter.HSet(ctx, key, dataField, payload, b.pageCacheOpt.Duration)
	}
	return rows, count, nil
}

// StructsAndCount 先取总数和当前页，再把行填进结构体切片。总数为 0 时不再查页。
func (b *SelectBuilder) StructsAndCount(ctx context.Context, dest any) (int64, error) {
	rows, count, err := b.RowsAndCount(ctx)
	if err != nil || count == 0 {
		return count, err
	}
	if err := rows.Structs(dest); err != nil {
		return 0, err
	}
	return count, nil
}

// ColumnAndCount 先取总数，再把当前页的那一列填进单元格切片。总数为 0 时不再查页。
// 结果列数不是 1 时返回错误。
func (b *SelectBuilder) ColumnAndCount(ctx context.Context, dest any) (int64, error) {
	if err := db.CheckColumn(dest); err != nil {
		return 0, err
	}
	restore, err := b.bind(ctx)
	if err != nil {
		return 0, err
	}
	defer restore()
	dialect := b.dialect()
	if b.pageCacheOpt != nil {
		return b.columnAndCountWithCache(ctx, dialect, dest)
	}
	count, err := b.countValue(ctx, dialect)
	if err != nil || count == 0 {
		return count, err
	}
	sql, args, err := b.ToSql(dialect)
	if err != nil {
		return 0, err
	}
	if err = execColumnExact(ctx, b.executor, sql, args, dest); err != nil {
		return 0, err
	}
	return count, nil
}

func (b *SelectBuilder) columnAndCountWithCache(ctx context.Context, dialect gooq.Dialect, dest any) (int64, error) {
	if executorInTx(b.executor) {
		count, err := b.countValue(ctx, dialect)
		if err != nil || count == 0 {
			return count, err
		}
		sql, args, err := b.ToSql(dialect)
		if err != nil {
			return 0, err
		}
		if err = execColumnExact(ctx, b.executor, sql, args, dest); err != nil {
			return 0, err
		}
		return count, nil
	}
	dataField, countField := b.cacheFieldNames()
	adapter, key, fields, err := b.compositeCacheBegin(ctx, dialect, "column")
	if err != nil {
		return 0, err
	}
	count, short, err := b.resolveCompositeCount(ctx, dialect, adapter, key, countField, fields)
	if err != nil || short {
		return count, err
	}
	codec := GetCacheCodec()
	if payload, ok := fields[dataField]; ok {
		if err = codec.Unmarshal(payload, dest); err != nil {
			return 0, err
		}
		return count, nil
	}
	sql, args, err := b.ToSql(dialect)
	if err != nil {
		return 0, err
	}
	if err = execColumnExact(ctx, b.executor, sql, args, dest); err != nil {
		return 0, err
	}
	if payload, err := codec.Marshal(dest); err == nil {
		_ = adapter.HSet(ctx, key, dataField, payload, b.pageCacheOpt.Duration)
	}
	return count, nil
}
