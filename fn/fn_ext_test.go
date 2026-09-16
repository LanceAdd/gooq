// 本文件以包外视角（fn_test）验证自定义函数契约：仅用导出 API 即可包装类型化函数构造器并参与渲染。
package fn_test

import (
	"testing"

	"github.com/gogf/gf/v2/test/gtest"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/dsl"
	"github.com/lanceadd/gooq/fn"
)

// concatPipe 是"用户包内自定义函数"示例：构造器 + 实例级方言渲染策略。
// MySQL/SQLite 渲染 CONCAT(a, b)，PG 渲染 (a || b)。
func concatPipe(a, b gooq.Expression) *fn.FuncExpr {
	return fn.New("CONCAT", a, b).RenderWith(
		func(rc *gooq.RenderContext, argsSQL []string) (string, []any) {
			if rc.Dialect() == gooq.DialectPgsql {
				return "(" + argsSQL[0] + " || " + argsSQL[1] + ")", nil
			}
			return "CONCAT(" + argsSQL[0] + ", " + argsSQL[1] + ")", nil
		},
	)
}

type userTable struct {
	*gooq.TableBase
	ID   gooq.Field[int64]
	Name gooq.Field[string]
}

func newUserTable() *userTable {
	t := &userTable{TableBase: gooq.NewTableBase(&gooq.TableMeta{
		TableName: "user",
		Fields: []gooq.FieldMeta{
			{ColumnName: "id", LocalType: gooq.LocalTypeInt64, Primary: true},
			{ColumnName: "name", LocalType: gooq.LocalTypeString},
		},
	})}
	t.ID = gooq.NewFieldAt[int64](t.TableBase, "id")
	t.Name = gooq.NewFieldAt[string](t.TableBase, "name")
	return t
}

var u = newUserTable()

func TestExt_CustomFunction(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		// 自定义函数可直接用于 SELECT 字段，As/Over 等链式能力与内建函数一致。
		sql, _, err := dsl.Select(concatPipe(u.Name, gooq.Str("-x")).As("tag")).From(u).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT CONCAT(`user`.`name`, '-x') AS tag FROM `user`")

		sql, _, err = dsl.Select(concatPipe(u.Name, gooq.Str("-x")).As("tag")).From(u).ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT ("user"."name" || '-x') AS tag FROM "user"`)

		// 条件位置同样可用。
		sql, _, err = dsl.Select(u.ID).From(u).
			Where(gooq.Eq(concatPipe(u.Name, gooq.Str("-x")), gooq.Str("a-x"))).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE CONCAT(`user`.`name`, '-x') = 'a-x'")
	})
}
