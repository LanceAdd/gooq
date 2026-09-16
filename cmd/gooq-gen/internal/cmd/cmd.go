// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// Package cmd provides the management of CLI commands for `gooq-gen` tool.
package cmd

import (
	"context"

	_ "github.com/gogf/gf/contrib/drivers/mysql/v2"
	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gmeta"
	"github.com/gogf/gf/v2/util/gtag"

	"github.com/lanceadd/gooq/cmd/gooq-gen/internal/cmd/gen"
)

// 使用 DM 等驱动时，在下方取消对应行注释并执行 go mod tidy：
// import (
//     _ "github.com/gogf/gf/contrib/drivers/dm/v2"
// )

// GF is the management object for `gooq-gen` command line tool.
var GF = cGF{}

type cGF struct {
	gmeta.Meta `name:"gooq-gen" brief:"{cGenBrief}" usage:"gooq-gen [OPTION]" eg:"{cGenEg}" ad:"{cGFAd}"`
}

const (
	cGFAd = `
ADDITIONAL
    Use "gooq-gen -h" for details about a command.
`
	cGenBrief = `generate gooq typed table files for all tables of a database`
	cGenEg    = `
gooq-gen -l "mysql:root:12345678@tcp(127.0.0.1:3306)/test"
gooq-gen -l "sqlite::@file(./test.db)"
`
	cInitBrief = `initialize hack/gooq.yaml and hack/template/table.tmpl for customization`
)

func init() {
	gtag.Sets(map[string]string{
		`cGFAd`:      cGFAd,
		`cGenBrief`:  cGenBrief,
		`cGenEg`:     cGenEg,
		`cInitBrief`: cInitBrief,
	})
}

type cGFInput struct {
	gmeta.Meta `name:"gooq-gen"`
	Debug      bool   `short:"d" name:"debug"   brief:"show internal detailed debugging information" orphan:"true"`
	Link       string `name:"link" short:"l" brief:"database link, like mysql:root:pass@tcp(127.0.0.1:3306)/db"`
	Path       string `name:"path" short:"p" brief:"directory path for generated files, default internal"`
}

type cGFOutput struct{}

func (c cGF) Index(ctx context.Context, in cGFInput) (out *cGFOutput, err error) {
	// 配置兜底：命令行未提供的参数从 hack/gooq.yaml 的 gooq.gen.table 节点读取。
	if g.Cfg().Available(ctx) {
		if in.Link == "" {
			if v, err := g.Cfg().Get(ctx, "gooq.gen.table.link"); err == nil && v != nil {
				in.Link = v.String()
			}
		}
		if in.Path == "" {
			if v, err := g.Cfg().Get(ctx, "gooq.gen.table.path"); err == nil && v != nil {
				in.Path = v.String()
			}
		}
	}
	gen.Generate(ctx, gen.Input{
		Link: in.Link,
		Path: in.Path,
	})
	return
}

type cInitInput struct {
	gmeta.Meta `name:"init" brief:"{cInitBrief}"`
}

type cInitOutput struct{}

func (c cGF) Init(ctx context.Context, in cInitInput) (out *cInitOutput, err error) {
	gen.Init()
	return
}
