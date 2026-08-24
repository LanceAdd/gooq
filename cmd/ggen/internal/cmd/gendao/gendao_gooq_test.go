// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件为 gooq 表对象生成的端到端测试：sqlite 建库 → gen dao → 产物断言。
package gendao

import (
	"context"
	"path/filepath"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/test/gtest"
	"github.com/gogf/gf/v2/text/gstr"
)

// initSqlite 创建 sqlite 测试库并建表。
func initSqlite(t *gtest.T, dbPath string) {
	err := gdb.AddConfigNode("ggen-test", gdb.ConfigNode{
		Link: "sqlite::@file(" + dbPath + ")",
	})
	t.AssertNil(err)
	db, err := gdb.Instance("ggen-test")
	t.AssertNil(err)
	_, err = db.Exec(context.Background(), `
CREATE TABLE IF NOT EXISTS user (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    age        INTEGER NOT NULL,
    status     TEXT NOT NULL DEFAULT 'active',
    deleted_at DATETIME,
    created_at DATETIME
)`)
	t.AssertNil(err)
}

// TestGooqGen_Sqlite 验证 sqlite 建库 → gen dao → 生成 gooq 表对象 → 产物内容断言。
func TestGooqGen_Sqlite(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		var (
			ctx     = context.Background()
			tmpDir  = gfile.Temp("ggen-gooq-test")
			dbPath  = filepath.Join(tmpDir, "gooq_test.db")
			genPath = filepath.Join(tmpDir, "gen")
		)
		defer gfile.Remove(tmpDir)
		t.AssertNil(gfile.Mkdir(genPath))
		initSqlite(t, dbPath)

		_, err := (CGenDao{}).Dao(ctx, CGenDaoInput{
			Link:      "sqlite::@file(" + dbPath + ")",
			Path:      genPath,
			TablePath: "table",
			Tables:    "user",
		})
		t.AssertNil(err)

		// 产物文件存在。
		generated := filepath.Join(genPath, "table", "user.go")
		t.Assert(gfile.Exists(generated), true)
		content := gfile.GetContents(generated)

		// 类型化表对象骨架。
		t.Assert(gstr.Contains(content, `type UserTable struct {`), true)
		t.Assert(gstr.Contains(content, `*gooq.TableBase`), true)
		// 全列由 TableBase.AllFields() 从元数据派生，生成代码不重复维护列名列表。
		t.Assert(gstr.Contains(content, `ALL_FIELDS`), false)

		// 字段定义（Go 类型映射：sqlite INTEGER → int，datetime → *gtime.Time）。
		t.Assert(gstr.Contains(content, `gooq.Field[int]`), true)
		t.Assert(gstr.Contains(content, `gooq.Field[string]`), true)
		t.Assert(gstr.Contains(content, `gooq.Field[*gtime.Time]`), true)

		// 全局表对象与元数据（软删标记由列名约定识别，与驱动无关）。
		t.Assert(gstr.Contains(content, `var User = &UserTable{`), true)
		t.Assert(gstr.Contains(content, `gooq.TableMeta{`), true)
		t.Assert(gstr.Contains(content, `TableName: "user"`), true)
		t.Assert(gstr.Contains(content, `LocalType: gooq.LocalType("int")`), true)
		t.Assert(gstr.Contains(content, `SoftDelete: true`), true)

		// NewField 赋值与 As/Clone。
		t.Assert(gstr.Contains(content, `gooq.NewField[int]("user", "id")`), true)
		t.Assert(gstr.Contains(content, `func (t *UserTable) As(alias string) *UserTable {`), true)
		t.Assert(gstr.Contains(content, `func (t *UserTable) Clone() *UserTable {`), true)

		// FieldMeta 含软删列。
		t.Assert(gstr.Contains(content, `"deleted_at"`), true)

		// 主键/自增标记依赖驱动返回 Key/Extra（sqlite 驱动不支持），由 MySQL e2e 验证。
	})
}
