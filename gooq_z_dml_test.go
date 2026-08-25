// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件为 DML 渲染单元测试：Insert/Update/Delete 各入口、批量、Upsert、Returning、软删。
package gooq

import (
	"testing"
	"time"

	"github.com/gogf/gf/v2/test/gtest"
)

type testUserRecord struct {
	Id        int64     `json:"id" orm:"id"`
	Name      string    `json:"name" orm:"name"`
	Age       int       `json:"age" orm:"age"`
	Status    string    `json:"status" orm:"status"`
	CreatedAt time.Time `json:"createdAt" orm:"created_at"`
	DeletedAt time.Time `json:"deletedAt" orm:"deleted_at"`
}

func TestDsl_Insert_Record(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Insert(testUser).Record(testUserRecord{Name: "john", Age: 18}).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO `user` (`name`, `age`) VALUES (?, ?)")
		t.AssertEQ(args, []any{"john", 18})

		sql, _, err = Insert(testUser).Record(testUserRecord{Name: "a", Status: "vip"}).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO `user` (`name`, `status`) VALUES (?, ?)")

		sql, _, err = Insert(testUser).Record(testUserRecord{}).ToSql(DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(sql, "")
	})
}

func TestDsl_Insert_ColumnsValues(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Insert(testUser).
			Columns(testUser.Name, testUser.Age).
			Values("a", 1).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO `user` (`name`, `age`) VALUES (?, ?)")
		t.AssertEQ(args, []any{"a", 1})

		sql, args, err = Insert(testUser).
			Columns(testUser.Name, testUser.Age).
			Values("a", 1).
			Values("b", 2).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO `user` (`name`, `age`) VALUES (?, ?), (?, ?)")
		t.AssertEQ(args, []any{"a", 1, "b", 2})

		sql, _, err = Insert(testUser).Values("a").ToSql(DialectMySQL)
		t.AssertNE(err, nil)

		sql, _, err = Insert(testUser).ToSql(DialectMySQL)
		t.AssertNE(err, nil)
	})
}

func TestDsl_Insert_Mixed(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Insert(testUser).
			Record(testUserRecord{Name: "a", Age: 1}).
			Columns(testUser.Name, testUser.Age).
			Values("b", 2).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO `user` (`name`, `age`) VALUES (?, ?), (?, ?)")
	})
}

func TestDsl_Insert_From(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := InsertFrom(testUser, Select(testUser.Name).From(testUser).Where(testUser.Age.Gt(18))).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO `user` (`name`) SELECT `user`.`name` FROM `user` WHERE `user`.`age` > ? AND `user`.`deleted_at` IS NULL")
	})
}

func TestDsl_Update_Set(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Update(testUser).
			Set(testUser.Name, "x").
			Set(testUser.Age, 20).
			Where(testUser.ID.Eq(1)).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "UPDATE `user` SET `name` = ?, `age` = ? WHERE `user`.`id` = ?")
		t.AssertEQ(args, []any{"x", 20, int64(1)})
	})
}

func TestDsl_Update_Record_Data(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Update(testUser).Record(testUserRecord{Name: "x", Age: 20}).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "UPDATE `user` SET `name` = ?, `age` = ?")
		t.AssertEQ(args, []any{"x", 20})

		sql, args, err = Update(testUser).Data(map[string]any{"age": 1, "name": "x"}).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "UPDATE `user` SET `age` = ?, `name` = ?")
		t.AssertEQ(args, []any{1, "x"})

		_, _, err = Update(testUser).ToSql(DialectMySQL)
		t.AssertNE(err, nil)
	})
}

func TestDsl_Update_Join(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		u := testUser.As("u")
		r := testRole.As("r")
		sql, _, err := Update(u).
			Set(u.Status, "vip").
			InnerJoin(r).On(r.ID.EqExpr(u.ID)).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "UPDATE `user` AS u INNER JOIN `role` AS r ON `r`.`id` = `u`.`id` SET `status` = ?")

		sql, _, err = Update(u).
			Set(u.Status, "vip").
			InnerJoin(r).On(r.ID.EqExpr(u.ID)).
			ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `UPDATE "user" AS u SET "status" = $1 FROM "role" AS r WHERE "r"."id" = "u"."id"`)
	})
}

func TestDsl_Update_Batch(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		b := Update(testUser).Records([]testUserRecord{
			{Id: 1, Name: "a"},
			{Id: 2, Name: "b"},
		})
		sqls, argss, err := b.renderBatchDML(DialectMySQL)
		t.AssertNil(err)
		t.Assert(len(sqls), 2)
		t.Assert(sqls[0], "UPDATE `user` SET `name` = ? WHERE `id` = ?")
		t.AssertEQ(argss[0], []any{"a", int64(1)})
		t.Assert(sqls[1], "UPDATE `user` SET `name` = ? WHERE `id` = ?")
		t.AssertEQ(argss[1], []any{"b", int64(2)})

		_, _, err = b.ToSql(DialectMySQL)
		t.AssertNE(err, nil)
	})
}

func TestDsl_Delete(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Delete(testUser).Where(testUser.ID.Eq(1)).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "UPDATE `user` SET `deleted_at` = ? WHERE `user`.`id` = ?")

		sql, _, err = Delete(testUser).Unscoped().Where(testUser.ID.Eq(1)).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "DELETE FROM `user` WHERE `user`.`id` = ?")

		sql, _, err = Delete(testUserRole).Where(testUserRole.ID.Eq(1)).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "DELETE FROM `user_role` WHERE `user_role`.`id` = ?")

		sqls, argss, err := Delete(testUser).Records([]testUserRecord{{Id: 1}, {Id: 2}}).renderBatchDML(DialectMySQL)
		t.AssertNil(err)
		t.Assert(len(sqls), 2)
		t.Assert(sqls[0], "UPDATE `user` SET `deleted_at` = ? WHERE `id` = ?")
		t.Assert(len(argss[0]), 2)
		t.AssertEQ(argss[0][1], int64(1))
	})
}

func TestDsl_Upsert(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Insert(testUser).
			Columns(testUser.Name, testUser.Age).
			Values("a", 1).
			OnConflictKey(testUser.ID).
			DoUpdate(testUser.Name, "x").
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO `user` (`name`, `age`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `name` = VALUES(`name`)")

		sql, _, err = Insert(testUser).
			Columns(testUser.Name).
			Values("a").
			DoNothing().
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT IGNORE INTO `user` (`name`) VALUES (?)")

		sql, args, err := Insert(testUser).
			Columns(testUser.Name, testUser.Age).
			Values("a", 1).
			OnConflictKey(testUser.ID).
			DoUpdate(testUser.Name, "x").
			ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `INSERT INTO "user" ("name", "age") VALUES ($1, $2) ON CONFLICT ("id") DO UPDATE SET "name" = $3`)
		t.AssertEQ(args, []any{"a", 1, "x"})

		sql, _, err = Insert(testUser).
			Columns(testUser.Name).
			Values("a").
			OnConflictKey(testUser.ID).
			DoNothing().
			ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `INSERT INTO "user" ("name") VALUES ($1) ON CONFLICT ("id") DO NOTHING`)
	})
}

func TestDsl_Returning(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _, err := Update(testUser).Set(testUser.Status, "vip").
			Where(testUser.ID.Eq(1)).
			Returning(testUser.ID).
			ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `UPDATE "user" SET "status" = $1 WHERE "user"."id" = $2 RETURNING "user"."id"`)

		sql, _, err = Insert(testUser).Columns(testUser.Name).Values("a").
			Returning(testUser.ID).
			ToSql(DialectSQLite)
		t.AssertNil(err)
		t.Assert(sql, `INSERT INTO "user" ("name") VALUES (?) RETURNING "user"."id"`)

		_, _, err = Update(testUser).Set(testUser.Status, "vip").Returning(testUser.ID).ToSql(DialectMySQL)
		t.AssertNE(err, nil)
	})
}
