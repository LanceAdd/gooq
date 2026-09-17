// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件为函数库单元测试：标准函数、聚合、DATE_FORMAT 跨库、字符串聚合、窗口与 SELECT 集成。
package fn

import (
	"testing"
	"time"

	"github.com/gogf/gf/v2/test/gtest"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/dsl"
)

type testUserTable struct {
	*gooq.TableBase
	ID        gooq.Field[int64]
	Name      gooq.Field[string]
	Age       gooq.Field[int]
	Status    gooq.Field[string]
	CreatedAt gooq.Field[time.Time]
	DeletedAt gooq.Field[time.Time]
}

var testUserMeta = &gooq.TableMeta{
	TableName: "user",
	Fields: []gooq.FieldMeta{
		{ColumnName: "id", LocalType: gooq.LocalTypeInt64, Primary: true, AutoIncrement: true},
		{ColumnName: "name", LocalType: gooq.LocalTypeString},
		{ColumnName: "age", LocalType: gooq.LocalTypeInt},
		{ColumnName: "status", LocalType: gooq.LocalTypeString},
		{ColumnName: "created_at", LocalType: gooq.LocalTypeDatetime},
		{ColumnName: "deleted_at", LocalType: gooq.LocalTypeDatetime, SoftDelete: true},
	},
}

func newTestUserTable(alias ...string) *testUserTable {
	t := &testUserTable{TableBase: gooq.NewTableBase(testUserMeta)}
	if len(alias) > 0 {
		t.TableBase = t.TableBase.As(alias[0])
	}
	t.ID = gooq.NewFieldAt[int64](t.TableBase, "id")
	t.Name = gooq.NewFieldAt[string](t.TableBase, "name")
	t.Age = gooq.NewFieldAt[int](t.TableBase, "age")
	t.Status = gooq.NewFieldAt[string](t.TableBase, "status")
	t.CreatedAt = gooq.NewFieldAt[time.Time](t.TableBase, "created_at")
	t.DeletedAt = gooq.NewFieldAt[time.Time](t.TableBase, "deleted_at")
	return t
}

var testUser = newTestUserTable()

func (t *testUserTable) As(alias string) *testUserTable {
	return newTestUserTable(alias)
}

func (t *testUserTable) Clone() *testUserTable {
	return newTestUserTable()
}

type testUserRoleTable struct {
	*gooq.TableBase
	UserID gooq.Field[int64]
}

func newTestUserRoleTable() *testUserRoleTable {
	t := &testUserRoleTable{TableBase: gooq.NewTableBase(&gooq.TableMeta{
		TableName: "user_role",
		Fields: []gooq.FieldMeta{
			{ColumnName: "user_id", LocalType: gooq.LocalTypeInt64},
		},
	})}
	t.UserID = gooq.NewFieldAt[int64](t.TableBase, "user_id")
	return t
}

var testUserRole = newTestUserRoleTable()

func TestFn_Basic(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		cases := []struct {
			expr gooq.Expression
			sql  string
			args []any
		}{
			{Upper(testUser.Name), "UPPER(`user`.`name`)", nil},
			{Lower(testUser.Name), "LOWER(`user`.`name`)", nil},
			{Concat(testUser.Name, gooq.Str(","), testUser.Status), "CONCAT(`user`.`name`, ',', `user`.`status`)", nil},
			{Coalesce(testUser.Name, gooq.Str("unknown")), "COALESCE(`user`.`name`, 'unknown')", nil},
			{IfNull(testUser.Name, gooq.Str("unknown")), "IFNULL(`user`.`name`, 'unknown')", nil},
			{Length(testUser.Name), "LENGTH(`user`.`name`)", nil},
			{Substring(testUser.Name, 1, 3), "SUBSTRING(`user`.`name`, ?, ?)", []any{1, 3}},
			{Trim(testUser.Name), "TRIM(`user`.`name`)", nil},
			{Replace(testUser.Name, gooq.Str("a"), gooq.Str("b")), "REPLACE(`user`.`name`, 'a', 'b')", nil},
			{Abs(testUser.Age), "ABS(`user`.`age`)", nil},
			{Round(testUser.Age, 2), "ROUND(`user`.`age`, ?)", []any{2}},
			{Ceil(testUser.Age), "CEIL(`user`.`age`)", nil},
			{Floor(testUser.Age), "FLOOR(`user`.`age`)", nil},
			{Mod(testUser.Age, 2), "MOD(`user`.`age`, ?)", []any{2}},
			{CurDate(), "CURDATE()", nil},
			{Now(), "NOW()", nil},
			{DateAdd(testUser.CreatedAt, 1, "DAY"), "DATE_ADD(`user`.`created_at`, INTERVAL ? DAY)", []any{1}},
			{DateDiff(testUser.CreatedAt, Now()), "DATEDIFF(`user`.`created_at`, NOW())", nil},
		}
		for _, c := range cases {
			sql, args, err := dsl.Select(c.expr).From(testUser).ToSql(gooq.DialectMySQL)
			t.AssertNil(err)
			t.Assert(sql, "SELECT "+c.sql+" FROM `user` WHERE `user`.`deleted_at` IS NULL")
			if c.args == nil {
				t.Assert(len(args), 0)
			} else {
				t.AssertEQ(args, c.args)
			}
		}
	})
}

func TestFn_Aggregate(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		cases := []struct {
			expr gooq.Expression
			sql  string
		}{
			{Count(testUser.ID), "COUNT(`user`.`id`)"},
			{Sum(testUser.Age), "SUM(`user`.`age`)"},
			{Avg(testUser.Age), "AVG(`user`.`age`)"},
			{Min(testUser.Age), "MIN(`user`.`age`)"},
			{Max(testUser.Age), "MAX(`user`.`age`)"},
			{CountDistinct(testUser.Status), "COUNT(DISTINCT `user`.`status`)"},
			{AnyValue(testUser.Age), "ANY_VALUE(`user`.`age`)"},
			{ArrayAgg(testUser.Age), "ARRAY_AGG(`user`.`age`)"},
			{BitAnd(testUser.Age), "BIT_AND(`user`.`age`)"},
			{BitOr(testUser.Age), "BIT_OR(`user`.`age`)"},
			{BitXor(testUser.Age), "BIT_XOR(`user`.`age`)"},
			{BoolAnd(testUser.Age), "BOOL_AND(`user`.`age`)"},
			{BoolOr(testUser.Age), "BOOL_OR(`user`.`age`)"},
			{Every(testUser.Age), "EVERY(`user`.`age`)"},
			{JsonArrayAgg(testUser.Age), "JSON_ARRAYAGG(`user`.`age`)"},
			{JsonObjectAgg(testUser.Name, testUser.Age), "JSON_OBJECTAGG(`user`.`name`, `user`.`age`)"},
			{Grouping(testUser.Status), "GROUPING(`user`.`status`)"},
			{XmlAgg(testUser.Name), "XMLAGG(`user`.`name`)"},
		}
		for _, c := range cases {
			sql, _, err := dsl.Select(c.expr).From(testUser).ToSql(gooq.DialectMySQL)
			t.AssertNil(err)
			t.Assert(sql, "SELECT "+c.sql+" FROM `user` WHERE `user`.`deleted_at` IS NULL")
		}
	})
}

func TestFn_DateFormat(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := dsl.Select(DateFormat(testUser.CreatedAt, "%Y-%m-%d")).From(testUser).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT DATE_FORMAT(`user`.`created_at`, '%Y-%m-%d') FROM `user` WHERE `user`.`deleted_at` IS NULL")
		t.AssertEQ(args, nil)

		sql, _, err = dsl.Select(DateFormat(testUser.CreatedAt, "%Y-%m-%d")).From(testUser).ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT TO_CHAR("user"."created_at", 'YYYY-MM-DD') FROM "user" WHERE "user"."deleted_at" IS NULL`)

		sql, _, err = dsl.Select(DateFormat(testUser.CreatedAt, "%Y-%m-%d")).From(testUser).ToSql(gooq.DialectSQLite)
		t.AssertNil(err)
		t.Assert(sql, `SELECT strftime('%Y-%m-%d', "user"."created_at") FROM "user" WHERE "user"."deleted_at" IS NULL`)

		sql, _, err = dsl.Select(DateFormat(testUser.CreatedAt, "%Y-%m-%d %H:%i:%s")).From(testUser).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT DATE_FORMAT(`user`.`created_at`, '%Y-%m-%d %H:%i:%s') FROM `user` WHERE `user`.`deleted_at` IS NULL")

		sql, _, err = dsl.Select(DateFormat(testUser.CreatedAt, "%Y-%m-%d %H:%i:%s")).From(testUser).ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT TO_CHAR("user"."created_at", 'YYYY-MM-DD HH24:MI:SS') FROM "user" WHERE "user"."deleted_at" IS NULL`)

		sql, _, err = dsl.Select(DateFormat(testUser.CreatedAt, "%Y-%m-%d %H:%i:%s")).From(testUser).ToSql(gooq.DialectSQLite)
		t.AssertNil(err)
		t.Assert(sql, `SELECT strftime('%Y-%m-%d %H:%M:%S', "user"."created_at") FROM "user" WHERE "user"."deleted_at" IS NULL`)
	})
}

func TestFn_GroupConcat(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := dsl.Select(GroupConcat(GroupConcatOptions{Field: testUser.Name})).
			From(testUser).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT GROUP_CONCAT(`user`.`name`) FROM `user` WHERE `user`.`deleted_at` IS NULL")

		sql, _, err = dsl.Select(GroupConcat(GroupConcatOptions{
			Field: testUser.Name, Separator: "-", OrderBy: []gooq.Expression{testUser.Name.Asc()},
		})).From(testUser).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT GROUP_CONCAT(`user`.`name` ORDER BY `user`.`name` ASC SEPARATOR '-') FROM `user` WHERE `user`.`deleted_at` IS NULL")

		sql, _, err = dsl.Select(GroupConcat(GroupConcatOptions{
			Field: testUser.Name, Distinct: true,
		})).From(testUser).ToSql(gooq.DialectSQLite)
		t.AssertNil(err)
		t.Assert(sql, `SELECT GROUP_CONCAT(DISTINCT "user"."name", ',') FROM "user" WHERE "user"."deleted_at" IS NULL`)

		sql, _, err = dsl.Select(GroupConcat(GroupConcatOptions{Field: testUser.Name})).
			From(testUser).ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT STRING_AGG("user"."name", ',') FROM "user" WHERE "user"."deleted_at" IS NULL`)

		_, _, err = dsl.Select(GroupConcat(GroupConcatOptions{Field: testUser.Name, Distinct: true})).
			From(testUser).ToSql(gooq.DialectPgsql)
		t.AssertNE(err, nil)
	})
}

func TestFn_Window(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := dsl.Select(
			Rank().Over([]gooq.Expression{testUser.Status}, []gooq.Expression{testUser.Age.Desc()}).
				As("r"),
		).From(testUser).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT RANK() OVER (PARTITION BY `user`.`status` ORDER BY `user`.`age` DESC) AS r FROM `user` WHERE `user`.`deleted_at` IS NULL")

		sql, _, err = dsl.Select(
			RowNumber().Over(nil, []gooq.Expression{testUser.ID.Asc()}),
		).From(testUser).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT ROW_NUMBER() OVER (ORDER BY `user`.`id` ASC) FROM `user` WHERE `user`.`deleted_at` IS NULL")

		sql, _, err = dsl.Select(
			Sum(testUser.Age).OverFrame(
				[]gooq.Expression{testUser.Status},
				[]gooq.Expression{testUser.ID.Asc()},
				RowsFrame("UNBOUNDED PRECEDING", "CURRENT ROW"),
			),
		).From(testUser).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT SUM(`user`.`age`) OVER (PARTITION BY `user`.`status` ORDER BY `user`.`id` ASC ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) FROM `user` WHERE `user`.`deleted_at` IS NULL")

		sql, _, err = dsl.Select(
			CumeDist().Over(nil, []gooq.Expression{testUser.Age.Asc()}),
		).From(testUser).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT CUME_DIST() OVER (ORDER BY `user`.`age` ASC) FROM `user` WHERE `user`.`deleted_at` IS NULL")

		sql, _, err = dsl.Select(
			PercentRank().Over([]gooq.Expression{testUser.Status}, []gooq.Expression{testUser.Age.Desc()}),
		).From(testUser).ToSql(gooq.DialectSQLite)
		t.AssertNil(err)
		t.Assert(sql, `SELECT PERCENT_RANK() OVER (PARTITION BY "user"."status" ORDER BY "user"."age" DESC) FROM "user" WHERE "user"."deleted_at" IS NULL`)
	})
}

func TestFn_OrderedSet(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := dsl.Select(PercentileCont(0.5, testUser.Age.Asc())).From(testUser).ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT PERCENTILE_CONT($1) WITHIN GROUP (ORDER BY "user"."age" ASC) FROM "user" WHERE "user"."deleted_at" IS NULL`)
		t.AssertEQ(args, []any{0.5})

		sql, args, err = dsl.Select(PercentileDisc(0.9, testUser.Age.Desc())).From(testUser).ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT PERCENTILE_DISC($1) WITHIN GROUP (ORDER BY "user"."age" DESC) FROM "user" WHERE "user"."deleted_at" IS NULL`)
		t.AssertEQ(args, []any{0.9})

		sql, _, err = dsl.Select(Mode(testUser.Age.Desc())).From(testUser).ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT MODE() WITHIN GROUP (ORDER BY "user"."age" DESC) FROM "user" WHERE "user"."deleted_at" IS NULL`)

		sql, _, err = dsl.Select(Median(testUser.Age)).From(testUser).ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY "user"."age") FROM "user" WHERE "user"."deleted_at" IS NULL`)

		sql, _, err = dsl.Select(Median(testUser.Age)).From(testUser).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT MEDIAN(`user`.`age`) FROM `user` WHERE `user`.`deleted_at` IS NULL")
	})
}

func TestFn_Filter(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := dsl.Select(Sum(testUser.Age).Filter(testUser.Status.Eq("active"))).
			From(testUser).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT SUM(`user`.`age`) FILTER (WHERE `user`.`status` = ?) FROM `user` WHERE `user`.`deleted_at` IS NULL")
		t.AssertEQ(args, []any{"active"})

		sql, args, err = dsl.Select(
			Count(testUser.ID).Filter(testUser.Status.Eq("active")).
				Over([]gooq.Expression{testUser.Status}, nil),
		).From(testUser).ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT COUNT("user"."id") FILTER (WHERE "user"."status" = $1) OVER (PARTITION BY "user"."status") FROM "user" WHERE "user"."deleted_at" IS NULL`)
		t.AssertEQ(args, []any{"active"})
	})
}

// TestFn_SelectIntegration 验证函数与 SELECT 构建的集成（分组聚合、LATERAL 派生表）。
func TestFn_SelectIntegration(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := dsl.Select(testUser.Status, Count(testUser.ID)).
			From(testUser).
			Group(testUser.Status).
			Having(gooq.Gt(Count(testUser.ID), 2)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`status`, COUNT(`user`.`id`) FROM `user` WHERE `user`.`deleted_at` IS NULL GROUP BY `user`.`status` HAVING COUNT(`user`.`id`) > ?")
		t.AssertEQ(args, []any{2})

		u := testUser.As("u")
		lt := dsl.Select(Count(testUserRole.UserID).As("cnt")).
			From(testUserRole).Where(testUserRole.UserID.EqExpr(u.ID)).As("lt")
		_, _, err = dsl.Select(u.ID, lt.Field("cnt")).From(u).
			LeftJoinLateral(lt).On(gooq.Raw("1 = 1")).
			ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
	})
}

// TestFn_Chaining 验证所有构造器统一返回 *FuncExpr：As/Over 对每个函数恒可用。
func TestFn_Chaining(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := dsl.Select(DateFormat(testUser.CreatedAt, "%Y").As("y")).From(testUser).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT DATE_FORMAT(`user`.`created_at`, '%Y') AS y FROM `user` WHERE `user`.`deleted_at` IS NULL")

		sql, _, err = dsl.Select(GroupConcat(GroupConcatOptions{Field: testUser.Name}).As("names")).
			From(testUser).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT GROUP_CONCAT(`user`.`name`) AS names FROM `user` WHERE `user`.`deleted_at` IS NULL")

		sql, _, err = dsl.Select(
			Count(testUser.ID).Over([]gooq.Expression{testUser.Status}, nil).As("cnt"),
		).From(testUser).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT COUNT(`user`.`id`) OVER (PARTITION BY `user`.`status`) AS cnt FROM `user` WHERE `user`.`deleted_at` IS NULL")
	})
}

// TestFn_RenderWith 验证实例级方言渲染策略（非全局注册表）。
func TestFn_RenderWith(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		// concatPipe：MySQL/SQLite 用 CONCAT，PG 用 ||。
		concatPipe := func(a, b gooq.Expression) *FuncExpr {
			return New("CONCAT", a, b).RenderWith(
				func(rc *gooq.RenderContext, argsSQL []string) (string, []any) {
					if rc.Dialect() == gooq.DialectPgsql {
						return "(" + argsSQL[0] + " || " + argsSQL[1] + ")", nil
					}
					return "CONCAT(" + argsSQL[0] + ", " + argsSQL[1] + ")", nil
				},
			)
		}

		sql, _, err := dsl.Select(concatPipe(testUser.Name, gooq.Str("-x"))).From(testUser).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT CONCAT(`user`.`name`, '-x') FROM `user` WHERE `user`.`deleted_at` IS NULL")

		sql, _, err = dsl.Select(concatPipe(testUser.Name, gooq.Str("-x"))).From(testUser).ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT ("user"."name" || '-x') FROM "user" WHERE "user"."deleted_at" IS NULL`)
	})
}
