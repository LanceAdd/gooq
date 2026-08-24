// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件为 ggen 的端到端测试：sqlite 建库 → Generate → do/entity/gooq table 产物断言。
package gendao

import (
	"context"
	"path/filepath"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/test/gtest"
	"github.com/gogf/gf/v2/text/gregex"
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

// TestGooqGen_Sqlite 验证 sqlite 建库 → Generate → 生成 do/entity/gooq table 三类产物 → 产物内容断言。
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

		Generate(ctx, Input{
			Link: "sqlite::@file(" + dbPath + ")",
			Path: genPath,
		})

		// gooq table 产物。
		generated := filepath.Join(genPath, "table", "user.go")
		t.Assert(gfile.Exists(generated), true)
		content := gfile.GetContents(generated)

		// 类型化表对象骨架。
		t.Assert(gstr.Contains(content, `type UserTable struct {`), true)
		t.Assert(gstr.Contains(content, `*gooq.TableBase`), true)
		// 全列由 TableBase.AllFields() 从元数据派生，生成代码不重复维护列名列表。
		t.Assert(gstr.Contains(content, `ALL_FIELDS`), false)

		// 字段定义（Go 类型映射：sqlite INTEGER → int，datetime → time.Time）。
		t.Assert(gstr.Contains(content, `gooq.Field[int]`), true)
		t.Assert(gstr.Contains(content, `gooq.Field[string]`), true)
		t.Assert(gstr.Contains(content, `gooq.Field[time.Time]`), true)

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

		// do 产物：字段类型统一为 any，带 g.Meta 标记。
		doFile := filepath.Join(genPath, "do", "user.go")
		t.Assert(gfile.Exists(doFile), true)
		doContent := gfile.GetContents(doFile)
		t.Assert(gstr.Contains(doContent, `type User struct {`), true)
		t.Assert(gstr.Contains(doContent, `g.Meta`), true)
		t.Assert(gstr.Contains(doContent, `orm:"table:user, do:true"`), true)
		t.Assert(gregex.IsMatchString(`(?m)^\s+Name\s+any\s+//`, doContent), true)
		t.Assert(gregex.IsMatchString(`(?m)^\s+DeletedAt\s+any\s+//`, doContent), true)

		// entity 产物：带 json/orm tag，时间字段 time.Time。
		entityFile := filepath.Join(genPath, "entity", "user.go")
		t.Assert(gfile.Exists(entityFile), true)
		entityContent := gfile.GetContents(entityFile)
		t.Assert(gstr.Contains(entityContent, `type User struct {`), true)
		t.Assert(gstr.Contains(entityContent, `json:"name"`), true)
		t.Assert(gstr.Contains(entityContent, `orm:"name"`), true)
		t.Assert(gstr.Contains(entityContent, `DeletedAt time.Time`), true)
		t.Assert(gstr.Contains(entityContent, `"time"`), true)
	})
}
