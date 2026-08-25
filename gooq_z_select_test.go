// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件为 SELECT 渲染单元测试：条件/软删/JOIN/子查询/分组/集合/CTE/锁/分页/窗口。
package gooq

import (
	"testing"

	"github.com/gogf/gf/v2/test/gtest"
)

func TestDsl_Select_Basic(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Select(testUser.ID, testUser.Name).
			From(testUser).
			Where(testUser.Age.Gt(18)).
			Order(testUser.ID.Desc()).
			Limit(10).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id`, `user`.`name` FROM `user` WHERE `user`.`age` > ? AND `user`.`deleted_at` IS NULL ORDER BY `user`.`id` DESC LIMIT 10")
		t.AssertEQ(args, []any{18})

		sql, _, err = Select(testUser.ID).From(testUser).Offset(20).Limit(10).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`deleted_at` IS NULL LIMIT 10 OFFSET 20")

		sql, _, err = Select(testUser.ID).From(testUser).Page(2, 10).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`deleted_at` IS NULL LIMIT 10 OFFSET 10")

		sql, _, err = Select(testUser.ID).From(testUser).Distinct().ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT DISTINCT `user`.`id` FROM `user` WHERE `user`.`deleted_at` IS NULL")

		sql, _, err = Select(testUser.ID).From(testUser).ToSql()
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`deleted_at` IS NULL")
	})
}

func TestDsl_Select_AllFields_FieldsEx(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Select(testUser.AllFields()).From(testUser).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id`, `user`.`name`, `user`.`age`, `user`.`status`, `user`.`created_at`, `user`.`deleted_at` FROM `user` WHERE `user`.`deleted_at` IS NULL")

		sql, _, err = Select(testUser.ID).From(testUser).FieldsEx(testUser.CreatedAt, testUser.DeletedAt).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id`, `user`.`name`, `user`.`age`, `user`.`status` FROM `user` WHERE `user`.`deleted_at` IS NULL")
	})
}

func TestDsl_Schema(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		su := newSchemaUserTable("")

		sql, _, err := Select(su.ID, su.Name).From(su).Where(su.ID.Eq(1)).ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "public"."user"."id", "public"."user"."name" FROM "public"."user" WHERE "public"."user"."id" = $1`)

		sql, _, err = Select(su.ID).From(su).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `public`.`user`.`id` FROM `public`.`user`")

		// 别名遮蔽 schema：字段用别名前缀。
		u1 := su.As("u1")
		sql, _, err = Select(u1.ID).From(u1).ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "u1"."id" FROM "public"."user" AS u1`)

		// DML：INSERT 与 DELETE 目标表带 schema 限定。
		sql, _, err = Insert(su).Columns(su.Name).Values("a").ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `INSERT INTO "public"."user" ("name") VALUES ($1)`)

		sql, _, err = Delete(su).Where(su.ID.Eq(1)).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "DELETE FROM `public`.`user` WHERE `public`.`user`.`id` = ?")
	})
}

func TestDsl_Select_SoftDelete(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Select(testUser.ID).From(testUser).Unscoped().ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user`")

		sql, _, err = Select(testUser.ID).From(testUser).
			Where(testUser.DeletedAt.IsNull()).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`deleted_at` IS NULL")

		sql, _, err = Select(testUserRole.ID).From(testUserRole).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user_role`.`id` FROM `user_role`")
	})
}

func TestDsl_Select_Join(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		u := testUser.As("u")
		ur := testUserRole.As("ur")
		r := testRole.As("r")

		sql, args, err := Select(u.ID, r.Name).
			From(u).
			InnerJoin(ur).On(ur.UserID.EqExpr(u.ID)).
			InnerJoin(r).On(r.ID.EqExpr(ur.RoleID)).
			Where(r.Name.Eq("admin")).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `u`.`id`, `r`.`name` FROM `user` AS u INNER JOIN `user_role` AS ur ON `ur`.`user_id` = `u`.`id` INNER JOIN `role` AS r ON `r`.`id` = `ur`.`role_id` WHERE `r`.`name` = ? AND `u`.`deleted_at` IS NULL")
		t.AssertEQ(args, []any{"admin"})

		sql, _, err = Select(u.ID).From(u).
			LeftJoin(r).On(r.ID.EqExpr(u.ID)).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `u`.`id` FROM `user` AS u LEFT JOIN `role` AS r ON `r`.`id` = `u`.`id` WHERE `u`.`deleted_at` IS NULL")

		// 无别名 JOIN：字段自动带表名前缀（避免同名列冲突）。
		sql, _, err = Select(testUser.ID).From(testUser).
			InnerJoin(testRole).On(testRole.ID.EqExpr(testUser.ID)).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` INNER JOIN `role` ON `role`.`id` = `user`.`id` WHERE `user`.`deleted_at` IS NULL")
	})
}

func TestDsl_Select_Lateral(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		u := testUser.As("u")
		lt := Select(CountFunc(testUserRole.UserID).As("cnt")).
			From(testUserRole).Where(testUserRole.UserID.EqExpr(u.ID)).As("lt")

		sql, _, err := Select(u.ID, lt.Field("cnt")).From(u).
			LeftJoinLateral(lt).On(Raw("1 = 1")).
			ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "u"."id", "lt"."cnt" FROM "user" AS u LEFT JOIN LATERAL (SELECT COUNT("user_role"."user_id") AS cnt FROM "user_role" WHERE "user_role"."user_id" = "u"."id") AS lt ON 1 = 1 WHERE "u"."deleted_at" IS NULL`)

		sql, _, err = Select(u.ID, lt.Field("cnt")).From(u).
			InnerJoinLateral(lt).On(Raw("1 = 1")).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `u`.`id`, `lt`.`cnt` FROM `user` AS u INNER JOIN LATERAL (SELECT COUNT(`user_role`.`user_id`) AS cnt FROM `user_role` WHERE `user_role`.`user_id` = `u`.`id`) AS lt ON 1 = 1 WHERE `u`.`deleted_at` IS NULL")

		// SQLite：INNER JOIN LATERAL 语法不支持，映射为 CROSS JOIN LATERAL。
		sql, _, err = Select(u.ID, lt.Field("cnt")).From(u).
			InnerJoinLateral(lt).On(Raw("1 = 1")).
			ToSql(DialectSQLite)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "u"."id", "lt"."cnt" FROM "user" AS u CROSS JOIN LATERAL (SELECT COUNT("user_role"."user_id") AS cnt FROM "user_role" WHERE "user_role"."user_id" = "u"."id") AS lt ON 1 = 1 WHERE "u"."deleted_at" IS NULL`)

		sql, _, err = Select(u.ID, lt.Field("cnt")).From(u).
			CrossJoinLateral(lt).
			ToSql(DialectSQLite)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "u"."id", "lt"."cnt" FROM "user" AS u CROSS JOIN LATERAL (SELECT COUNT("user_role"."user_id") AS cnt FROM "user_role" WHERE "user_role"."user_id" = "u"."id") AS lt WHERE "u"."deleted_at" IS NULL`)
	})
}

func TestDsl_Select_SelfJoin(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		// 自连接：同一表的两个别名实例，字段随实例解析各自前缀。
		u1 := testUser.As("u1")
		u2 := testUser.As("u2")
		sql, _, err := Select(u1.ID, u2.ID).
			From(u1).
			InnerJoin(u2).On(u1.ID.EqExpr(u2.ID)).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `u1`.`id`, `u2`.`id` FROM `user` AS u1 INNER JOIN `user` AS u2 ON `u1`.`id` = `u2`.`id` WHERE `u1`.`deleted_at` IS NULL")
	})
}

func TestDsl_Select_Subquery(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sub := Select(testRole.ID).From(testRole).Where(testRole.Name.Eq("admin"))

		sql, args, err := Select(testUser.ID).From(testUser).
			Where(testUser.ID.InExpr(sub)).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`id` IN (SELECT `role`.`id` FROM `role` WHERE `role`.`name` = ? AND `role`.`deleted_at` IS NULL) AND `user`.`deleted_at` IS NULL")
		t.AssertEQ(args, []any{"admin"})

		sql, _, err = Select(testUser.ID).From(testUser).
			Where(Exists(sub)).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE EXISTS (SELECT `role`.`id` FROM `role` WHERE `role`.`name` = ? AND `role`.`deleted_at` IS NULL) AND `user`.`deleted_at` IS NULL")

		sql, _, err = Select(testUser.ID).From(testUser).
			Where(NotExists(sub)).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE NOT EXISTS (SELECT `role`.`id` FROM `role` WHERE `role`.`name` = ? AND `role`.`deleted_at` IS NULL) AND `user`.`deleted_at` IS NULL")

		sql, _, err = Select(testUser.ID).From(Select(testUser.ID).From(testUser).Where(testUser.Age.Gt(18)).As("t")).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM (SELECT `user`.`id` FROM `user` WHERE `user`.`age` > ? AND `user`.`deleted_at` IS NULL) AS t")
	})
}

func TestDsl_Select_Correlated(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		// 相关子查询：子查询内引用外层别名，全限定前缀保证列归属。
		u := testUser.As("u")
		ur := testUserRole.As("ur")
		sql, _, err := Select(u.ID).From(u).
			Where(Exists(
				Select(ur.UserID).From(ur).Where(ur.UserID.EqExpr(u.ID)),
			)).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `u`.`id` FROM `user` AS u WHERE EXISTS (SELECT `ur`.`user_id` FROM `user_role` AS ur WHERE `ur`.`user_id` = `u`.`id`) AND `u`.`deleted_at` IS NULL")
	})
}

func TestDsl_Select_Group(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Select(testUser.Status, CountFunc(testUser.ID)).
			From(testUser).
			Group(testUser.Status).
			Having(Gt(CountFunc(testUser.ID), 2)).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`status`, COUNT(`user`.`id`) FROM `user` WHERE `user`.`deleted_at` IS NULL GROUP BY `user`.`status` HAVING COUNT(`user`.`id`) > ?")
		t.AssertEQ(args, []any{2})

		sql, _, err = Select(testUser.Status).From(testUser).
			GroupRollup(testUser.Status).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`status` FROM `user` WHERE `user`.`deleted_at` IS NULL GROUP BY `user`.`status` WITH ROLLUP")

		sql, _, err = Select(testUser.Status).From(testUser).
			GroupCube(testUser.Status).
			ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "user"."status" FROM "user" WHERE "user"."deleted_at" IS NULL GROUP BY CUBE("user"."status")`)

		_, _, err = Select(testUser.Status).From(testUser).
			GroupCube(testUser.Status).
			ToSql(DialectMySQL)
		t.AssertNE(err, nil)
	})
}

func TestDsl_Select_SetOps(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Select(testUser.ID).From(testUser).Where(testUser.Age.Eq(1)).
			UnionAll(Select(testUser.ID).From(testUser).Where(testUser.Age.Eq(2))).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`age` = ? AND `user`.`deleted_at` IS NULL UNION ALL SELECT `user`.`id` FROM `user` WHERE `user`.`age` = ? AND `user`.`deleted_at` IS NULL")

		sql, _, err = Select(testUser.ID).From(testUser).
			Intersect(Select(testUser.ID).From(testUser)).
			Except(Select(testUser.ID).From(testUser)).
			ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "user"."id" FROM "user" WHERE "user"."deleted_at" IS NULL INTERSECT SELECT "user"."id" FROM "user" WHERE "user"."deleted_at" IS NULL EXCEPT SELECT "user"."id" FROM "user" WHERE "user"."deleted_at" IS NULL`)
	})
}

func TestDsl_Select_Cte(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := With("adults", Select(testUser.ID).From(testUser).Where(testUser.Age.Gt(18))).
			Fields(Cte("adults").Field("id")).From(Cte("adults")).
			ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `WITH adults AS (SELECT "user"."id" FROM "user" WHERE "user"."age" > $1 AND "user"."deleted_at" IS NULL) SELECT "adults"."id" FROM "adults"`)

		sql, _, err = WithRecursive("t", Select(testUser.ID).From(testUser).Where(testUser.ID.Eq(1))).
			Fields(Cte("t").Field("id")).From(Cte("t")).
			ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `WITH RECURSIVE t AS (SELECT "user"."id" FROM "user" WHERE "user"."id" = $1 AND "user"."deleted_at" IS NULL) SELECT "t"."id" FROM "t"`)
	})
}

func TestDsl_Select_Lock(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Select(testUser.ID).From(testUser).LockForUpdate().ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`deleted_at` IS NULL FOR UPDATE")

		sql, _, err = Select(testUser.ID).From(testUser).LockInShareMode().ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`deleted_at` IS NULL LOCK IN SHARE MODE")

		sql, _, err = Select(testUser.ID).From(testUser).LockInShareMode().ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "user"."id" FROM "user" WHERE "user"."deleted_at" IS NULL FOR SHARE`)

		sql, _, err = Select(testUser.ID).From(testUser).LockForUpdate().ToSql(DialectSQLite)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "user"."id" FROM "user" WHERE "user"."deleted_at" IS NULL`)
	})
}

func TestDsl_Select_Conditions(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Select(testUser.ID).From(testUser).
			Where(OR(testUser.Age.Lt(18), testUser.Status.Eq("vip"))).
			And(testUser.Name.Like("j%")).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE (`user`.`age` < ? OR `user`.`status` = ?) AND `user`.`name` LIKE ? AND `user`.`deleted_at` IS NULL")
		t.AssertEQ(args, []any{18, "vip", "j%"})

		sql, _, err = Select(testUser.ID).From(testUser).
			Where(NOT(testUser.Status.Eq("banned"))).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE (NOT `user`.`status` = ?) AND `user`.`deleted_at` IS NULL")

		sql, args, err = Select(testUser.ID).From(testUser).
			Where(testUser.Age.Between(18, 60)).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`age` BETWEEN ? AND ? AND `user`.`deleted_at` IS NULL")
		t.AssertEQ(args, []any{18, 60})
	})
}

func TestDsl_Select_OrderNulls(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Select(testUser.ID).From(testUser).
			Order(testUser.Age.Desc().NullsLast()).
			ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "user"."id" FROM "user" WHERE "user"."deleted_at" IS NULL ORDER BY "user"."age" DESC NULLS LAST`)

		sql, _, err = Select(testUser.ID).From(testUser).
			Order(testUser.Age.Desc().NullsLast()).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`deleted_at` IS NULL ORDER BY `user`.`age` DESC")
	})
}

func TestDsl_Select_Clone(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		base := Select(testUser.ID).From(testUser)
		sql1, _, err := base.Clone().Where(testUser.Age.Gt(18)).ToSql(DialectMySQL)
		t.AssertNil(err)
		sql2, _, err := base.Clone().Where(testUser.Status.Eq("vip")).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql1, "SELECT `user`.`id` FROM `user` WHERE `user`.`age` > ? AND `user`.`deleted_at` IS NULL")
		t.Assert(sql2, "SELECT `user`.`id` FROM `user` WHERE `user`.`status` = ? AND `user`.`deleted_at` IS NULL")
	})
}
