// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件为函数/表达式/方言差异单元测试：函数库、DATE_FORMAT 跨库、字符串聚合、窗口、CASE、算术。
package gooq

import (
	"context"
	"fmt"
	"testing"

	"github.com/gogf/gf/v2/test/gtest"
)

func TestDsl_Funcs_Basic(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		cases := []struct {
			expr Expression
			sql  string
			args []any
		}{
			{UpperFunc(testUser.Name), "UPPER(`name`)", nil},
			{LowerFunc(testUser.Name), "LOWER(`name`)", nil},
			{ConcatFunc(testUser.Name, Str(","), testUser.Status), "CONCAT(`name`, ',', `status`)", nil},
			{CoalesceFunc(testUser.Name, Str("unknown")), "COALESCE(`name`, 'unknown')", nil},
			{IfNullFunc(testUser.Name, Str("unknown")), "IFNULL(`name`, 'unknown')", nil},
			{LengthFunc(testUser.Name), "LENGTH(`name`)", nil},
			{SubstringFunc(testUser.Name, 1, 3), "SUBSTRING(`name`, ?, ?)", []any{1, 3}},
			{TrimFunc(testUser.Name), "TRIM(`name`)", nil},
			{ReplaceFunc(testUser.Name, Str("a"), Str("b")), "REPLACE(`name`, 'a', 'b')", nil},
			{AbsFunc(testUser.Age), "ABS(`age`)", nil},
			{RoundFunc(testUser.Age, 2), "ROUND(`age`, ?)", []any{2}},
			{CeilFunc(testUser.Age), "CEIL(`age`)", nil},
			{FloorFunc(testUser.Age), "FLOOR(`age`)", nil},
			{ModFunc(testUser.Age, 2), "MOD(`age`, ?)", []any{2}},
			{CurDateFunc(), "CURDATE()", nil},
			{NowFunc(), "NOW()", nil},
			{DateAddFunc(testUser.CreatedAt, "INTERVAL 1 DAY"), "DATE_ADD(`created_at`, ?)", []any{"INTERVAL 1 DAY"}},
			{DateDiffFunc(testUser.CreatedAt, NowFunc()), "DATEDIFF(`created_at`, NOW())", nil},
		}
		for _, c := range cases {
			sql, args, err := Select(c.expr).From(testUser).ToSql(DialectMySQL)
			t.AssertNil(err)
			t.Assert(sql, "SELECT "+c.sql+" FROM `user` WHERE `deleted_at` IS NULL")
			if c.args == nil {
				t.Assert(len(args), 0)
			} else {
				t.AssertEQ(args, c.args)
			}
		}
	})
}

func TestDsl_Funcs_Aggregate(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		cases := []struct {
			expr Expression
			sql  string
		}{
			{CountFunc(testUser.ID), "COUNT(`id`)"},
			{SumFunc(testUser.Age), "SUM(`age`)"},
			{AvgFunc(testUser.Age), "AVG(`age`)"},
			{MinFunc(testUser.Age), "MIN(`age`)"},
			{MaxFunc(testUser.Age), "MAX(`age`)"},
			{CountDistinctFunc(testUser.Status), "COUNT(DISTINCT `status`)"},
		}
		for _, c := range cases {
			sql, _, err := Select(c.expr).From(testUser).ToSql(DialectMySQL)
			t.AssertNil(err)
			t.Assert(sql, "SELECT "+c.sql+" FROM `user` WHERE `deleted_at` IS NULL")
		}
	})
}

func TestDsl_Funcs_DateFormat(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Select(DateFormatFunc(testUser.CreatedAt, "%Y-%m-%d")).From(testUser).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT DATE_FORMAT(`created_at`, '%Y-%m-%d') FROM `user` WHERE `deleted_at` IS NULL")
		t.AssertEQ(args, nil)

		sql, _, err = Select(DateFormatFunc(testUser.CreatedAt, "%Y-%m-%d")).From(testUser).ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT TO_CHAR("created_at", 'YYYY-MM-DD') FROM "user" WHERE "deleted_at" IS NULL`)

		sql, _, err = Select(DateFormatFunc(testUser.CreatedAt, "%Y-%m-%d")).From(testUser).ToSql(DialectSQLite)
		t.AssertNil(err)
		t.Assert(sql, `SELECT strftime('%Y-%m-%d', "created_at") FROM "user" WHERE "deleted_at" IS NULL`)
	})
}

func TestDsl_Funcs_GroupConcat(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Select(GroupConcatFunc(GroupConcatOptions{Field: testUser.Name})).
			From(testUser).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT GROUP_CONCAT(`name`) FROM `user` WHERE `deleted_at` IS NULL")

		sql, _, err = Select(GroupConcatFunc(GroupConcatOptions{
			Field: testUser.Name, Separator: "-", OrderBy: []OrderClause{testUser.Name.Asc()},
		})).From(testUser).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT GROUP_CONCAT(`name` ORDER BY `name` ASC SEPARATOR '-') FROM `user` WHERE `deleted_at` IS NULL")

		sql, _, err = Select(GroupConcatFunc(GroupConcatOptions{
			Field: testUser.Name, Distinct: true,
		})).From(testUser).ToSql(DialectSQLite)
		t.AssertNil(err)
		t.Assert(sql, `SELECT GROUP_CONCAT(DISTINCT "name", ',') FROM "user" WHERE "deleted_at" IS NULL`)

		sql, _, err = Select(GroupConcatFunc(GroupConcatOptions{Field: testUser.Name})).
			From(testUser).ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT STRING_AGG("name", ',') FROM "user" WHERE "deleted_at" IS NULL`)

		_, _, err = Select(GroupConcatFunc(GroupConcatOptions{Field: testUser.Name, Distinct: true})).
			From(testUser).ToSql(DialectPgsql)
		t.AssertNE(err, nil)
	})
}

func TestDsl_Funcs_Window(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Select(
			RankFunc().Over([]Expression{testUser.Status}, []OrderClause{testUser.Age.Desc()}).
				As("r"),
		).From(testUser).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT RANK() OVER (PARTITION BY `status` ORDER BY `age` DESC) AS r FROM `user` WHERE `deleted_at` IS NULL")

		sql, _, err = Select(
			RowNumberFunc().Over(nil, []OrderClause{testUser.ID.Asc()}),
		).From(testUser).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT ROW_NUMBER() OVER (ORDER BY `id` ASC) FROM `user` WHERE `deleted_at` IS NULL")

		sql, _, err = Select(
			SumFunc(testUser.Age).OverFrame(
				[]Expression{testUser.Status},
				[]OrderClause{testUser.ID.Asc()},
				RowsFrame("UNBOUNDED PRECEDING", "CURRENT ROW"),
			),
		).From(testUser).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT SUM(`age`) OVER (PARTITION BY `status` ORDER BY `id` ASC ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) FROM `user` WHERE `deleted_at` IS NULL")
	})
}

func TestDsl_Arith(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		cases := []struct {
			expr Expression
			sql  string
			args []any
		}{
			{testUser.Age.Add(1), "(`age` + ?)", []any{1}},
			{testUser.Age.Sub(1), "(`age` - ?)", []any{1}},
			{testUser.Age.Mul(2), "(`age` * ?)", []any{2}},
			{testUser.Age.Div(2), "(`age` / ?)", []any{2}},
			{Add(testUser.Age, Mul(2, testUser.Age)), "(`age` + (? * `age`))", []any{2}},
			{Negate(testUser.Age), "(-`age`)", nil},
		}
		for _, c := range cases {
			sql, args, err := Select(c.expr).From(testUser).ToSql(DialectMySQL)
			t.AssertNil(err)
			t.Assert(sql, "SELECT "+c.sql+" FROM `user` WHERE `deleted_at` IS NULL")
			if c.args == nil {
				t.Assert(len(args), 0)
			} else {
				t.AssertEQ(args, c.args)
			}
		}
	})
}

func TestDsl_Case(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Select(
			Case().When(testUser.Age.Gt(60)).Then(Str("old")).
				When(testUser.Age.Gt(18)).Then(Str("adult")).
				Else(Str("young")).
				End().As("bucket"),
		).From(testUser).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT CASE WHEN `age` > ? THEN 'old' WHEN `age` > ? THEN 'adult' ELSE 'young' END AS bucket FROM `user` WHERE `deleted_at` IS NULL")
		t.AssertEQ(args, []any{60, 18})
	})
}

func TestDsl_Raw(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Select(Raw("JSON_EXTRACT(data, ?)", "$.name")).From(testUser).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT JSON_EXTRACT(data, ?) FROM `user` WHERE `deleted_at` IS NULL")
		t.AssertEQ(args, []any{"$.name"})

		sql, args, err = Select(testUser.ID).From(testUser).
			Where(Raw("age > ?", 18)).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `id` FROM `user` WHERE age > ? AND `deleted_at` IS NULL")
		t.AssertEQ(args, []any{18})
	})
}

func TestDsl_OperatorFunc(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		OperatorFunc("JSON_EXTRACT", func(ctx context.Context, args ...any) (string, []any, error) {
			return fmt.Sprintf("JSON_EXTRACT(%s, %s)", args[0], args[1]), nil, nil
		}, "mysql")
		sql, _, err := Select(Func("JSON_EXTRACT", testUser.Name, Str("$.key"))).From(testUser).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT JSON_EXTRACT(`name`, '$.key') FROM `user` WHERE `deleted_at` IS NULL")
	})
}

func TestDsl_Placeholder_Dialects(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Select(testUser.ID).From(testUser).
			Where(testUser.Age.Gt(18)).Where(testUser.Status.Eq("vip")).
			ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "id" FROM "user" WHERE "age" > $1 AND "status" = $2 AND "deleted_at" IS NULL`)
		t.AssertEQ(args, []any{18, "vip"})

		sql, args, err = Select(testUser.ID).From(testUser).
			Where(testUser.Age.In(1, 2, 3)).
			ToSql(DialectSQLite)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "id" FROM "user" WHERE "age" IN (?, ?, ?) AND "deleted_at" IS NULL`)
		t.AssertEQ(args, []any{1, 2, 3})
	})
}
