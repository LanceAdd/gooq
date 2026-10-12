package dsl

import (
	"strings"
	"testing"

	"github.com/gogf/gf/v2/test/gtest"

	"github.com/lanceadd/gooq"
)

func TestDsl_MariaDBAndTiDBMatchMySQL(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		for _, dialect := range []gooq.Dialect{gooq.DialectMariaDB, gooq.DialectTiDB} {
			sql, args, err := Insert(testUser).
				Columns(testUser.Name, testUser.Age).
				Values("a", 1).
				DoUpdate(testUser.Name, gooq.Excluded(testUser.Name)).
				ToSql(dialect)
			t.AssertNil(err)
			t.Assert(sql, "INSERT INTO `user` (`name`, `age`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `name` = VALUES(`name`)")
			t.AssertEQ(args, []any{"a", 1})

			sql, _, err = Select(testUser.Id).From(testUser).LockInShareMode().ToSql(dialect)
			t.AssertNil(err)
			t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`deleted_at` IS NULL LOCK IN SHARE MODE")

			_, _, err = Update(testUser).Set(testUser.Status, "vip").Returning(testUser.Id).ToSql(dialect)
			t.AssertNE(err, nil)
			t.Assert(strings.Contains(err.Error(), "RETURNING is not supported by dialect "+string(dialect)), true)

			_, _, err = Select(testUser.Status).From(testUser).GroupCube(testUser.Status).ToSql(dialect)
			t.AssertNE(err, nil)
			t.Assert(strings.Contains(err.Error(), "not supported by dialect "+string(dialect)), true)
		}
	})
}

func TestDsl_GaussDB(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Insert(testUser).
			Columns(testUser.Name, testUser.Age).
			Values("a", 1).
			DoUpdate(testUser.Name, "x").
			ToSql(gooq.DialectGaussDB)
		t.AssertNil(err)
		t.Assert(sql, `INSERT INTO "user" ("name", "age") VALUES ($1, $2) ON DUPLICATE KEY UPDATE "name" = $3`)
		t.AssertEQ(args, []any{"a", 1, "x"})

		sql, args, err = Insert(testUser).
			Columns(testUser.Name, testUser.Age).
			Values("a", 1).
			OnConflictKey(testUser.Id).
			DoUpdate(testUser.Name, gooq.Excluded(testUser.Name)).
			ToSql(gooq.DialectGaussDB)
		t.AssertNil(err)
		t.Assert(sql, `INSERT INTO "user" ("name", "age") VALUES ($1, $2) ON DUPLICATE KEY UPDATE "name" = VALUES("name")`)
		t.AssertEQ(args, []any{"a", 1})

		_, _, err = Insert(testUser).
			Columns(testUser.Name).
			Values("a").
			DoNothing().
			ToSql(gooq.DialectGaussDB)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "INSERT IGNORE is not supported by dialect gaussdb"), true)

		sql, _, err = Update(testUser).Set(testUser.Status, "vip").
			Where(testUser.Id.Eq(1)).
			Returning(testUser.Id).
			ToSql(gooq.DialectGaussDB)
		t.AssertNil(err)
		t.Assert(sql, `UPDATE "user" SET "status" = $1 WHERE "user"."id" = $2 RETURNING "user"."id"`)

		sql, _, err = Select(testUser.Id).From(testUser).LockInShareMode().Offset(20).ToSql(gooq.DialectGaussDB)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "user"."id" FROM "user" WHERE "user"."deleted_at" IS NULL OFFSET 20 FOR SHARE`)

		sql, _, err = Select(testUser.Status).From(testUser).GroupRollup(testUser.Status).ToSql(gooq.DialectGaussDB)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "user"."status" FROM "user" WHERE "user"."deleted_at" IS NULL GROUP BY ROLLUP("user"."status")`)
	})
}
