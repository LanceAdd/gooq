// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// Package cmd provides the management of CLI commands for `ggen` tool.
package cmd

import (
	"context"

	_ "github.com/gogf/gf/contrib/drivers/mysql/v2"
	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"

	"github.com/gogf/gf/v2/util/gmeta"
	"github.com/gogf/gf/v2/util/gtag"

	"github.com/gogf/gf/cmd/ggen/internal/cmd/gendao"
)

// 使用 DM 等驱动时，在下方取消对应行注释并执行 go mod tidy：
// import (
//     _ "github.com/gogf/gf/contrib/drivers/dm/v2"
// )

// GF is the management object for `ggen` command line tool.
var GF = cGF{}

type cGF struct {
	gmeta.Meta `name:"ggen" brief:"{cGenBrief}" usage:"ggen [OPTION]" eg:"{cGenEg}" ad:"{cGFAd}"`
}

const (
	cGFAd = `
ADDITIONAL
    Use "ggen -h" for details about a command.
`
	cGenBrief = `generate do/entity/gooq-table files for all tables of a database`
	cGenEg    = `
ggen -l "mysql:root:12345678@tcp(127.0.0.1:3306)/test"
ggen -l "sqlite::@file(./test.db)"
`
)

func init() {
	gtag.Sets(map[string]string{
		`cGFAd`:     cGFAd,
		`cGenBrief`: cGenBrief,
		`cGenEg`:    cGenEg,
	})
}

type cGFInput struct {
	gmeta.Meta `name:"ggen"`
	Yes        bool   `short:"y" name:"yes"     brief:"all yes for all command without prompt ask"   orphan:"true"`
	Debug      bool   `short:"d" name:"debug"   brief:"show internal detailed debugging information" orphan:"true"`
	Link       string `name:"link" short:"l" brief:"database link, like mysql:root:pass@tcp(127.0.0.1:3306)/db"`
	Path       string `name:"path" short:"p" brief:"directory path for generated files" d:"internal"`
}

type cGFOutput struct{}

func (c cGF) Index(ctx context.Context, in cGFInput) (out *cGFOutput, err error) {
	gendao.Generate(ctx, gendao.Input{
		Link: in.Link,
		Path: in.Path,
	})
	return
}
