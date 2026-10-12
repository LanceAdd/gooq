// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件为 DML 渲染单元测试：Insert/Update/Delete 各入口、批量、Upsert、Returning、软删。
package dsl

import (
	"context"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/test/gtest"

	"github.com/lanceadd/gooq"
)

type assignRow []gooq.Assignment

func (r assignRow) Assignments() []gooq.Assignment { return r }

func setCol(column string, value any) gooq.Assignment {
	return gooq.Assignment{Column: column, Value: value}
}

func nullCol(column string) gooq.Assignment {
	return gooq.Assignment{Column: column, Null: true}
}

func TestDsl_Insert_ValuesOf(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Insert(testUser).ValuesOf(assignRow{
			setCol("name", "john"),
			setCol("age", 0),
		}).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO `user` (`name`, `age`) VALUES (?, ?)")
		t.AssertEQ(args, []any{"john", 0})

		sql, args, err = Insert(testUser).ValuesOf(assignRow{
			setCol("name", "a"),
			nullCol("deleted_at"),
		}).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO `user` (`name`, `deleted_at`) VALUES (?, NULL)")
		t.AssertEQ(args, []any{"a"})

		sql, _, err = Insert(testUser).ValuesOf(assignRow{}).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(sql, "")
	})
}

func TestDsl_Insert_ColumnsValues(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Insert(testUser).
			Columns(testUser.Name, testUser.Age).
			Values("a", 1).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO `user` (`name`, `age`) VALUES (?, ?)")
		t.AssertEQ(args, []any{"a", 1})

		sql, args, err = Insert(testUser).
			Columns(testUser.Name, testUser.Age).
			Values("a", 1).
			Values("b", 2).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO `user` (`name`, `age`) VALUES (?, ?), (?, ?)")
		t.AssertEQ(args, []any{"a", 1, "b", 2})

		sql, _, err = Insert(testUser).Values("a").ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)

		sql, _, err = Insert(testUser).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)

		_, _, err = Insert(testUser).Columns(testUser.Name, testUser.Age).Values("x").ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "does not match"), true)

		_, err = Insert(testUser).Columns(testUser.Name).Values("x", 1).Exec(context.Background())
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "does not match"), true)
	})
}

func TestDsl_Insert_Mixed(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Insert(testUser).
			ValuesOf(assignRow{setCol("name", "a"), setCol("age", 1)}).
			Columns(testUser.Name, testUser.Age).
			Values("b", 2).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO `user` (`name`, `age`) VALUES (?, ?), (?, ?)")
	})
}

func TestDsl_Insert_MixedColumns(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		mixed := Insert(testUser).ValuesOf(
			assignRow{setCol("name", "a"), setCol("age", 1)},
			assignRow{setCol("name", "b")},
			assignRow{setCol("name", "c"), setCol("age", 3)},
			assignRow{setCol("name", "d")},
		)
		_, _, err := mixed.ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "use Exec"), true)

		fe := &fakeExecutor{}
		mixed.Batch(2)
		mixed.executor = fe
		result, err := mixed.Exec(context.Background())
		t.AssertNil(err)
		t.AssertEQ(fe.queries, []string{
			"INSERT INTO `user` (`name`, `age`) VALUES (?, ?), (?, ?)",
			"INSERT INTO `user` (`name`) VALUES (?), (?)",
		})
		affected, err := result.RowsAffected()
		t.AssertNil(err)
		t.Assert(affected, int64(2))

		uniform := Insert(testUser).
			Columns(testUser.Name).
			Values("a").Values("b").Values("c").Values("d").
			Batch(2)
		fe = &fakeExecutor{}
		uniform.executor = fe
		_, err = uniform.Exec(context.Background())
		t.AssertNil(err)
		t.AssertEQ(fe.queries, []string{
			"INSERT INTO `user` (`name`) VALUES (?), (?)",
			"INSERT INTO `user` (`name`) VALUES (?), (?)",
		})
	})
}

func TestDsl_Insert_From(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := InsertFrom(testUser, Select(testUser.Name).From(testUser).Where(testUser.Age.Gt(18))).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO `user` (`name`) SELECT `user`.`name` FROM `user` WHERE `user`.`age` > ? AND `user`.`deleted_at` IS NULL")
	})
}

func TestDsl_Update_Set(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Update(testUser).
			Set(testUser.Name, "x").
			Set(testUser.Age, 20).
			Where(testUser.Id.Eq(1)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "UPDATE `user` SET `name` = ?, `age` = ? WHERE `user`.`id` = ?")
		t.AssertEQ(args, []any{"x", 20, int64(1)})

		// Set 表达式值：字段算术渲染为 SQL 片段。
		sql, args, err = Update(testUser).
			Set(testUser.Age, testUser.Age.Add(1)).
			Where(testUser.Id.Eq(1)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "UPDATE `user` SET `age` = (`user`.`age` + ?) WHERE `user`.`id` = ?")
		t.AssertEQ(args, []any{1, int64(1)})

		// Set 表达式值：gooq.Raw 渲染为 SQL 片段。
		sql, args, err = Update(testUser).
			Set(testUser.Age, gooq.Raw("age + 1")).
			Where(testUser.Id.Eq(1)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "UPDATE `user` SET `age` = age + 1 WHERE `user`.`id` = ?")
		t.AssertEQ(args, []any{int64(1)})
	})
}

func TestDsl_Update_ValuesOf_Data(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Update(testUser).
			ValuesOf(assignRow{setCol("name", "x"), setCol("age", 0)}).
			Where(testUser.Id.Eq(1)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "UPDATE `user` SET `name` = ?, `age` = ? WHERE `user`.`id` = ?")
		t.AssertEQ(args, []any{"x", 0, int64(1)})

		sql, args, err = Update(testUser).
			ValuesOf(assignRow{nullCol("deleted_at")}).
			Where(testUser.Id.Eq(1)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "UPDATE `user` SET `deleted_at` = NULL WHERE `user`.`id` = ?")
		t.AssertEQ(args, []any{int64(1)})

		sql, args, err = Update(testUser).Data(map[string]any{"age": 1, "name": "x"}).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "UPDATE `user` SET `age` = ?, `name` = ?")
		t.AssertEQ(args, []any{1, "x"})

		_, _, err = Update(testUser).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
	})
}

func TestDsl_Update_Join(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		u := testUser.As("u")
		r := testRole.As("r")
		sql, args, err := Update(u).
			Set(u.Status, "vip").
			InnerJoin(r).On(r.Id.EqExpr(u.Id)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "UPDATE `user` AS `u` INNER JOIN `role` AS `r` ON `r`.`id` = `u`.`id` SET `status` = ?")

		sql, args, err = Update(u).
			Set(u.Status, "vip").
			InnerJoin(r).On(r.Id.Eq(7)).
			Where(u.Name.Eq("ann")).
			ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `UPDATE "user" AS "u" SET "status" = $1 FROM "role" AS "r" WHERE "r"."id" = $2 AND "u"."name" = $3`)
		t.AssertEQ(args, []any{"vip", int64(7), "ann"})

		sql, _, err = Update(u).
			Set(u.Status, "vip").
			InnerJoin(r).On(r.Id.EqExpr(u.Id)).
			ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `UPDATE "user" AS "u" SET "status" = $1 FROM "role" AS "r" WHERE "r"."id" = "u"."id"`)

		sql, _, err = Update(u).
			Set(u.Status, "vip").
			LeftJoin(r).On(r.Id.EqExpr(u.Id)).
			ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `UPDATE "user" AS "u" SET "status" = $1 FROM (SELECT 1) AS "_gooq" LEFT JOIN "role" AS "r" ON "r"."id" = "u"."id"`)

		sql, _, err = Update(u).
			Set(u.Status, "vip").
			LeftJoin(r).On(r.Id.EqExpr(u.Id)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "UPDATE `user` AS `u` LEFT JOIN `role` AS `r` ON `r`.`id` = `u`.`id` SET `status` = ?")

		r2 := testRole.As("r2")
		sql, _, err = Update(u).
			Set(u.Status, "vip").
			InnerJoin(r).On(r.Id.EqExpr(u.Id)).
			LeftJoin(r2).On(r2.Id.EqExpr(u.Id)).
			ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `UPDATE "user" AS "u" SET "status" = $1 FROM (SELECT 1) AS "_gooq" INNER JOIN "role" AS "r" ON "r"."id" = "u"."id" LEFT JOIN "role" AS "r2" ON "r2"."id" = "u"."id"`)

		_, _, err = Update(u).
			Set(u.Status, "vip").
			LeftJoin(r).On(r.Id.EqExpr(u.Id)).
			ToSql(gooq.DialectSQLite)
		t.AssertNE(err, nil)
	})
}

func TestDsl_Update_Batch(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		b := Update(testUser).ValuesOf(
			assignRow{setCol("id", int64(1)), setCol("name", "a")},
			assignRow{setCol("id", int64(2)), setCol("name", "b")},
		)
		sqls, argss, err := b.renderBatchDML(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(len(sqls), 2)
		t.Assert(sqls[0], "UPDATE `user` SET `name` = ? WHERE `id` = ?")
		t.AssertEQ(argss[0], []any{"a", int64(1)})
		t.Assert(sqls[1], "UPDATE `user` SET `name` = ? WHERE `id` = ?")
		t.AssertEQ(argss[1], []any{"b", int64(2)})

		_, _, err = b.ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
	})
}

func TestDsl_Delete(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Delete(testUser).Where(testUser.Id.Eq(1)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "UPDATE `user` SET `deleted_at` = ? WHERE `user`.`id` = ?")

		sql, _, err = Delete(testUser).Unscoped().Where(testUser.Id.Eq(1)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "DELETE FROM `user` WHERE `user`.`id` = ?")

		sql, _, err = Delete(testUserRole).Where(testUserRole.Id.Eq(1)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "DELETE FROM `user_role` WHERE `user_role`.`id` = ?")

		sqls, argss, err := Delete(testUser).ValuesOf(
			assignRow{setCol("id", int64(1))},
			assignRow{setCol("id", int64(2))},
		).renderBatchDML(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(len(sqls), 2)
		t.Assert(sqls[0], "UPDATE `user` SET `deleted_at` = ? WHERE `id` = ?")
		t.Assert(len(argss[0]), 2)
		t.AssertEQ(argss[0][1], int64(1))

		sqls, argss, err = Delete(Cte("x")).
			Keys(testUser.Id).
			ValuesOf(assignRow{setCol("id", int64(1))}).
			renderBatchDML(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(len(sqls), 1)
		t.Assert(sqls[0], "DELETE FROM `x` WHERE `id` = ?")
		t.AssertEQ(argss[0], []any{int64(1)})

		u := testUserRole.As("u")
		r := testRole.As("r")
		sql, _, err = Delete(u).Where(u.Id.Eq(1)).ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `DELETE FROM "user_role" AS "u" WHERE "u"."id" = $1`)

		sql, _, err = Delete(u).InnerJoin(r).On(r.Id.EqExpr(u.Id)).ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `DELETE FROM "user_role" AS "u" USING "role" AS "r" WHERE "r"."id" = "u"."id"`)

		sql, _, err = Delete(u).InnerJoin(r).On(r.Id.EqExpr(u.Id)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "DELETE `u` FROM `user_role` AS `u` INNER JOIN `role` AS `r` ON `r`.`id` = `u`.`id`")

		sql, _, err = Delete(testUser.As("u")).LeftJoin(r).On(r.Id.EqExpr(testUser.As("u").Id)).ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(strings.Contains(sql, `FROM (SELECT 1) AS "_gooq"`), true)
		t.Assert(strings.Contains(sql, `LEFT JOIN "role" AS "r"`), true)

		_, _, err = Delete(u).InnerJoin(r).On(r.Id.EqExpr(u.Id)).ToSql(gooq.DialectSQLite)
		t.AssertNE(err, nil)
	})
}

func TestDsl_Upsert(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Insert(testUser).
			Columns(testUser.Name, testUser.Age).
			Values("a", 1).
			OnConflictKey(testUser.Id).
			DoUpdate(testUser.Name, "x").
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO `user` (`name`, `age`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `name` = ?")
		t.AssertEQ(args, []any{"a", 1, "x"})

		sql, _, err = Insert(testUser).
			Columns(testUser.Name, testUser.Age).
			Values("a", 1).
			DoUpdate(testUser.Name, gooq.Excluded(testUser.Name)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO `user` (`name`, `age`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `name` = VALUES(`name`)")

		sql, _, err = Insert(testUser).
			Columns(testUser.Name).
			Values("a").
			DoNothing().
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT IGNORE INTO `user` (`name`) VALUES (?)")

		sql, args, err = Insert(testUser).
			Columns(testUser.Name, testUser.Age).
			Values("a", 1).
			OnConflictKey(testUser.Id).
			DoUpdate(testUser.Name, "x").
			ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `INSERT INTO "user" ("name", "age") VALUES ($1, $2) ON CONFLICT ("id") DO UPDATE SET "name" = $3`)
		t.AssertEQ(args, []any{"a", 1, "x"})

		sql, args, err = Insert(testUser).
			Columns(testUser.Name, testUser.Age).
			Values("a", 1).
			Values("b", 2).
			OnConflictKey(testUser.Id).
			DoUpdate(testUser.Name, gooq.Excluded(testUser.Name)).
			ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `INSERT INTO "user" ("name", "age") VALUES ($1, $2), ($3, $4) ON CONFLICT ("id") DO UPDATE SET "name" = EXCLUDED."name"`)
		t.AssertEQ(args, []any{"a", 1, "b", 2})

		sql, _, err = Insert(testUser).
			Columns(testUser.Name).
			Values("a").
			OnConflictKey(testUser.Id).
			DoNothing().
			ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `INSERT INTO "user" ("name") VALUES ($1) ON CONFLICT ("id") DO NOTHING`)

		sql, _, err = Insert(testUser).
			Columns(testUser.Name).
			Values("a").
			DoNothing().
			ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `INSERT INTO "user" ("name") VALUES ($1) ON CONFLICT DO NOTHING`)

		_, _, err = Insert(testUser).
			Columns(testUser.Name).
			Values("a").
			DoUpdate(testUser.Name, "x").
			ToSql(gooq.DialectPgsql)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "ON CONFLICT DO UPDATE requires a conflict target"), true)

		sql, args, err = Insert(testUser).
			Columns(testUser.Name, testUser.Age).
			Values("a", 1).
			OnConflictKey(testUser.Id).
			DoUpdate(testUser.Name, "x").
			ToSql(gooq.DialectSQLite)
		t.AssertNil(err)
		t.Assert(sql, `INSERT INTO "user" ("name", "age") VALUES (?, ?) ON CONFLICT ("id") DO UPDATE SET "name" = ?`)
		t.AssertEQ(args, []any{"a", 1, "x"})

		sql, args, err = Insert(testUser).
			Columns(testUser.Name, testUser.Age).
			Values("a", 1).
			OnConflictKey(testUser.Id).
			DoUpdate(testUser.Name, "x").
			DoUpdate(testUser.Age, 2).
			ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `INSERT INTO "user" ("name", "age") VALUES ($1, $2) ON CONFLICT ("id") DO UPDATE SET "age" = $3, "name" = $4`)
		t.AssertEQ(args, []any{"a", 1, 2, "x"})
		sqlAgain, _, err := Insert(testUser).
			Columns(testUser.Name, testUser.Age).
			Values("a", 1).
			OnConflictKey(testUser.Id).
			DoUpdate(testUser.Age, 2).
			DoUpdate(testUser.Name, "x").
			ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sqlAgain, sql)

		sql, args, err = Insert(testUser).
			Columns(testUser.Name, testUser.Age).
			Values("a", 1).
			OnConflictKey(testUser.Id).
			DoUpdate(testUser.Name, "x").
			DoUpdate(testUser.Age, 2).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO `user` (`name`, `age`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `age` = ?, `name` = ?")
		t.AssertEQ(args, []any{"a", 1, 2, "x"})

		sql, _, err = Insert(testUser).
			Columns(testUser.Name, testUser.Age).
			Values("a", 1).
			DoUpdate(testUser.Age, 2).
			DoUpdate(testUser.Name, gooq.Excluded(testUser.Name)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO `user` (`name`, `age`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `age` = ?, `name` = VALUES(`name`)")
	})
}

func TestDsl_Returning(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Update(testUser).Set(testUser.Status, "vip").
			Where(testUser.Id.Eq(1)).
			Returning(testUser.Id).
			ToSql(gooq.DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `UPDATE "user" SET "status" = $1 WHERE "user"."id" = $2 RETURNING "user"."id"`)

		sql, _, err = Insert(testUser).Columns(testUser.Name).Values("a").
			Returning(testUser.Id).
			ToSql(gooq.DialectSQLite)
		t.AssertNil(err)
		t.Assert(sql, `INSERT INTO "user" ("name") VALUES (?) RETURNING "user"."id"`)

		_, _, err = Update(testUser).Set(testUser.Status, "vip").Returning(testUser.Id).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
	})
}
