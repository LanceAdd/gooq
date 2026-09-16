// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件为表达式层单元测试：算术、CASE、RAW、CAST、自定义节点与方言占位符（直接渲染，不经构建器）。
package gooq

import (
	"testing"

	"github.com/gogf/gf/v2/test/gtest"
)

// renderExpr 以指定方言渲染表达式并返回 SQL 与参数。
func renderExpr(dialect Dialect, e Expression) (string, []any) {
	return NewRenderContext(dialect).Render(e)
}

func TestDsl_Arith(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		cases := []struct {
			expr Expression
			sql  string
			args []any
		}{
			{testUser.Age.Add(1), "(`user`.`age` + ?)", []any{1}},
			{testUser.Age.Sub(1), "(`user`.`age` - ?)", []any{1}},
			{testUser.Age.Mul(2), "(`user`.`age` * ?)", []any{2}},
			{testUser.Age.Div(2), "(`user`.`age` / ?)", []any{2}},
			{Add(testUser.Age, Mul(2, testUser.Age)), "(`user`.`age` + (? * `user`.`age`))", []any{2}},
			{Negate(testUser.Age), "(-`user`.`age`)", nil},
		}
		for _, c := range cases {
			sql, args := renderExpr(DialectMySQL, c.expr)
			t.Assert(sql, c.sql)
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
		sql, args := renderExpr(DialectMySQL, Case().
			When(testUser.Age.Gt(60)).Then(Str("old")).
			When(testUser.Age.Gt(18)).Then(Str("adult")).
			Else(Str("young")).
			End().As("bucket"))
		t.Assert(sql, "CASE WHEN `user`.`age` > ? THEN 'old' WHEN `user`.`age` > ? THEN 'adult' ELSE 'young' END AS bucket")
		t.AssertEQ(args, []any{60, 18})
	})
}

func TestDsl_Raw(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args := renderExpr(DialectMySQL, Raw("JSON_EXTRACT(data, ?)", "$.name"))
		t.Assert(sql, "JSON_EXTRACT(data, ?)")
		t.AssertEQ(args, []any{"$.name"})
	})
}

func TestDsl_Cast(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _ := renderExpr(DialectMySQL, testUser.Age.Cast(LocalTypeString))
		t.Assert(sql, "CAST(`user`.`age` AS CHAR)")

		sql, _ = renderExpr(DialectPgsql, Cast(testUser.Age, LocalTypeInt64))
		t.Assert(sql, `CAST("user"."age" AS BIGINT)`)

		sql, args := renderExpr(DialectSQLite, Eq(testUser.Age.Cast(LocalTypeString), Str("18")))
		t.Assert(sql, `CAST("user"."age" AS TEXT) = '18'`)
		t.Assert(len(args), 0)

		sql, _ = renderExpr(DialectMySQL, testUser.CreatedAt.Cast(LocalTypeDatetime))
		t.Assert(sql, "CAST(`user`.`created_at` AS DATETIME)")
	})
}

// pgAwareConcat 是自定义节点示例：实现 Expression + Renderer（方言分支）即可参与渲染。
type pgAwareConcat struct {
	a, b Expression
}

func (c pgAwareConcat) Condition() (string, []any) {
	return c.Render(NewRenderContext(DialectMySQL))
}

func (c pgAwareConcat) Render(rc *RenderContext) (string, []any) {
	aSQL, aArgs := rc.Render(c.a)
	bSQL, bArgs := rc.Render(c.b)
	if rc.Dialect() == DialectPgsql {
		return "(" + aSQL + " || " + bSQL + ")", append(aArgs, bArgs...)
	}
	return "CONCAT(" + aSQL + ", " + bSQL + ")", append(aArgs, bArgs...)
}

func (c pgAwareConcat) SubExpressions() []Expression {
	return []Expression{c.a, c.b}
}

func TestDsl_CustomNode(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _ := renderExpr(DialectMySQL, pgAwareConcat{a: testUser.Name, b: Str("-x")})
		t.Assert(sql, "CONCAT(`user`.`name`, '-x')")

		sql, _ = renderExpr(DialectPgsql, pgAwareConcat{a: testUser.Name, b: Str("-x")})
		t.Assert(sql, `("user"."name" || '-x')`)
	})
}

func TestDsl_Placeholder_Dialects(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args := renderExpr(DialectPgsql, AND(testUser.Age.Gt(18), testUser.Status.Eq("vip")))
		t.Assert(sql, `("user"."age" > $1 AND "user"."status" = $2)`)
		t.AssertEQ(args, []any{18, "vip"})

		sql, args = renderExpr(DialectSQLite, testUser.Age.In(1, 2, 3))
		t.Assert(sql, `"user"."age" IN (?, ?, ?)`)
		t.AssertEQ(args, []any{1, 2, 3})
	})
}
