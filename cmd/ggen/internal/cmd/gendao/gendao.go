// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件实现 ggen 主流程：连接数据库 → 解析全部表 → 生成 do/entity/gooq table 三类产物。
package gendao

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/os/gview"

	"github.com/gogf/gf/cmd/ggen/internal/mlog"
)

// Input is the input parameter of ggen.
type Input struct {
	Link string // 数据库连接，如 mysql:root:pass@tcp(127.0.0.1:3306)/db。
	Path string // 输出目录，默认 internal。
}

// 输出子目录（固定结构）。
const (
	dirPathDo     = "do"
	dirPathEntity = "entity"
	dirPathTable  = "table"
)

// tplView 是模板渲染视图（跨表生成复用）。
var tplView = gview.New()

// 内置模板文件名。
const (
	tplFileDo     = "do.tpl"
	tplFileEntity = "entity.tpl"
	tplFileTable  = "table.tmpl"
)

// getTemplate 读取模板内容：优先使用工作目录下 template/ 目录中的本地模板（便于调试定制），
// 否则回退到内嵌模板。
func getTemplate(name string) string {
	if path := gfile.Join("template", name); gfile.Exists(path) {
		return gfile.GetContents(path)
	}
	content, err := gooqTemplateFS.ReadFile("template/" + name)
	if err != nil {
		mlog.Fatalf(`reading embedded template "%s" failed: %+v`, name, err)
	}
	return string(content)
}

// ExportTemplates 将内置模板落盘到当前工作目录的 template/ 目录（-t 参数使用）。
func ExportTemplates() {
	if !gfile.Exists("template") {
		if err := gfile.Mkdir("template"); err != nil {
			mlog.Fatalf(`creating template directory failed: %+v`, err)
		}
	}
	for _, name := range []string{tplFileDo, tplFileEntity, tplFileTable} {
		path := gfile.Join("template", name)
		if err := gfile.PutContents(path, getTemplate(name)); err != nil {
			mlog.Fatalf(`writing template "%s" failed: %+v`, path, err)
		}
		mlog.Print("exported:", gfile.RealPath(path))
	}
	mlog.Print(fmt.Sprintf(`templates exported, edit files under "template/" then re-run ggen`))
}

// Generate generates do/entity/gooq-table files for all tables of the given database.
func Generate(ctx context.Context, in Input) {
	if in.Path == "" {
		in.Path = "internal"
	}
	if dirRealPath := gfile.RealPath(in.Path); dirRealPath == "" {
		mlog.Fatalf(`path "%s" does not exist`, in.Path)
	}
	if in.Link == "" {
		mlog.Fatalf(`database link is required, use -l/--link`)
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

	// Do.
	generateDo(ctx, db, tableNames, gfile.Join(in.Path, dirPathDo))
	// Entity.
	generateEntity(ctx, db, tableNames, gfile.Join(in.Path, dirPathEntity))
	// Gooq table.
	generateTable(ctx, db, tableNames, gfile.Join(in.Path, dirPathTable))

	mlog.Print("done!")
}
