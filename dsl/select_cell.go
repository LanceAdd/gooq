package dsl

import (
	"context"
	"fmt"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

// queryWithCache 执行单元格查询。Count 和 Exists 走这里。PageCache 不服务单值查询。
func (b *SelectBuilder) queryWithCache(ctx context.Context, sql string, args []any, dest any, kind string) error {
	if b.pageCacheOpt != nil {
		return errPageCache
	}
	if err := db.CheckValue(dest); err != nil {
		return err
	}
	run := func() error {
		return execValue(ctx, b.executor, sql, args, dest)
	}
	if b.cacheOption != nil {
		if adapter := GetCacheAdapter(); adapter != nil {
			return b.cellWithCache(ctx, adapter, sql, args, dest, kind, run)
		}
	}
	return run()
}

// Value 把单行的那一列写进 dest。
// dest 是一个单元格，例如 *time.Time、*sql.NullTime、*sql.NullString 或 *int。
// 结果列数不是 1 时返回错误。
// 没有 Limit 时本次执行补 LIMIT 1，保留 Offset，查询链不变。
// Limit 大于 1 时返回错误；没有行返回 sql.ErrNoRows；NULL 写零值。
// 行结构体用 Row 和 Record.Struct，切片用 Column。
func (b *SelectBuilder) Value(ctx context.Context, dest any) error {
	if err := db.CheckValue(dest); err != nil {
		return err
	}
	if b.limit > 1 {
		return errManyRows(b.limit)
	}
	return b.readCell(ctx, dest, "value", b.oneRowSQL, execValueExact)
}

// Column 把那一列的每个值写进 dest。
// dest 是单元格切片的指针；构建器上的分页保留。
// 结果列数不是 1 时返回错误。
// 没有行时写成空切片；行结构体切片用 Rows 和 Result.Structs。
func (b *SelectBuilder) Column(ctx context.Context, dest any) error {
	if err := db.CheckColumn(dest); err != nil {
		return err
	}
	return b.readCell(ctx, dest, "column", func(dialect gooq.Dialect) (string, []any, error) {
		return b.ToSql(dialect)
	}, execColumnExact)
}

func errManyRows(limit int) error {
	return fmt.Errorf("gooq: query limit %d returns more than one row", limit)
}

// oneRowSQL 准备只返回一行的语句。Limit 为 1 时原样渲染，可以带 Offset。
// 还没有 Limit 时，这次执行补 LIMIT 1 并保留 Offset，不改调用方的查询链。
// Limit 大于 1 时返回错误。
func (b *SelectBuilder) oneRowSQL(dialect gooq.Dialect) (string, []any, error) {
	if b.limit > 1 {
		return "", nil, errManyRows(b.limit)
	}
	query := b
	if b.limit != 1 {
		query = b.Clone()
		query.limit = 1
	}
	return query.ToSql(dialect)
}

func (b *SelectBuilder) readCell(
	ctx context.Context, dest any, kind string,
	prepare func(gooq.Dialect) (string, []any, error),
	run func(context.Context, executor, string, []any, any) error,
) error {
	restore, err := b.bind(ctx)
	if err != nil {
		return err
	}
	defer restore()
	if b.pageCacheOpt != nil {
		return errPageCache
	}
	dialect := b.dialect()
	sql, args, err := prepare(dialect)
	if err != nil {
		return err
	}
	if b.cacheOption != nil {
		if adapter := GetCacheAdapter(); adapter != nil {
			return b.cellWithCache(ctx, adapter, sql, args, dest, kind, func() error {
				return run(ctx, b.executor, sql, args, dest)
			})
		}
	}
	return run(ctx, b.executor, sql, args, dest)
}

func executorInTx(exec executor) bool {
	_, ok := exec.(*db.Tx)
	return ok
}

func (b *SelectBuilder) cellWithCache(
	ctx context.Context, adapter CacheAdapter, sql string, args []any, dest any, kind string, run func() error,
) error {
	if executorInTx(b.executor) {
		return run()
	}
	key, err := b.cacheKey(kind, sql, args)
	if err != nil {
		return err
	}
	codec := GetCacheCodec()
	if payload, ok, err := adapter.Get(ctx, key); err == nil && ok {
		return codec.Unmarshal(payload, dest)
	}
	if err = run(); err != nil {
		return err
	}
	if isEmptyResult(dest) && !b.cacheOption.Force {
		return nil
	}
	if payload, err := codec.Marshal(dest); err == nil {
		_ = adapter.Set(ctx, key, payload, b.cacheOption.Duration)
	}
	return nil
}
