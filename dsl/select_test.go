// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件为 SELECT 渲染单元测试：条件/软删/JOIN/子查询/分组/集合/CTE/锁/分页/窗口。
package dsl

import (
	"testing"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/fn"

	"github.com/gogf/gf/v2/test/gtest"
)

func TestDsl_Select_Basic(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Select(testUser.Id, testUser.Name).
			From(testUser).
			Where(testUser.Age.Gt(18)).
			Order(testUser.Id.Desc()).
			Limit(10).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id`, `user`.`name` FROM `user` WHERE `user`.`age` > ? AND `user`.`deleted_at` IS NULL ORDER BY `user`.`id` DESC LIMIT 10")
		t.AssertEQ(args, []any{18})

		sql, _, err = Select(testUser.Id).From(testUser).Offset(20).Limit(10).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`deleted_at` IS NULL LIMIT 10 OFFSET 20")

		sql, _, err = Select(testUser.Id).From(testUser).Page(2, 10).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`deleted_at` IS NULL LIMIT 10 OFFSET 10")

		sql, _, err = Select(testUser.Id).From(testUser).Distinct().ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT DISTINCT `user`.`id` FROM `user` WHERE `user`.`deleted_at` IS NULL")

		sql, _, err = Select(testUser.Id).From(testUser).ToSql()
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`deleted_at` IS NULL")
	})
}

func TestDsl_Select_AllFields_FieldsEx(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Select(testUser.AllFields()).From(testUser).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id`, `user`.`name`, `user`.`age`, `user`.`status`, `user`.`created_at`, `user`.`deleted_at` FROM `user` WHERE `user`.`deleted_at` IS NULL")

		sql, _, err = Select(testUser.Id).From(testUser).FieldsEx(testUser.CreatedAt, testUser.DeletedAt).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id`, `user`.`name`, `user`.`age`, `user`.`status` FROM `user` WHERE `user`.`deleted_at` IS NULL")
	})
}

func TestDsl_Schema(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		su := newSchemaUserTable("")

		sql, _, err := Select(su.Id, su.Name).From(su).Where(su.Id.Eq(1)).ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "public"."user"."id", "public"."user"."name" FROM "public"."user" WHERE "public"."user"."id" = $1`)

		sql, _, err = Select(su.Id).From(su).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `public`.`user`.`id` FROM `public`.`user`")

		// 别名遮蔽 schema：字段用别名前缀。
		u1 := su.As("u1")
		sql, _, err = Select(u1.Id).From(u1).ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "u1"."id" FROM "public"."user" AS u1`)

		// DML：INSERT 与 DELETE 目标表带 schema 限定。
		sql, _, err = Insert(su).Columns(su.Name).Values("a").ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `INSERT INTO "public"."user" ("name") VALUES ($1)`)

		sql, _, err = Delete(su).Where(su.Id.Eq(1)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "DELETE FROM `public`.`user` WHERE `public`.`user`.`id` = ?")
	})
}

func TestDsl_Select_SoftDelete(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Select(testUser.Id).From(testUser).Unscoped().ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user`")

		sql, _, err = Select(testUser.Id).From(testUser).
			Where(testUser.DeletedAt.IsNull()).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`deleted_at` IS NULL")

		sql, _, err = Select(testUserRole.Id).From(testUserRole).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user_role`.`id` FROM `user_role`")
	})
}

func TestDsl_Select_Join(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		u := testUser.As("u")
		ur := testUserRole.As("ur")
		r := testRole.As("r")

		sql, args, err := Select(u.Id, r.Name).
			From(u).
			InnerJoin(ur).On(ur.UserId.EqExpr(u.Id)).
			InnerJoin(r).On(r.Id.EqExpr(ur.RoleId)).
			Where(r.Name.Eq("admin")).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `u`.`id`, `r`.`name` FROM `user` AS u INNER JOIN `user_role` AS ur ON `ur`.`user_id` = `u`.`id` INNER JOIN `role` AS r ON `r`.`id` = `ur`.`role_id` WHERE `r`.`name` = ? AND `u`.`deleted_at` IS NULL")
		t.AssertEQ(args, []any{"admin"})

		sql, _, err = Select(u.Id).From(u).
			LeftJoin(r).On(r.Id.EqExpr(u.Id)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `u`.`id` FROM `user` AS u LEFT JOIN `role` AS r ON `r`.`id` = `u`.`id` WHERE `u`.`deleted_at` IS NULL")

		// 无别名 JOIN：字段自动带表名前缀（避免同名列冲突）。
		sql, _, err = Select(testUser.Id).From(testUser).
			InnerJoin(testRole).On(testRole.Id.EqExpr(testUser.Id)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` INNER JOIN `role` ON `role`.`id` = `user`.`id` WHERE `user`.`deleted_at` IS NULL")
	})
}

func TestDsl_Select_Lateral(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		u := testUser.As("u")
		lt := Select(testCount(testUserRole.UserId).As("cnt")).
			From(testUserRole).Where(testUserRole.UserId.EqExpr(u.Id)).As("lt")

		sql, _, err := Select(u.Id, lt.Field("cnt")).From(u).
			LeftJoinLateral(lt).On(gooq.Raw("1 = 1")).
			ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "u"."id", "lt"."cnt" FROM "user" AS u LEFT JOIN LATERAL (SELECT COUNT("user_role"."user_id") AS cnt FROM "user_role" WHERE "user_role"."user_id" = "u"."id") AS lt ON 1 = 1 WHERE "u"."deleted_at" IS NULL`)

		sql, _, err = Select(u.Id, lt.Field("cnt")).From(u).
			InnerJoinLateral(lt).On(gooq.Raw("1 = 1")).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `u`.`id`, `lt`.`cnt` FROM `user` AS u INNER JOIN LATERAL (SELECT COUNT(`user_role`.`user_id`) AS cnt FROM `user_role` WHERE `user_role`.`user_id` = `u`.`id`) AS lt ON 1 = 1 WHERE `u`.`deleted_at` IS NULL")

		// SQLite：INNER JOIN LATERAL 语法不支持，映射为 CROSS JOIN LATERAL。
		sql, _, err = Select(u.Id, lt.Field("cnt")).From(u).
			InnerJoinLateral(lt).On(gooq.Raw("1 = 1")).
			ToSql(gooq.DialectSQLite)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "u"."id", "lt"."cnt" FROM "user" AS u CROSS JOIN LATERAL (SELECT COUNT("user_role"."user_id") AS cnt FROM "user_role" WHERE "user_role"."user_id" = "u"."id") AS lt ON 1 = 1 WHERE "u"."deleted_at" IS NULL`)

		sql, _, err = Select(u.Id, lt.Field("cnt")).From(u).
			CrossJoinLateral(lt).
			ToSql(gooq.DialectSQLite)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "u"."id", "lt"."cnt" FROM "user" AS u CROSS JOIN LATERAL (SELECT COUNT("user_role"."user_id") AS cnt FROM "user_role" WHERE "user_role"."user_id" = "u"."id") AS lt WHERE "u"."deleted_at" IS NULL`)
	})
}

func TestDsl_Select_SelfJoin(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		// 自连接：同一表的两个别名实例，字段随实例解析各自前缀。
		u1 := testUser.As("u1")
		u2 := testUser.As("u2")
		sql, _, err := Select(u1.Id, u2.Id).
			From(u1).
			InnerJoin(u2).On(u1.Id.EqExpr(u2.Id)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `u1`.`id`, `u2`.`id` FROM `user` AS u1 INNER JOIN `user` AS u2 ON `u1`.`id` = `u2`.`id` WHERE `u1`.`deleted_at` IS NULL")
	})
}

func TestDsl_Select_Subquery(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sub := Select(testRole.Id).From(testRole).Where(testRole.Name.Eq("admin"))

		sql, args, err := Select(testUser.Id).From(testUser).
			Where(testUser.Id.InExpr(sub)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`id` IN (SELECT `role`.`id` FROM `role` WHERE `role`.`name` = ? AND `role`.`deleted_at` IS NULL) AND `user`.`deleted_at` IS NULL")
		t.AssertEQ(args, []any{"admin"})

		sql, _, err = Select(testUser.Id).From(testUser).
			Where(gooq.Exists(sub)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE EXISTS (SELECT `role`.`id` FROM `role` WHERE `role`.`name` = ? AND `role`.`deleted_at` IS NULL) AND `user`.`deleted_at` IS NULL")

		sql, _, err = Select(testUser.Id).From(testUser).
			Where(gooq.NotExists(sub)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE NOT EXISTS (SELECT `role`.`id` FROM `role` WHERE `role`.`name` = ? AND `role`.`deleted_at` IS NULL) AND `user`.`deleted_at` IS NULL")

		sql, _, err = Select(testUser.Id).From(Select(testUser.Id).From(testUser).Where(testUser.Age.Gt(18)).As("t")).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM (SELECT `user`.`id` FROM `user` WHERE `user`.`age` > ? AND `user`.`deleted_at` IS NULL) AS t")
	})
}

func TestDsl_Select_Correlated(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		// 相关子查询：子查询内引用外层别名，全限定前缀保证列归属。
		u := testUser.As("u")
		ur := testUserRole.As("ur")
		sql, _, err := Select(u.Id).From(u).
			Where(gooq.Exists(
				Select(ur.UserId).From(ur).Where(ur.UserId.EqExpr(u.Id)),
			)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `u`.`id` FROM `user` AS u WHERE EXISTS (SELECT `ur`.`user_id` FROM `user_role` AS ur WHERE `ur`.`user_id` = `u`.`id`) AND `u`.`deleted_at` IS NULL")
	})
}

func TestDsl_Select_Group(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Select(testUser.Status, testCount(testUser.Id)).
			From(testUser).
			Group(testUser.Status).
			Having(gooq.Gt(testCount(testUser.Id), 2)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`status`, COUNT(`user`.`id`) FROM `user` WHERE `user`.`deleted_at` IS NULL GROUP BY `user`.`status` HAVING COUNT(`user`.`id`) > ?")
		t.AssertEQ(args, []any{2})

		sql, _, err = Select(testUser.Status).From(testUser).
			GroupRollup(testUser.Status).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`status` FROM `user` WHERE `user`.`deleted_at` IS NULL GROUP BY `user`.`status` WITH ROLLUP")

		sql, _, err = Select(testUser.Status).From(testUser).
			GroupCube(testUser.Status).
			ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "user"."status" FROM "user" WHERE "user"."deleted_at" IS NULL GROUP BY CUBE("user"."status")`)

		_, _, err = Select(testUser.Status).From(testUser).
			GroupCube(testUser.Status).
			ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
	})
}

func TestDsl_Select_SetOps(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Select(testUser.Id).From(testUser).Where(testUser.Age.Eq(1)).
			UnionAll(Select(testUser.Id).From(testUser).Where(testUser.Age.Eq(2))).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`age` = ? AND `user`.`deleted_at` IS NULL UNION ALL SELECT `user`.`id` FROM `user` WHERE `user`.`age` = ? AND `user`.`deleted_at` IS NULL")

		sql, _, err = Select(testUser.Id).From(testUser).
			Intersect(Select(testUser.Id).From(testUser)).
			Except(Select(testUser.Id).From(testUser)).
			ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "user"."id" FROM "user" WHERE "user"."deleted_at" IS NULL INTERSECT SELECT "user"."id" FROM "user" WHERE "user"."deleted_at" IS NULL EXCEPT SELECT "user"."id" FROM "user" WHERE "user"."deleted_at" IS NULL`)
	})
}

func TestDsl_Select_Cte(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := With("adults", Select(testUser.Id).From(testUser).Where(testUser.Age.Gt(18))).
			Fields(Cte("adults").Field("id")).From(Cte("adults")).
			ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `WITH adults AS (SELECT "user"."id" FROM "user" WHERE "user"."age" > $1 AND "user"."deleted_at" IS NULL) SELECT "adults"."id" FROM "adults"`)

		sql, _, err = WithRecursive("t", Select(testUser.Id).From(testUser).Where(testUser.Id.Eq(1))).
			Fields(Cte("t").Field("id")).From(Cte("t")).
			ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `WITH RECURSIVE t AS (SELECT "user"."id" FROM "user" WHERE "user"."id" = $1 AND "user"."deleted_at" IS NULL) SELECT "t"."id" FROM "t"`)
	})
}

func TestDsl_Select_Lock(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Select(testUser.Id).From(testUser).LockForUpdate().ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`deleted_at` IS NULL FOR UPDATE")

		sql, _, err = Select(testUser.Id).From(testUser).LockInShareMode().ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`deleted_at` IS NULL LOCK IN SHARE MODE")

		sql, _, err = Select(testUser.Id).From(testUser).LockInShareMode().ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "user"."id" FROM "user" WHERE "user"."deleted_at" IS NULL FOR SHARE`)

		sql, _, err = Select(testUser.Id).From(testUser).LockForUpdate().ToSql(gooq.DialectSQLite)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "user"."id" FROM "user" WHERE "user"."deleted_at" IS NULL`)
	})
}

func TestDsl_Select_Conditions(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Select(testUser.Id).From(testUser).
			Where(gooq.OR(testUser.Age.Lt(18), testUser.Status.Eq("vip"))).
			And(testUser.Name.Like("j%")).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE (`user`.`age` < ? OR `user`.`status` = ?) AND `user`.`name` LIKE ? AND `user`.`deleted_at` IS NULL")
		t.AssertEQ(args, []any{18, "vip", "j%"})

		sql, _, err = Select(testUser.Id).From(testUser).
			Where(gooq.NOT(testUser.Status.Eq("banned"))).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE (NOT `user`.`status` = ?) AND `user`.`deleted_at` IS NULL")

		sql, args, err = Select(testUser.Id).From(testUser).
			Where(testUser.Age.Between(18, 60)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`age` BETWEEN ? AND ? AND `user`.`deleted_at` IS NULL")
		t.AssertEQ(args, []any{18, 60})
	})
}

// TestDsl_Select_OrderRaw 验证 Raw 作为排序逃生舱：片段原样渲染（方向/NULLS 自定，不追加任何后缀）。
func TestDsl_Select_OrderRaw(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Select(testUser.Id).From(testUser).
			Order(gooq.Raw("`user`.`age` DESC NULLS LAST")).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`deleted_at` IS NULL ORDER BY `user`.`age` DESC NULLS LAST")

		sql, _, err = Select(testUser.Id).From(testUser).
			Order(gooq.Raw("RAND()")).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`deleted_at` IS NULL ORDER BY RAND()")
	})
}

func TestDsl_Select_Clone(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		base := Select(testUser.Id).From(testUser)
		sql1, _, err := base.Clone().Where(testUser.Age.Gt(18)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		sql2, _, err := base.Clone().Where(testUser.Status.Eq("vip")).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql1, "SELECT `user`.`id` FROM `user` WHERE `user`.`age` > ? AND `user`.`deleted_at` IS NULL")
		t.Assert(sql2, "SELECT `user`.`id` FROM `user` WHERE `user`.`status` = ? AND `user`.`deleted_at` IS NULL")
	})
}

// TestDsl_Select_OrderExpr 验证表达式排序：方向打包进表达式，Order/OrderAsc/OrderDesc 参数统一为 Expression。
func TestDsl_Select_OrderExpr(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Select(testUser.Id).From(testUser).
			OrderDesc(gooq.Raw("ABS(`user`.`age`)")).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`deleted_at` IS NULL ORDER BY ABS(`user`.`age`) DESC")

		// Order 原样 + 链式追加混用；字段方向来自 Field.Desc()。
		sql, _, err = Select(testUser.Id).From(testUser).
			Order(testUser.Age.Desc()).
			OrderAsc(gooq.Raw("ABS(`user`.`age`)")).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`deleted_at` IS NULL ORDER BY `user`.`age` DESC, ABS(`user`.`age`) ASC")

		// 聚合排序：gooq.OrderDescExpr 包装任意表达式。
		sql, _, err = Select(testUser.Status, fn.Count(testUser.Id)).From(testUser).
			Group(testUser.Status).
			Order(gooq.OrderDescExpr(fn.Count(testUser.Id))).
			ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "user"."status", COUNT("user"."id") FROM "user" WHERE "user"."deleted_at" IS NULL GROUP BY "user"."status" ORDER BY COUNT("user"."id") DESC`)
	})
}
