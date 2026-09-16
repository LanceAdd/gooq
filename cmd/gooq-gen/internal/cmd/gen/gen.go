// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件实现 gooq-gen 主流程：连接数据库 → 解析全部表 → 生成 gooq table 产物。
package gen

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/os/gview"

	"github.com/lanceadd/gooq/cmd/gooq-gen/internal/mlog"
)

// Input is the input parameter of gooq-gen.
type Input struct {
	Link string // 数据库连接，如 mysql:root:pass@tcp(127.0.0.1:3306)/db。
	Path string // 输出目录，默认 internal。
}

// 固定目录与文件名。
const (
	dirPathTable   = "table"
	tplFileTable   = "table.tmpl"
	hackDirName    = "hack"
	tplDirName     = "template"
	configFileName = "gooq.yaml"
)

// initConfigContent 是 init 命令生成的配置模板。
const initConfigContent = `# gooq-gen configuration; CLI options take priority over this file.
gooq:
  gen:
    table:
      # Database link, same format as the ORM configuration of GoFrame.
      # link: "mysql:root:pass@tcp(127.0.0.1:3306)/db"
      path: internal
`

// tplView 是模板渲染视图（跨表生成复用）。
var tplView = gview.New()

// hackDirPath 向上查找 hack 目录（不存在返回空串）。
func hackDirPath() string {
	dir, _ := gfile.Search(hackDirName)
	return dir
}

// getTemplate 读取模板内容：优先使用 hack/template/ 下的本地模板（便于定制），
// 否则回退到内嵌模板。
func getTemplate(name string) string {
	if dir := hackDirPath(); dir != "" {
		if path := gfile.Join(dir, tplDirName, name); gfile.Exists(path) {
			return gfile.GetContents(path)
		}
	}
	return embeddedTemplate(name)
}

// embeddedTemplate 返回内嵌模板内容。
func embeddedTemplate(name string) string {
	content, err := gooqTemplateFS.ReadFile("template/" + name)
	if err != nil {
		mlog.Fatalf(`reading embedded template "%s" failed: %+v`, name, err)
	}
	return string(content)
}

// Init 在当前工作目录创建 hack/gooq.yaml 与 hack/template/table.tmpl；已存在的文件不覆盖。
func Init() {
	for _, f := range []struct{ path, content string }{
		{gfile.Join(hackDirName, configFileName), initConfigContent},
		{gfile.Join(hackDirName, tplDirName, tplFileTable), embeddedTemplate(tplFileTable)},
	} {
		if gfile.Exists(f.path) {
			mlog.Print("skipped, already exists:", gfile.RealPath(f.path))
			continue
		}
		if err := gfile.PutContents(f.path, f.content); err != nil {
			mlog.Fatalf(`writing "%s" failed: %+v`, f.path, err)
		}
		mlog.Print("generated:", gfile.RealPath(f.path))
	}
	mlog.Print(fmt.Sprintf(`edit "%s" to set the database link, then run gooq-gen`, gfile.Join(hackDirName, configFileName)))
}

// Generate generates gooq typed table files for all tables of the given database.
func Generate(ctx context.Context, in Input) {
	if in.Path == "" {
		in.Path = "internal"
	}
	if in.Link == "" {
		mlog.Fatalf(`database link is required, use -l/--link or set "gooq.gen.table.link" in hack/gooq.yaml`)
	}
	if dirRealPath := gfile.RealPath(in.Path); dirRealPath == "" {
		mlog.Fatalf(`path "%s" does not exist`, in.Path)
	}
	var (
		err error
		db  gdb.DB
	)
	// It uses user passed database configuration.
	var tempGroup = gtime.TimestampNanoStr()
	err = gdb.AddConfigNode(tempGroup, gdb.ConfigNode{
		Link: in.Link,
	})
	if err != nil {
		mlog.Fatalf(`database configuration failed: %+v`, err)
	}
	if db, err = gdb.Instance(tempGroup); err != nil {
		mlog.Fatalf(`database initialization failed: %+v`, err)
	}

	// 一次性生成全部表。
	tableNames, err := db.Tables(context.TODO())
	if err != nil {
		mlog.Fatalf("fetching tables failed: %+v", err)
	}
	generateTable(ctx, db, tableNames, gfile.Join(in.Path, dirPathTable))

	mlog.Print("done!")
}
