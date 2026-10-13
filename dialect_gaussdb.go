package gooq

import "fmt"

// GaussDBDialect 是 GaussDB（openGauss）的 SQL 渲染实现。
// 引号、占位符、行锁、删除、更新和函数嵌入 PostgreSQL。
// openGauss 没有 ON CONFLICT。冲突更新写成 ON DUPLICATE KEY UPDATE，本次插入值写成 VALUES(col)。
// 单条语句表达不了 DO NOTHING，DoNothing 返回错误。
type GaussDBDialect struct {
	PostgresDialect
}

// Name 返回 gaussdb。
func (GaussDBDialect) Name() Dialect { return DialectGaussDB }

// RenderUpsert 渲染 ON DUPLICATE KEY UPDATE。
// 冲突列不写进 SQL，由主键或唯一键决定。DoNothing 没有对应子句。
func (GaussDBDialect) RenderUpsert(rc *RenderContext, clause UpsertClause) (string, error) {
	if clause.DoNothing {
		return "", fmt.Errorf("gooq: INSERT IGNORE is not supported by dialect %s", DialectGaussDB)
	}
	sets, err := renderUpsertSets(rc, clause.Updates)
	if err != nil {
		return "", err
	}
	return " ON DUPLICATE KEY UPDATE " + sets, nil
}

// RenderExcluded 渲染本次插入的列值。openGauss 使用 VALUES(col)，不是 EXCLUDED.col。
func (GaussDBDialect) RenderExcluded(rc *RenderContext, column string) string {
	return "VALUES(" + rc.Quote(column) + ")"
}

// RenderDateAdd 渲染间隔乘法。openGauss 没有 make_interval。
func (GaussDBDialect) RenderDateAdd(field, count, unit string) (string, error) {
	canonical, err := canonicalDateUnit(unit)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("(%s + (%s::int * INTERVAL '1 %s'))", field, count, canonical), nil
}
