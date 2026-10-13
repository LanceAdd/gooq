package gooq

import (
	"testing"

	"github.com/gogf/gf/v2/test/gtest"
)

func TestConditionChain(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args := renderExpr(DialectMySQL, testUser.Age.Gt(11).Or(testUser.Age.Lt(-1)))
		t.Assert(sql, "(`user`.`age` > ? OR `user`.`age` < ?)")
		t.AssertEQ(args, []any{11, -1})

		sql, args = renderExpr(DialectMySQL, testUser.Name.Gt("m").Or(testUser.Age.Lt(-1)))
		t.Assert(sql, "(`user`.`name` > ? OR `user`.`age` < ?)")
		t.AssertEQ(args, []any{"m", -1})

		sql, args = renderExpr(DialectMySQL, testUser.Age.Gt(11).Or(testUser.Age.Lt(-1)).And(testUser.Status.Eq("vip")))
		t.Assert(sql, "((`user`.`age` > ? OR `user`.`age` < ?) AND `user`.`status` = ?)")
		t.AssertEQ(args, []any{11, -1, "vip"})

		sql, args = renderExpr(DialectMySQL, testUser.Age.Gt(11).Or(testUser.Age.Lt(-1).And(testUser.Status.Eq("vip"))))
		t.Assert(sql, "(`user`.`age` > ? OR (`user`.`age` < ? AND `user`.`status` = ?))")
		t.AssertEQ(args, []any{11, -1, "vip"})

		sql, args = renderExpr(DialectMySQL, testUser.Age.Gt(11).Or(testUser.Age.Lt(-1), testUser.Status.Eq("vip")))
		t.Assert(sql, "(`user`.`age` > ? OR `user`.`age` < ? OR `user`.`status` = ?)")
		t.AssertEQ(args, []any{11, -1, "vip"})

		sql, args = renderExpr(DialectMySQL, testUser.Age.Gt(11).Or(nil, testUser.Age.Lt(-1)))
		t.Assert(sql, "(`user`.`age` > ? OR `user`.`age` < ?)")
		t.AssertEQ(args, []any{11, -1})

		sql, _ = renderExpr(DialectMySQL, testUser.Status.Eq("banned").Not())
		t.Assert(sql, "(NOT `user`.`status` = ?)")

		sql, args = renderExpr(DialectMySQL, OR(testUser.Age.Gt(11), testUser.Age.Lt(-1)).And(testUser.Status.Eq("vip")))
		t.Assert(sql, "((`user`.`age` > ? OR `user`.`age` < ?) AND `user`.`status` = ?)")
		t.AssertEQ(args, []any{11, -1, "vip"})

		sql, args = renderExpr(DialectMySQL, Gt(testUser.Age.Add(1), 11).Or(Lt(testUser.Age, 0)))
		t.Assert(sql, "((`user`.`age` + ?) > ? OR `user`.`age` < ?)")
		t.AssertEQ(args, []any{1, 11, 0})

		sql, args = renderExpr(DialectMySQL, Exists(Raw("(SELECT 1)")).And(testUser.Age.Gt(11)))
		t.Assert(sql, "(EXISTS (SELECT 1) AND `user`.`age` > ?)")
		t.AssertEQ(args, []any{11})

		base := testUser.Age.Gt(11)
		t.Assert(Cond(base), base)
		t.Assert(Cond(nil), nil)
		sql, args = renderExpr(DialectMySQL, Cond(Raw("a = 1")).Or(testUser.Age.Lt(-1)))
		t.Assert(sql, "(a = 1 OR `user`.`age` < ?)")
		t.AssertEQ(args, []any{-1})

		t.Assert(ReferencesColumn([]Expression{testUser.Name.Gt("m").Or(testUser.Age.Lt(-1))}, testUser.TableBase, "age"), true)
		t.Assert(ReferencesColumn([]Expression{testUser.Name.Gt("m").Or(testUser.Age.Lt(-1))}, testUser.TableBase, "id"), false)
	})
}
