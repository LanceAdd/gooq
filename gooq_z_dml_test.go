// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件为写操作 DSL（Insert/Update/Delete/Upsert）与操作符方言覆盖的单元测试。
package gooq

import (
	"context"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/test/gtest"
)

// TestDml_Insert 验证 INSERT 渲染（跳过自增主键）。
func TestDml_Insert(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Insert(User, map[string]any{
			"name":   "john",
			"age":    18,
			"status": "active",
		}).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO user (age, name, status) VALUES (?, ?, ?)")
		t.Assert(args, []any{18, "john", "active"})
	})
}

// TestDml_Update 验证 UPDATE 渲染。
func TestDml_Update(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		sql, args, err := Update(User).
			Set(User.Name, "john2").
			Where(User.ID.Eq(1001)).
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "UPDATE user SET name = ? WHERE id = ?")
		t.Assert(args, []any{"john2", 1001})
	})
}

// TestDml_Delete 验证软删转 UPDATE 与 Unscoped 真 DELETE。
func TestDml_Delete(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		// 软删表：转 UPDATE deleted_at。
		sql, args, err := Delete(User).Where(User.ID.Eq(1001)).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "UPDATE user SET deleted_at = ? WHERE id = ?")
		t.Assert(len(args), 2)

		// Unscoped：真 DELETE。
		sql, args, err = Delete(User).Unscoped().Where(User.ID.Eq(1001)).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "DELETE FROM user WHERE id = ?")
		t.Assert(args, []any{1001})
	})
}

// TestDml_Upsert 验证 Upsert 按方言渲染。
func TestDml_Upsert(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		// MySQL：ON DUPLICATE KEY UPDATE。
		sql, _, err := Insert(User, map[string]any{"id": 1, "name": "john"}).
			OnConflictKey(User.ID).
			DoUpdate(User.Name, "john2").
			ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(
			sql,
			"INSERT INTO user (name) VALUES (?) ON DUPLICATE KEY UPDATE name = VALUES(name)",
		)

		// PG：ON CONFLICT DO UPDATE。
		sql, _, err = Insert(User, map[string]any{"id": 1, "name": "john"}).
			OnConflictKey(User.ID).
			DoUpdate(User.Name, "john2").
			ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(
			sql,
			"INSERT INTO user (name) VALUES ($1) ON CONFLICT (id) DO UPDATE SET name = $2",
		)
	})
}

// TestOperatorFunc_Dialect 验证操作符按方言覆盖（DATE_FORMAT → TO_CHAR）。
func TestOperatorFunc_Dialect(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		// pgsql 驱动注册覆盖实现。
		OperatorFunc("DATE_FORMAT", func(ctx context.Context, db gdb.DB, args ...any) (string, []any, error) {
			return "TO_CHAR(" + args[0].(string) + ", " + args[1].(string) + ")", nil, nil
		}, "pgsql")

		// MySQL 方言：默认实现。
		sql, args, err := Select(DateFormatFunc(User.CreatedAt, "%Y-%m").As("month")).From(User).ToSql(DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT DATE_FORMAT(created_at, ?) AS month FROM user WHERE deleted_at IS NULL")
		t.Assert(args, []any{"%Y-%m"})

		// PG 方言：驱动覆盖实现（离线渲染按方言推断驱动名）。
		sql, _, err = Select(DateFormatFunc(User.CreatedAt, "YYYY-MM").As("month")).From(User).ToSql(DialectPgsql)
		t.AssertNil(err)
		t.Assert(sql, `SELECT TO_CHAR(created_at, $1) AS month FROM user WHERE deleted_at IS NULL`)
	})
}
