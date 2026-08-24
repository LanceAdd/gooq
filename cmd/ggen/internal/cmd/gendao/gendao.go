// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件实现 ggen 主流程：连接数据库 → 解析全部表 → 生成 do/entity/gooq table 三类产物。
package gendao

import (
	"context"

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
	generateGooqTable(ctx, db, tableNames, gfile.Join(in.Path, dirPathTable))

	mlog.Print("done!")
}
