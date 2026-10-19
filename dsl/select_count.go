package dsl

import (
	"context"

	"github.com/lanceadd/gooq"
)

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
