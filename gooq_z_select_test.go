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
		t.Assert(sql, "SELECT `id`, `name` FROM `user` WHERE `age` > ? AND `deleted_at` IS NULL ORDER BY `id` DESC LIMIT 10")
		t.AssertEQ(args, []any{18})

		sql, _, err = Select(testUser.ID).From(testUser).Offset(20).Limit(10).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `id` FROM `user` WHERE `deleted_at` IS NULL LIMIT 10 OFFSET 20")

		sql, _, err = Select(testUser.ID).From(testUser).Page(2, 10).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `id` FROM `user` WHERE `deleted_at` IS NULL LIMIT 10 OFFSET 10")

		sql, _, err = Select(testUser.ID).From(testUser).Distinct().ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT DISTINCT `id` FROM `user` WHERE `deleted_at` IS NULL")

		sql, _, err = Select(testUser.ID).From(testUser).ToSql()
		t.AssertNil(err)
		t.Assert(sql, "SELECT `id` FROM `user` WHERE `deleted_at` IS NULL")
	})
}

func TestDsl_Select_AllFields_FieldsEx(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Select(testUser.AllFields()).From(testUser).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `id`, `name`, `age`, `status`, `created_at`, `deleted_at` FROM `user` WHERE `deleted_at` IS NULL")

		sql, _, err = Select(testUser.ID).From(testUser).FieldsEx(testUser.CreatedAt, testUser.DeletedAt).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `id`, `name`, `age`, `status` FROM `user` WHERE `deleted_at` IS NULL")
	})
}

func TestDsl_Select_SoftDelete(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Select(testUser.ID).From(testUser).Unscoped().ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `id` FROM `user`")

		sql, _, err = Select(testUser.ID).From(testUser).
			Where(testUser.DeletedAt.IsNull()).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `id` FROM `user` WHERE `deleted_at` IS NULL")

		sql, _, err = Select(testUserRole.ID).From(testUserRole).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `id` FROM `user_role`")
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

func TestDsl_Select_Subquery(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sub := Select(testRole.ID).From(testRole).Where(testRole.Name.Eq("admin"))

		sql, args, err := Select(testUser.ID).From(testUser).
			Where(testUser.ID.InExpr(sub)).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `id` FROM `user` WHERE `id` IN (SELECT `id` FROM `role` WHERE `name` = ? AND `deleted_at` IS NULL) AND `deleted_at` IS NULL")
		t.AssertEQ(args, []any{"admin"})

		sql, _, err = Select(testUser.ID).From(testUser).
			Where(Exists(sub)).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `id` FROM `user` WHERE EXISTS (SELECT `id` FROM `role` WHERE `name` = ? AND `deleted_at` IS NULL) AND `deleted_at` IS NULL")

		sql, _, err = Select(testUser.ID).From(testUser).
			Where(NotExists(sub)).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `id` FROM `user` WHERE NOT EXISTS (SELECT `id` FROM `role` WHERE `name` = ? AND `deleted_at` IS NULL) AND `deleted_at` IS NULL")

		sql, _, err = Select(testUser.ID).From(Select(testUser.ID).From(testUser).Where(testUser.Age.Gt(18)).As("t")).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `id` FROM (SELECT `id` FROM `user` WHERE `age` > ? AND `deleted_at` IS NULL) AS t")
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
		t.Assert(sql, "SELECT `status`, COUNT(`id`) FROM `user` WHERE `deleted_at` IS NULL GROUP BY `status` HAVING COUNT(`id`) > ?")
		t.AssertEQ(args, []any{2})

		sql, _, err = Select(testUser.Status).From(testUser).
			GroupRollup(testUser.Status).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `status` FROM `user` WHERE `deleted_at` IS NULL GROUP BY `status` WITH ROLLUP")

		sql, _, err = Select(testUser.Status).From(testUser).
			GroupCube(testUser.Status).
			ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "status" FROM "user" WHERE "deleted_at" IS NULL GROUP BY CUBE("status")`)

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
		t.Assert(sql, "SELECT `id` FROM `user` WHERE `age` = ? AND `deleted_at` IS NULL UNION ALL SELECT `id` FROM `user` WHERE `age` = ? AND `deleted_at` IS NULL")

		sql, _, err = Select(testUser.ID).From(testUser).
			Intersect(Select(testUser.ID).From(testUser)).
			Except(Select(testUser.ID).From(testUser)).
			ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "id" FROM "user" WHERE "deleted_at" IS NULL INTERSECT SELECT "id" FROM "user" WHERE "deleted_at" IS NULL EXCEPT SELECT "id" FROM "user" WHERE "deleted_at" IS NULL`)
	})
}

func TestDsl_Select_Cte(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := With("adults", Select(testUser.ID).From(testUser).Where(testUser.Age.Gt(18))).
			Fields(testUser.ID).From(Cte("adults")).
			ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `WITH adults AS (SELECT "id" FROM "user" WHERE "age" > $1 AND "deleted_at" IS NULL) SELECT "id" FROM "adults"`)

		sql, _, err = WithRecursive("t", Select(testUser.ID).From(testUser).Where(testUser.ID.Eq(1))).
			Fields(testUser.ID).From(Cte("t")).
			ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `WITH RECURSIVE t AS (SELECT "id" FROM "user" WHERE "id" = $1 AND "deleted_at" IS NULL) SELECT "id" FROM "t"`)
	})
}

func TestDsl_Select_Lock(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Select(testUser.ID).From(testUser).LockForUpdate().ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `id` FROM `user` WHERE `deleted_at` IS NULL FOR UPDATE")

		sql, _, err = Select(testUser.ID).From(testUser).LockInShareMode().ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `id` FROM `user` WHERE `deleted_at` IS NULL LOCK IN SHARE MODE")

		sql, _, err = Select(testUser.ID).From(testUser).LockInShareMode().ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "id" FROM "user" WHERE "deleted_at" IS NULL FOR SHARE`)

		sql, _, err = Select(testUser.ID).From(testUser).LockForUpdate().ToSql(DialectSQLite)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "id" FROM "user" WHERE "deleted_at" IS NULL`)
	})
}

func TestDsl_Select_Conditions(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Select(testUser.ID).From(testUser).
			Where(OR(testUser.Age.Lt(18), testUser.Status.Eq("vip"))).
			And(testUser.Name.Like("j%")).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `id` FROM `user` WHERE (`age` < ? OR `status` = ?) AND `name` LIKE ? AND `deleted_at` IS NULL")
		t.AssertEQ(args, []any{18, "vip", "j%"})

		sql, _, err = Select(testUser.ID).From(testUser).
			Where(NOT(testUser.Status.Eq("banned"))).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `id` FROM `user` WHERE (NOT `status` = ?) AND `deleted_at` IS NULL")

		sql, args, err = Select(testUser.ID).From(testUser).
			Where(testUser.Age.Between(18, 60)).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `id` FROM `user` WHERE `age` BETWEEN ? AND ? AND `deleted_at` IS NULL")
		t.AssertEQ(args, []any{18, 60})
	})
}

func TestDsl_Select_OrderNulls(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Select(testUser.ID).From(testUser).
			Order(testUser.Age.Desc().NullsLast()).
			ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "id" FROM "user" WHERE "deleted_at" IS NULL ORDER BY "age" DESC NULLS LAST`)

		sql, _, err = Select(testUser.ID).From(testUser).
			Order(testUser.Age.Desc().NullsLast()).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `id` FROM `user` WHERE `deleted_at` IS NULL ORDER BY `age` DESC")
	})
}

func TestDsl_Select_Clone(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		base := Select(testUser.ID).From(testUser)
		sql1, _, err := base.Clone().Where(testUser.Age.Gt(18)).ToSql(DialectMySQL)
		t.AssertNil(err)
		sql2, _, err := base.Clone().Where(testUser.Status.Eq("vip")).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql1, "SELECT `id` FROM `user` WHERE `age` > ? AND `deleted_at` IS NULL")
		t.Assert(sql2, "SELECT `id` FROM `user` WHERE `status` = ? AND `deleted_at` IS NULL")
	})
}
