package dsl

import (
	"fmt"
	"strings"

	"github.com/lanceadd/gooq"
)

func parenthesizeSQL(sql string) string {
	return "(" + sql + ")"
}

func joinKeywordSQL(rc *gooq.RenderContext, keyword string) string {
	if spec := rc.Spec(); spec != nil {
		keyword = spec.JoinKeyword(keyword)
	}
	if rc.Dialect() == gooq.DialectSQLite && strings.Contains(keyword, "LATERAL") {
		rc.Fail(fmt.Errorf("gooq: LATERAL join is not supported by dialect %s", gooq.DialectSQLite))
	}
	return keyword
}

// renderTableName 渲染表或派生表（表名 + 别名）；SelectBuilder 与 DMLBuilder 共用。
func renderTableName(rc *gooq.RenderContext, t gooq.Table) string {
	if sub, ok := t.(*SelectBuilder); ok {
		subSQL := sub.renderSelect(rc)
		sql := "(" + subSQL + ")"
		if sub.alias != "" {
			sql += " AS " + rc.Quote(sub.alias)
		}
		return sql
	}
	var sql = tableNameSQL(rc, t)
	if t.AliasName() != "" {
		sql += " AS " + rc.Quote(t.AliasName())
	}
	return sql
}

// tableNameSQL 渲染限定表名（schema.table）；分片表先按分片值解析物理名。
func tableNameSQL(rc *gooq.RenderContext, t gooq.Table) string {
	var (
		tableName = t.TableName()
		schema    string
	)
	if m := t.Meta(); m != nil {
		tableName = gooq.ResolveTableName(rc, t, tableName)
		schema = m.Schema
	}
	name := rc.Quote(tableName)
	if schema != "" {
		name = rc.Quote(schema) + "." + name
	}
	return name
}
