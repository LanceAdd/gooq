// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件为 gooq 端到端冒烟测试：gooq 生成 SQL → 标准库 database/sql 执行真实 MySQL。
// gooq 本身不依赖数据库，执行由调用方（此处为标准库）负责。
// 运行前提：docker 启动 MySQL（见本目录 README），然后 cd 本目录执行 go test ./...
package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/gogf/gf/v2/test/gtest"
	"github.com/lanceadd/gooq"
)

// UserEntity 是对应用户表的实体。
type UserEntity struct {
	Id        int64      `json:"id"        orm:"id"`
	Name      string     `json:"name"      orm:"name"`
	Age       int        `json:"age"       orm:"age"`
	Status    string     `json:"status"    orm:"status"`
	DeletedAt *time.Time `json:"deletedAt" orm:"deleted_at"`
	CreatedAt *time.Time `json:"createdAt" orm:"created_at"`
}

// openDB 使用标准库连接 MySQL（端口 3307）。
func openDB(t *gtest.T) *sql.DB {
	db, err := sql.Open("mysql", "root:root123@tcp(127.0.0.1:3307)/gooq_test?parseTime=true")
	t.AssertNil(err)
	t.AssertNil(db.Ping())
	return db
}

// createTable 创建测试表（幂等）。
func createTable(t *gtest.T, db *sql.DB) {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS user (
    id         BIGINT PRIMARY KEY AUTO_INCREMENT,
    name       VARCHAR(32)  NOT NULL,
    age        INT          NOT NULL,
    status     VARCHAR(16)  NOT NULL DEFAULT 'active',
    deleted_at DATETIME,
    created_at DATETIME
)`)
	t.AssertNil(err)
	_, err = db.Exec(`TRUNCATE TABLE user`)
	t.AssertNil(err)
}

// queryRows 用标准库执行查询并返回行。
func queryRows(t *gtest.T, db *sql.DB, sqlStr string, args []any) *sql.Rows {
	rows, err := db.Query(sqlStr, args...)
	t.AssertNil(err)
	return rows
}

// TestE2E_CRUD 验证 gooq 生成 SQL → 标准库执行：Insert/Select/Update/Delete（软删）。
func TestE2E_CRUD(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		db := openDB(t)
		createTable(t, db)

		// Insert：gooq 渲染 → 标准库执行。
		sqlStr, args, err := gooq.Insert(gooq.User, map[string]any{
			"name":   "john",
			"age":    18,
			"status": "active",
		}).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		result, err := db.Exec(sqlStr, args...)
		t.AssertNil(err)
		affected, err := result.RowsAffected()
		t.AssertNil(err)
		t.Assert(affected, 1)

		// Select：gooq 类型化条件 → 标准库查询 → 手动扫描。
		sqlStr, args, err = gooq.SelectFrom(gooq.User).Ctx(context.Background()).
			Where(gooq.User.Name.Eq("john")).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		rows := queryRows(t, db, sqlStr, args)
		var users []UserEntity
		for rows.Next() {
			var u UserEntity
			t.AssertNil(rows.Scan(&u.Id, &u.Name, &u.Age, &u.Status, &u.DeletedAt, &u.CreatedAt))
			users = append(users, u)
		}
		t.AssertNil(rows.Close())
		t.Assert(len(users), 1)
		t.Assert(users[0].Name, "john")
		t.Assert(users[0].Age, 18)

		// Update：类型化 Set。
		sqlStr, args, err = gooq.Update(gooq.User).
			Set(gooq.User.Name, "john2").
			Where(gooq.User.ID.Eq(users[0].Id)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		_, err = db.Exec(sqlStr, args...)
		t.AssertNil(err)

		// Delete：软删（deleted_at 置位）。
		sqlStr, args, err = gooq.Delete(gooq.User).
			Where(gooq.User.ID.Eq(users[0].Id)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		_, err = db.Exec(sqlStr, args...)
		t.AssertNil(err)

		// 自动过滤：普通查询查不到已删数据。
		sqlStr, args, err = gooq.SelectFrom(gooq.User).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		rows = queryRows(t, db, sqlStr, args)
		count := 0
		for rows.Next() {
			count++
		}
		t.AssertNil(rows.Close())
		t.Assert(count, 0)

		// Unscoped：能查到，deleted_at 已置位。
		sqlStr, args, err = gooq.SelectFrom(gooq.User).Unscoped().ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		rows = queryRows(t, db, sqlStr, args)
		deletedAtSet := false
		for rows.Next() {
			var u UserEntity
			t.AssertNil(rows.Scan(&u.Id, &u.Name, &u.Age, &u.Status, &u.DeletedAt, &u.CreatedAt))
			deletedAtSet = u.DeletedAt != nil
		}
		t.AssertNil(rows.Close())
		t.Assert(deletedAtSet, true)
	})
}

// TestE2E_PageCount 验证 Count/Page 生成 SQL 的真实执行。
func TestE2E_PageCount(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		db := openDB(t)
		createTable(t, db)

		// 批量插入 25 条。
		var rowsData []map[string]any
		for i := 1; i <= 25; i++ {
			rowsData = append(rowsData, map[string]any{
				"name":   fmt.Sprintf("user%d", i),
				"age":    i,
				"status": "active",
			})
		}
		sqlStr, args, err := gooq.Insert(gooq.User, rowsData).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		_, err = db.Exec(sqlStr, args...)
		t.AssertNil(err)

		// Count。
		var count int
		t.AssertNil(db.QueryRow(
			"SELECT COUNT(*) FROM user WHERE age >= ? AND deleted_at IS NULL", 1,
		).Scan(&count))
		t.Assert(count, 25)

		// Page(1,10)。
		sqlStr, args, err = gooq.SelectFrom(gooq.User).Where(gooq.User.Age.Gte(1)).
			Page(1, 10).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		rows := queryRows(t, db, sqlStr, args)
		page1Count := 0
		for rows.Next() {
			page1Count++
		}
		t.AssertNil(rows.Close())
		t.Assert(page1Count, 10)

		// Page(3,10)。
		sqlStr, args, err = gooq.SelectFrom(gooq.User).Where(gooq.User.Age.Gte(1)).
			Page(3, 10).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		rows = queryRows(t, db, sqlStr, args)
		page3Count := 0
		for rows.Next() {
			page3Count++
		}
		t.AssertNil(rows.Close())
		t.Assert(page3Count, 5)
	})
}

// mapHashAdapter 是最小哈希实现（保留备用）。
type mapHashAdapter struct {
	mu sync.Mutex
	m  map[string]map[string][]byte
}

// HGet 实现 HashCacheAdapter 接口。
func (a *mapHashAdapter) HGet(ctx context.Context, key, field string) ([]byte, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	container, ok := a.m[key]
	if !ok {
		return nil, false, nil
	}
	value, ok := container[field]
	return value, ok, nil
}

// HSet 实现 HashCacheAdapter 接口。
func (a *mapHashAdapter) HSet(ctx context.Context, key, field string, value []byte, ttl time.Duration) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.m[key] == nil {
		a.m[key] = make(map[string][]byte)
	}
	a.m[key][field] = value
	return nil
}

// HDel 实现 HashCacheAdapter 接口。
func (a *mapHashAdapter) HDel(ctx context.Context, key string, fields ...string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	container, ok := a.m[key]
	if !ok {
		return nil
	}
	for _, field := range fields {
		delete(container, field)
	}
	return nil
}
