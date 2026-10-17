package dsl

import (
	"strings"
	"testing"

	"github.com/gogf/gf/v2/test/gtest"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/fn"
)

func TestDsl_MariaDBAndTiDBMatchMySQL(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		for _, dialect := range []gooq.Dialect{gooq.DialectMariaDB, gooq.DialectTiDB, gooq.DialectOceanBase} {
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
		t.Assert(sql, `UPDATE "user" SET "status" = $1 WHERE ("user"."id" = $2) RETURNING "user"."id"`)

		sql, _, err = Select(testUser.Id).From(testUser).LockInShareMode().Offset(20).ToSql(gooq.DialectGaussDB)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "user"."id" FROM "user" WHERE "user"."deleted_at" IS NULL OFFSET 20 FOR SHARE`)

		sql, _, err = Select(testUser.Status).From(testUser).GroupRollup(testUser.Status).ToSql(gooq.DialectGaussDB)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "user"."status" FROM "user" WHERE "user"."deleted_at" IS NULL GROUP BY ROLLUP("user"."status")`)
	})
}

func TestDsl_DM(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Insert(testUser).
			Columns(testUser.Name, testUser.Age).
			Values("a", 1).
			ToSql(gooq.DialectDM)
		t.AssertNil(err)
		t.Assert(sql, `INSERT INTO "USER" ("NAME", "AGE") VALUES (?, ?)`)
		t.AssertEQ(args, []any{"a", 1})

		sql, args, err = Insert(testUser).
			Columns(testUser.Id, testUser.Name, testUser.Age).
			Values(1, "a", 1).
			OnConflictKey(testUser.Id).
			DoUpdate(testUser.Name, gooq.Excluded(testUser.Name)).
			ToSql(gooq.DialectDM)
		t.AssertNil(err)
		t.Assert(sql, `MERGE INTO "USER" T1 USING (SELECT ? AS "ID", ? AS "NAME", ? AS "AGE" FROM DUAL) T2 ON (T1."ID" = T2."ID") WHEN NOT MATCHED THEN INSERT("ID", "NAME", "AGE") VALUES (T2."ID", T2."NAME", T2."AGE") WHEN MATCHED THEN UPDATE SET T1."NAME" = T2."NAME"`)
		t.AssertEQ(args, []any{1, "a", 1})

		sql, args, err = Insert(testUser).
			Columns(testUser.Id, testUser.Name, testUser.Age).
			Values(1, "a", 1).
			Values(2, "b", 2).
			OnConflictKey(testUser.Id).
			DoUpdate(testUser.Name, "x").
			ToSql(gooq.DialectDM)
		t.AssertNil(err)
		t.Assert(sql, `MERGE INTO "USER" T1 USING (SELECT ? AS "ID", ? AS "NAME", ? AS "AGE" FROM DUAL UNION ALL SELECT ?, ?, ? FROM DUAL) T2 ON (T1."ID" = T2."ID") WHEN NOT MATCHED THEN INSERT("ID", "NAME", "AGE") VALUES (T2."ID", T2."NAME", T2."AGE") WHEN MATCHED THEN UPDATE SET T1."NAME" = ?`)
		t.AssertEQ(args, []any{1, "a", 1, 2, "b", 2, "x"})

		sql, _, err = Insert(testUser).
			Columns(testUser.Id, testUser.Name).
			Values(1, "a").
			OnConflictKey(testUser.Id).
			DoNothing().
			ToSql(gooq.DialectDM)
		t.AssertNil(err)
		t.Assert(sql, `MERGE INTO "USER" T1 USING (SELECT ? AS "ID", ? AS "NAME" FROM DUAL) T2 ON (T1."ID" = T2."ID") WHEN NOT MATCHED THEN INSERT("ID", "NAME") VALUES (T2."ID", T2."NAME")`)

		_, _, err = Insert(testUser).Columns(testUser.Name).Values("a").DoNothing().ToSql(gooq.DialectDM)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "conflict target"), true)

		_, _, err = Update(testUser).Set(testUser.Status, "vip").Returning(testUser.Id).ToSql(gooq.DialectDM)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "RETURNING is not supported by dialect dm"), true)

		_, _, err = Delete(testUser).Unscoped().InnerJoin(testRole).On(testRole.Id.EqExpr(testUser.Id)).ToSql(gooq.DialectDM)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "DELETE with JOIN is not supported by dialect dm"), true)

		_, _, err = Update(testUser).Set(testUser.Status, "vip").LeftJoin(testRole).On(testRole.Id.EqExpr(testUser.Id)).ToSql(gooq.DialectDM)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "outer join is not supported by dialect dm"), true)

		sql, args, err = Update(testUser).Set(testUser.Status, "vip").InnerJoin(testRole).On(testRole.Id.EqExpr(testUser.Id)).ToSql(gooq.DialectDM)
		t.AssertNil(err)
		t.Assert(sql, `UPDATE "USER" SET "STATUS" = ? FROM "ROLE" WHERE ("ROLE"."ID" = "USER"."ID")`)
		t.AssertEQ(args, []any{"vip"})

		sql, _, err = Select(testUser.Id).From(testUser).Offset(20).ToSql(gooq.DialectDM)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "USER"."ID" FROM "USER" WHERE "USER"."DELETED_AT" IS NULL OFFSET 20 ROWS`)

		sql, _, err = Select(testUser.Id).From(testUser).LockForUpdate().ToSql(gooq.DialectDM)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "USER"."ID" FROM "USER" WHERE "USER"."DELETED_AT" IS NULL FOR UPDATE`)

		sql, _, err = Select(testUser.Status).From(testUser).GroupRollup(testUser.Status).ToSql(gooq.DialectDM)
		t.AssertNil(err)
		t.Assert(sql, `SELECT "USER"."STATUS" FROM "USER" WHERE "USER"."DELETED_AT" IS NULL GROUP BY ROLLUP("USER"."STATUS")`)

		sql, _, err = Select(fn.DateFormat(testUser.CreatedAt, "%Y-%m-%d")).From(testUser).ToSql(gooq.DialectDM)
		t.AssertNil(err)
		t.Assert(sql, `SELECT TO_CHAR("USER"."CREATED_AT", 'YYYY-MM-DD') FROM "USER" WHERE "USER"."DELETED_AT" IS NULL`)

		sql, args, err = Select(fn.DateAdd(testUser.CreatedAt, 1, "DAY")).From(testUser).ToSql(gooq.DialectDM)
		t.AssertNil(err)
		t.Assert(sql, `SELECT DATEADD(DAY, ?, "USER"."CREATED_AT") FROM "USER" WHERE "USER"."DELETED_AT" IS NULL`)
		t.AssertEQ(args, []any{1})

		sql, _, err = Select(fn.DateDiff(testUser.CreatedAt, testUser.DeletedAt)).From(testUser).Unscoped().ToSql(gooq.DialectDM)
		t.AssertNil(err)
		t.Assert(sql, `SELECT DATEDIFF(DAY, "USER"."DELETED_AT", "USER"."CREATED_AT") FROM "USER"`)

		sql, _, err = Select(fn.IfNull(testUser.Name, gooq.Str("x"))).From(testUser).ToSql(gooq.DialectDM)
		t.AssertNil(err)
		t.Assert(sql, `SELECT NVL("USER"."NAME", 'x') FROM "USER" WHERE "USER"."DELETED_AT" IS NULL`)

		sql, _, err = Select(fn.GroupConcat(fn.GroupConcatOptions{
			Field:     testUser.Name,
			Separator: "|",
			OrderBy:   []gooq.Expression{testUser.Id.Asc()},
		})).From(testUser).ToSql(gooq.DialectDM)
		t.AssertNil(err)
		t.Assert(sql, `SELECT LISTAGG("USER"."NAME", '|') WITHIN GROUP (ORDER BY "USER"."ID" ASC) FROM "USER" WHERE "USER"."DELETED_AT" IS NULL`)
	})
}
