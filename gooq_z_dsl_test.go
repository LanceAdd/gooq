// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件为 V3 类型化 DSL 的单元测试：渲染断言覆盖条件、组合、Join、子查询与操作符。
package gooq

import (
	"testing"

	"github.com/gogf/gf/v2/test/gtest"
)

// mustToSql 渲染 SQL 并返回 (sql, args)。
func mustToSql(t *gtest.T, b *SelectBuilder) (string, []any) {
	sql, args, err := b.ToSql(DialectMySQL)
	t.AssertNil(err)
	return sql, args
}

// TestDsl_SelectBasic 验证基础 SELECT 渲染。
func TestDsl_SelectBasic(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args := mustToSql(t, SelectFrom(User))
		t.Assert(sql, "SELECT * FROM user WHERE deleted_at IS NULL")

		sql, args = mustToSql(t, Select(User.ID, User.Name).From(User))
		t.Assert(sql, "SELECT id, name FROM user WHERE deleted_at IS NULL")
		t.Assert(len(args), 0)
	})
}

// TestDsl_SelectAlias 验证表别名与字段别名渲染。
func TestDsl_SelectAlias(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		u := User.As("u")
		sql, _ := mustToSql(t, Select(u.ID, u.Name).From(u))
		t.Assert(sql, "SELECT u.id, u.name FROM user AS u WHERE u.deleted_at IS NULL")

		sql, _ = mustToSql(t, Select(User.ID.As("user_id")).From(User))
		t.Assert(sql, "SELECT id AS user_id FROM user WHERE deleted_at IS NULL")
	})
}

// TestDsl_Where 验证条件渲染与多条件默认 AND。
func TestDsl_Where(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args := mustToSql(t, SelectFrom(User).Where(User.Age.Gt(18)))
		t.Assert(sql, "SELECT * FROM user WHERE age > ? AND deleted_at IS NULL")
		t.Assert(args, []any{18})

		// 多条件默认 AND。
		sql, args = mustToSql(t, SelectFrom(User).Where(
			User.Age.Gt(18),
			User.Status.Eq("active"),
		))
		t.Assert(sql, "SELECT * FROM user WHERE age > ? AND status = ? AND deleted_at IS NULL")
		t.Assert(args, []any{18, "active"})
	})
}

// TestDsl_ConditionGroup 验证 AND/OR/NOT 组合渲染。
func TestDsl_ConditionGroup(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _ := mustToSql(t, SelectFrom(User).Where(
			AND(User.Age.Gt(18), User.Status.Eq("active")),
			OR(User.Status.Eq("vip"), User.Status.Eq("guest")),
		))
		t.Assert(
			sql,
			"SELECT * FROM user WHERE (age > ? AND status = ?) AND (status = ? OR status = ?) AND deleted_at IS NULL",
		)

		sql, _ = mustToSql(t, SelectFrom(User).Where(NOT(User.Status.Eq("deleted"))))
		t.Assert(sql, "SELECT * FROM user WHERE (NOT status = ?) AND deleted_at IS NULL")
	})
}

// TestDsl_FieldOps 验证字段操作符渲染。
func TestDsl_FieldOps(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		u := User.As("u")
		cases := map[string]Expression{
			"u.id = ?":          u.ID.Eq(1001),
			"u.id != ?":         u.ID.Ne(1001),
			"u.age > ?":         u.Age.Gt(18),
			"u.age >= ?":        u.Age.Gte(18),
			"u.age < ?":         u.Age.Lt(30),
			"u.age <= ?":        u.Age.Lte(30),
			"u.name LIKE ?":     u.Name.Like("j%"),
			"u.name NOT LIKE ?": u.Name.NotLike("j%"),
			"u.id IS NULL":      u.ID.IsNull(),
			"u.id IS NOT NULL":  u.ID.IsNotNull(),
		}
		for expected, expr := range cases {
			sql, _ := mustToSql(t, SelectFrom(u).Where(expr))
			t.Assert(sql, "SELECT * FROM user AS u WHERE "+expected+" AND u.deleted_at IS NULL")
		}

		// IN / BETWEEN。
		sql, args := mustToSql(t, SelectFrom(u).Where(u.ID.In(1, 2, 3)))
		t.Assert(sql, "SELECT * FROM user AS u WHERE u.id IN (?, ?, ?) AND u.deleted_at IS NULL")
		t.Assert(args, []any{1, 2, 3})

		sql, args = mustToSql(t, SelectFrom(u).Where(u.Age.Between(18, 30)))
		t.Assert(sql, "SELECT * FROM user AS u WHERE u.age BETWEEN ? AND ? AND u.deleted_at IS NULL")
		t.Assert(args, []any{18, 30})
	})
}

// TestDsl_Join 验证 Join 渲染。
func TestDsl_Join(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		u := User.As("u")
		o := Order.As("o")
		sql, _ := mustToSql(t, Select(u.Name, o.Amount).From(u).
			LeftJoin(o).On(u.ID.Eq(o.UserID)).
			Where(u.Status.Eq("active")))
		t.Assert(
			sql,
			"SELECT u.name, o.amount FROM user AS u LEFT JOIN order AS o ON u.id = o.user_id WHERE u.status = ? AND u.deleted_at IS NULL",
		)
	})
}

// TestDsl_Subquery 验证子查询渲染（IN/标量/派生表）。
func TestDsl_Subquery(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		u := User.As("u")
		o := Order.As("o")

		// IN 子查询。
		sql, _ := mustToSql(t, SelectFrom(u).Where(u.ID.In(
			Select(o.UserID).From(o).Where(o.Amount.Gt(1000)),
		)))
		t.Assert(
			sql,
			"SELECT * FROM user AS u WHERE u.id IN (SELECT o.user_id FROM order AS o WHERE o.amount > ? AND o.deleted_at IS NULL) AND u.deleted_at IS NULL",
		)

		// 标量子查询（SELECT 位置）。
		sql, _ = mustToSql(t, Select(
			u.Name,
			Select(CountFunc(o.ID)).From(o).Where(o.UserID.Eq(u.ID)).As("order_cnt"),
		).From(u).Group(u.ID))
		t.Assert(
			sql,
			"SELECT u.name, (SELECT COUNT(o.id) FROM order AS o WHERE o.user_id = u.id AND o.deleted_at IS NULL) AS order_cnt FROM user AS u WHERE u.deleted_at IS NULL GROUP BY u.id",
		)

		// 派生表。
		sql, _ = mustToSql(t, Select().From(SelectFrom(u).As("t")))
		t.Assert(sql, "SELECT * FROM (SELECT * FROM user AS u WHERE u.deleted_at IS NULL) AS t")
	})
}

// TestDsl_OrderGroupHaving 验证排序、分组与过滤。
func TestDsl_OrderGroupHaving(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _ := mustToSql(t, SelectFrom(User).
			Order(User.ID.Desc()).
			Limit(10))
		t.Assert(sql, "SELECT * FROM user WHERE deleted_at IS NULL ORDER BY id DESC LIMIT 10")

		sql, _ = mustToSql(t, Select(User.Status, CountFunc(User.ID)).From(User).
			Group(User.Status).
			Having(Raw("COUNT(id) > ?", 100)))
		t.Assert(
			sql,
			"SELECT status, COUNT(id) FROM user WHERE deleted_at IS NULL GROUP BY status HAVING COUNT(id) > ?",
		)
	})
}

// TestDsl_OperatorFunc 验证操作符渲染与别名函数。
func TestDsl_OperatorFunc(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args := mustToSql(t, Select(
			DateFormatFunc(User.CreatedAt, "%Y-%m").As("month"),
			CountFunc(User.ID).As("cnt"),
		).From(User))
		t.Assert(sql, "SELECT DATE_FORMAT(created_at, ?) AS month, COUNT(id) AS cnt FROM user WHERE deleted_at IS NULL")
		t.Assert(args, []any{"%Y-%m"})
	})
}

// TestDsl_StructRaw 验证结构化 Raw 渲染。
func TestDsl_StructRaw(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args := mustToSql(t, SelectFrom(User).Where(
			Raw("JSON_EXTRACT(meta, ?) = ?", "$.level", "vip"),
		))
		t.Assert(sql, "SELECT * FROM user WHERE JSON_EXTRACT(meta, ?) = ? AND deleted_at IS NULL")
		t.Assert(args, []any{"$.level", "vip"})
	})
}

// TestDsl_FieldsEx 验证字段排除差集。
func TestDsl_FieldsEx(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _ := mustToSql(t, Select().From(User).FieldsEx(User.DeletedAt))
		t.Assert(sql, "SELECT id, name, age, status, created_at FROM user WHERE deleted_at IS NULL")
	})
}

// TestDsl_SoftDelete 验证软删除自动条件、显式接管与 Unscoped。
func TestDsl_SoftDelete(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		// 自动条件。
		sql, _ := mustToSql(t, SelectFrom(User).Where(User.Age.Gt(18)))
		t.Assert(sql, "SELECT * FROM user WHERE age > ? AND deleted_at IS NULL")

		// 显式接管。
		sql, _ = mustToSql(t, SelectFrom(User).Where(User.DeletedAt.Eq(nil)))
		t.Assert(sql, "SELECT * FROM user WHERE deleted_at = ?")

		// Unscoped 绕过。
		sql, _ = mustToSql(t, SelectFrom(User).Unscoped().Where(User.Age.Gt(18)))
		t.Assert(sql, "SELECT * FROM user WHERE age > ?")
	})
}

// TestDsl_PgsqlDialect 验证 PG 方言（占位符 $n）。
func TestDsl_PgsqlDialect(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := SelectFrom(User).Where(User.Age.Gt(18), User.Status.Eq("active")).ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT * FROM user WHERE age > $1 AND status = $2 AND deleted_at IS NULL`)
		t.Assert(args, []any{18, "active"})
	})
}

// TestDsl_Exists 验证 EXISTS/NOT EXISTS 子查询条件。
func TestDsl_Exists(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		u := User.As("u")
		o := Order.As("o")

		sql, _ := mustToSql(t, SelectFrom(u).Where(Exists(
			Select(o.ID).From(o).Where(o.UserID.Eq(u.ID)),
		)))
		t.Assert(
			sql,
			"SELECT * FROM user AS u WHERE EXISTS (SELECT o.id FROM order AS o WHERE o.user_id = u.id AND o.deleted_at IS NULL) AND u.deleted_at IS NULL",
		)

		sql, _ = mustToSql(t, SelectFrom(u).Where(NotExists(
			Select(o.ID).From(o).Where(o.UserID.Eq(u.ID)),
		)))
		t.Assert(
			sql,
			"SELECT * FROM user AS u WHERE NOT EXISTS (SELECT o.id FROM order AS o WHERE o.user_id = u.id AND o.deleted_at IS NULL) AND u.deleted_at IS NULL",
		)
	})
}

// TestDsl_FullJoin 验证 FULL JOIN 渲染。
func TestDsl_FullJoin(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		u := User.As("u")
		o := Order.As("o")
		sql, _ := mustToSql(t, Select(u.ID, o.Amount).From(u).
			FullJoin(o).On(u.ID.Eq(o.UserID)))
		t.Assert(
			sql,
			"SELECT u.id, o.amount FROM user AS u FULL JOIN order AS o ON u.id = o.user_id WHERE u.deleted_at IS NULL",
		)
	})
}

// TestDsl_Lock 验证行锁渲染（MySQL/PG 差异）。
func TestDsl_Lock(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, _ := mustToSql(t, SelectFrom(User).Where(User.ID.Eq(1)).LockForUpdate())
		t.Assert(sql, "SELECT * FROM user WHERE id = ? AND deleted_at IS NULL FOR UPDATE")

		sql, _ = mustToSql(t, SelectFrom(User).Where(User.ID.Eq(1)).LockInShareMode())
		t.Assert(sql, "SELECT * FROM user WHERE id = ? AND deleted_at IS NULL LOCK IN SHARE MODE")

		// PG：FOR SHARE。
		sql, _, err := SelectFrom(User).Where(User.ID.Eq(1)).LockInShareMode().ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, "SELECT * FROM user WHERE id = $1 AND deleted_at IS NULL FOR SHARE")

		// SQLite：忽略锁。
		sql, _, err = SelectFrom(User).Where(User.ID.Eq(1)).LockForUpdate().ToSql(DialectSQLite)
		t.AssertNil(err)
		t.Assert(sql, "SELECT * FROM user WHERE id = ? AND deleted_at IS NULL")
	})
}

// TestDsl_CaseWhen 验证 CASE WHEN 表达式。
func TestDsl_CaseWhen(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args := mustToSql(t, Select(
			Case().
				When(User.Age.Gt(60)).Then("elder").
				When(User.Age.Gt(18)).Then("adult").
				Else("minor").
				End().As("age_group"),
		).From(User))
		t.Assert(
			sql,
			"SELECT CASE WHEN age > ? THEN ? WHEN age > ? THEN ? ELSE ? END AS age_group FROM user WHERE deleted_at IS NULL",
		)
		t.Assert(args, []any{60, "elder", 18, "adult", "minor"})
	})
}

// TestDsl_ArithExpr 验证算术表达式与字段运算。
func TestDsl_ArithExpr(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		// 字段方法。
		sql, args := mustToSql(t, Select(User.Age.Add(1)).From(User).Where(User.Age.Gt(18)))
		t.Assert(sql, "SELECT (age + ?) FROM user WHERE age > ? AND deleted_at IS NULL")
		t.Assert(args, []any{1, 18})

		// 嵌套 + 包级函数。
		sql, args = mustToSql(t, Select(Mul(User.Age, 2)).From(User).Where(User.Age.Gt(18)))
		t.Assert(sql, "SELECT (age * ?) FROM user WHERE age > ? AND deleted_at IS NULL")
		t.Assert(args, []any{2, 18})

		// 条件位置 + 取负（包级比较函数作用于任意表达式）。
		sql, args = mustToSql(t, SelectFrom(User).Where(Gt(Negate(User.Age), -10)))
		t.Assert(sql, "SELECT * FROM user WHERE (-age) > ? AND deleted_at IS NULL")
		t.Assert(args, []any{-10})
	})
}

// TestDml_BatchInsert 验证批量 INSERT VALUES。
func TestDml_BatchInsert(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Insert(User, []map[string]any{
			{"name": "a", "age": 18, "status": "active"},
			{"name": "b", "age": 20, "status": "active"},
		}).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO user (age, name, status) VALUES (?, ?, ?), (?, ?, ?)")
		t.Assert(args, []any{18, "a", "active", 20, "b", "active"})
	})
}

// TestDml_InsertFrom 验证 INSERT ... SELECT。
func TestDml_InsertFrom(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		u := User.As("u")
		sql, args, err := InsertFrom(User, Select(u.Name, u.Age).From(u).Where(u.Age.Gt(18))).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(
			sql,
			"INSERT INTO user (name, age) SELECT u.name, u.age FROM user AS u WHERE u.age > ? AND u.deleted_at IS NULL",
		)
		t.Assert(args, []any{18})
	})
}

// TestDsl_ConditionExternal 验证条件一等对象（外部构建/复用）。
func TestDsl_ConditionExternal(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		cond := AND(User.Age.Gt(18), User.Status.Eq("active"))
		cond = OR(cond, User.Name.Like("j%"))
		sql, _ := mustToSql(t, SelectFrom(User).Where(cond))
		t.Assert(
			sql,
			"SELECT * FROM user WHERE ((age > ? AND status = ?) OR name LIKE ?) AND deleted_at IS NULL",
		)
	})
}
