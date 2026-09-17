// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件为 gooq-gen 的端到端测试：sqlite 建库 → Generate/init 子命令 → 产物断言。
package gen

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/test/gtest"
	"github.com/gogf/gf/v2/text/gstr"
)

// initSqlite 创建 sqlite 测试库并建表（SetConfigGroup 整组替换，可跨测试重复运行）。
func initSqlite(t *gtest.T, dbPath string) {
	err := gdb.SetConfigGroup("gooq-gen-test", gdb.ConfigGroup{{
		Link: "sqlite::@file(" + dbPath + ")",
	}})
	t.AssertNil(err)
	db, err := gdb.Instance("gooq-gen-test")
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

// goTool 返回 go 可执行文件路径（优先 PATH，回退 GOROOT/bin）。
func goTool() string {
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(runtime.GOROOT(), "bin", "go.exe")
	}
	return filepath.Join(runtime.GOROOT(), "bin", "go")
}

var (
	binOnce sync.Once
	binPath string
)

// buildBinary 编译一次 gooq-gen 二进制（跨测试复用，位于独立临时目录避免被用例清理）。
func buildBinary(t *gtest.T) string {
	binOnce.Do(func() {
		// 模块根目录经文件位置推导（不受工作目录影响）。
		_, file, _, _ := runtime.Caller(0)
		moduleDir := filepath.Join(filepath.Dir(file), "..", "..", "..")
		path := filepath.Join(gfile.Temp("gooq-gen-bin"), "gooq-gen.exe")
		if err := gfile.Mkdir(gfile.Dir(path)); err != nil {
			panic(err)
		}
		cmd := exec.Command(goTool(), "build", "-o", path, ".")
		cmd.Dir = moduleDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			panic("build gooq-gen failed: " + err.Error() + "\n" + string(out))
		}
		binPath = path
	})
	return binPath
}

// TestGen_Sqlite 验证 sqlite 建库 → Generate → 生成 gooq table 产物 → 产物内容断言。
func TestGen_Sqlite(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		var (
			ctx     = context.Background()
			tmpDir  = gfile.Temp("gooq-gen-test")
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
		t.Assert(gstr.Contains(content, `var User = newUserTable()`), true)
		t.Assert(gstr.Contains(content, `gooq.TableMeta{`), true)
		t.Assert(gstr.Contains(content, `TableName: "user"`), true)
		t.Assert(gstr.Contains(content, `LocalType: gooq.LocalType("int")`), true)
		t.Assert(gstr.Contains(content, `SoftDelete: true`), true)

		// NewFieldAt 赋值与 As/Clone。
		t.Assert(gstr.Contains(content, `t.Id = gooq.NewFieldAt[int](t.TableBase, "id")`), true)
		t.Assert(gstr.Contains(content, `func (t *UserTable) As(alias string) *UserTable {`), true)
		t.Assert(gstr.Contains(content, `func (t *UserTable) Clone() *UserTable {`), true)

		// FieldMeta 含软删列。
		t.Assert(gstr.Contains(content, `"deleted_at"`), true)

		// 生成的文件头标识。
		t.Assert(gstr.Contains(content, `Code generated and maintained by gooq-gen. DO NOT EDIT.`), true)

		// 不再生成 do/entity 产物。
		t.Assert(gfile.Exists(filepath.Join(genPath, "do")), false)
		t.Assert(gfile.Exists(filepath.Join(genPath, "entity")), false)
	})
}

// TestInit 验证 init 子命令：生成 hack/gooq.yaml 与 hack/template/table.tmpl，已存在文件不覆盖。
func TestInit(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		var (
			tmpDir = gfile.Temp("gooq-gen-init-test")
			bin    = buildBinary(t)
		)
		defer gfile.Remove(tmpDir)
		t.AssertNil(gfile.Mkdir(tmpDir))

		run := func(args ...string) ([]byte, error) {
			cmd := exec.Command(bin, args...)
			cmd.Dir = tmpDir
			return cmd.CombinedOutput()
		}

		cfgPath := filepath.Join(tmpDir, "hack", "gooq.yaml")
		tplPath := filepath.Join(tmpDir, "hack", "template", "table.tmpl")

		// 首次 init：生成配置与模板。
		out, err := run("init")
		if err != nil {
			t.Fatalf("init failed: %v\n%s", err, out)
		}
		t.Assert(gfile.Exists(cfgPath), true)
		t.Assert(gfile.Exists(tplPath), true)
		cfgContent := gfile.GetContents(cfgPath)
		t.Assert(gstr.Contains(cfgContent, "gooq:"), true)
		t.Assert(gstr.Contains(cfgContent, "gen:"), true)
		t.Assert(gstr.Contains(cfgContent, "table:"), true)
		t.Assert(gstr.Contains(gfile.GetContents(tplPath), "TableBase"), true)

		// 二次 init：已存在文件不覆盖。
		t.AssertNil(gfile.PutContents(cfgPath, "# CUSTOM_CONFIG\n"))
		out, err = run("init")
		if err != nil {
			t.Fatalf("second init failed: %v\n%s", err, out)
		}
		t.Assert(gfile.GetContents(cfgPath), "# CUSTOM_CONFIG\n")
		t.Assert(gstr.Contains(string(out), "skipped"), true)
	})
}

// TestGen_Config 验证 hack/gooq.yaml（gooq.gen.table）配置读取与 hack/template 本地模板覆盖：
// 无参执行时 link/path 来自配置文件；本地模板优先于内置模板；无配置且无参数时报错退出。
func TestGen_Config(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		var (
			tmpDir  = gfile.Temp("gooq-gen-cfg-test")
			dbPath  = filepath.Join(tmpDir, "gooq_test.db")
			workDir = filepath.Join(tmpDir, "work")
			bin     = buildBinary(t)
		)
		defer gfile.Remove(tmpDir)
		t.AssertNil(gfile.Mkdir(workDir))
		initSqlite(t, dbPath)

		// hack/gooq.yaml（sqlite link 使用正斜杠避免 YAML 转义）+ 输出目录。
		cfg := "gooq:\n  gen:\n    table:\n" +
			"      link: \"sqlite::@file(" + filepath.ToSlash(dbPath) + ")\"\n" +
			"      path: out\n"
		t.AssertNil(gfile.PutContents(filepath.Join(workDir, "hack", "gooq.yaml"), cfg))
		t.AssertNil(gfile.Mkdir(filepath.Join(workDir, "out")))

		run := func() ([]byte, error) {
			cmd := exec.Command(bin)
			cmd.Dir = workDir
			return cmd.CombinedOutput()
		}

		// 无参数执行：link/path 全部来自配置文件。
		out, err := run()
		if err != nil {
			t.Fatalf("run with config failed: %v\n%s", err, out)
		}
		generated := filepath.Join(workDir, "out", "table", "user.go")
		t.Assert(gfile.Exists(generated), true)
		t.Assert(gstr.Contains(gfile.GetContents(generated), `type UserTable struct {`), true)

		// hack/template/table.tmpl 本地模板优先于内置模板。
		t.AssertNil(gfile.PutContents(
			filepath.Join(workDir, "hack", "template", "table.tmpl"),
			"// CUSTOM_TEMPLATE_MARKER\npackage {{.TplPackageName}}\n\n"+
				"var {{.TplTableNameCamelCase}}Generated = \"{{.TplTableName}}\"\n",
		))
		out, err = run()
		if err != nil {
			t.Fatalf("run with local template failed: %v\n%s", err, out)
		}
		content := gfile.GetContents(generated)
		t.Assert(gstr.Contains(content, "CUSTOM_TEMPLATE_MARKER"), true)
		t.Assert(gstr.Contains(content, `UserGenerated = "user"`), true)

		// 无配置且无参数：报错退出。
		noCfgDir := filepath.Join(tmpDir, "no-cfg")
		t.AssertNil(gfile.Mkdir(noCfgDir))
		cmd := exec.Command(bin)
		cmd.Dir = noCfgDir
		_, err = cmd.CombinedOutput()
		t.Assert(err != nil, true)
	})
}
