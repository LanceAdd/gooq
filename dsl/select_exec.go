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
		return errPageCache()
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
		return errPageCache()
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

func (b *SelectBuilder) cellWithCache(
	ctx context.Context, adapter CacheAdapter, sql string, args []any, dest any, kind string, run func() error,
) error {
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

// countWrapped 为 true 时，计数和存在性判断都要把原查询套成子查询。
// 普通查询直接改选择列表会丢掉 DISTINCT、分组或集合操作的结果行。
func (b *SelectBuilder) countWrapped() bool {
	return b.distinct || len(b.groupBy) > 0 || b.groupExt != groupExtNone || len(b.setOps) > 0
}

// unbounded 复制查询并去掉分页和排序。原查询链不变。
func (b *SelectBuilder) unbounded() *SelectBuilder {
	inner := b.Clone()
	inner.limit = 0
	inner.offset = 0
	inner.orderBy = nil
	return inner
}

// countBuilderOf 构造 COUNT(*)：清除分页与排序。
// 分组、集合操作或 DISTINCT 外包子查询 (SELECT COUNT(*) FROM (原查询) AS t)，
// 计数为分组、集合或去重后的结果行数。
func (b *SelectBuilder) countBuilderOf() *SelectBuilder {
	if b.countWrapped() {
		outer := Select(gooq.Raw("COUNT(*)"))
		outer.from = b.unbounded().As("t")
		outer.hasCtxTenant, outer.ctxTenant = b.hasCtxTenant, b.ctxTenant
		return outer
	}
	countBuilder := b.unbounded()
	countBuilder.fields = []gooq.Expression{gooq.Raw("COUNT(*)")}
	return countBuilder
}

// existsBuilder 构造存在性子查询，判断的行与 Count 相同。
// 普通查询把选择列表换成 1。DISTINCT、分组或集合操作套的子查询与 Count 相同。
func (b *SelectBuilder) existsBuilder() *SelectBuilder {
	if b.countWrapped() {
		outer := Select(gooq.Raw("1"))
		outer.from = b.unbounded().As("t")
		outer.hasCtxTenant, outer.ctxTenant = b.hasCtxTenant, b.ctxTenant
		return outer
	}
	sub := b.unbounded()
	sub.fields = []gooq.Expression{gooq.Raw("1")}
	return sub
}

// checkedSQL 在改写语句之前校验构造错误和原查询的方言能力。
// 包裹后的外层查询看不到内层 GROUP BY 扩展，也不会带回 buildErr。
func (b *SelectBuilder) checkedSQL(dialect gooq.Dialect, build func() *SelectBuilder) (string, []any, error) {
	if b.buildErr != nil {
		return "", nil, b.buildErr
	}
	if err := b.ValidateDialect(dialect); err != nil {
		return "", nil, err
	}
	return build().ToSql(dialect)
}

// existsSQL 渲染存在性探测。普通方言是 SELECT EXISTS (SELECT 1 ...)。
// 达梦是 SELECT COUNT(*) FROM (SELECT 1 ...) WHERE ROWNUM = 1。
func (b *SelectBuilder) existsSQL(dialect gooq.Dialect) (string, []any, error) {
	sql, args, err := b.checkedSQL(dialect, b.existsBuilder)
	if err != nil {
		return "", nil, err
	}
	spec, err := gooq.DialectSpecOf(dialect)
	if err != nil {
		return "", nil, err
	}
	return spec.RenderExists(gooq.NewRenderContext(dialect), sql), args, nil
}

// countValue 执行 COUNT(*) 查询（Count/RowsAndCount 共用）。
func (b *SelectBuilder) countValue(ctx context.Context, dialect gooq.Dialect) (int64, error) {
	sql, args, err := b.countSQL(dialect)
	if err != nil {
		return 0, err
	}
	value, err := b.executor.GetValue(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return toInt64(value)
}

// countSQL 生成 COUNT(*) 查询 SQL（Count 缓存 key 与 countValue 共用）。
func (b *SelectBuilder) countSQL(dialect gooq.Dialect) (string, []any, error) {
	return b.checkedSQL(dialect, b.countBuilderOf)
}

// Count 返回去掉分页和排序之后的结果行数。查询链上的 Page 或 Limit 在执行时去掉。
func (b *SelectBuilder) Count(ctx context.Context) (int64, error) {
	restore, err := b.bind(ctx)
	if err != nil {
		return 0, err
	}
	defer restore()
	dialect := b.dialect()
	sql, args, err := b.countSQL(dialect)
	if err != nil {
		return 0, err
	}
	var count int64
	if err := b.queryWithCache(ctx, sql, args, &count, "count"); err != nil {
		return 0, err
	}
	return count, nil
}

// Exists 判断 Count 是否大于 0。分页和排序会去掉，选择列表换成 1。
// 有 DISTINCT、分组或集合操作时，套的子查询与 Count 相同。
func (b *SelectBuilder) Exists(ctx context.Context) (bool, error) {
	restore, err := b.bind(ctx)
	if err != nil {
		return false, err
	}
	defer restore()
	dialect := b.dialect()
	sql, args, err := b.existsSQL(dialect)
	if err != nil {
		return false, err
	}
	var ok bool
	if err := b.queryWithCache(ctx, sql, args, &ok, "exists"); err != nil {
		return false, err
	}
	return ok, nil
}

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
