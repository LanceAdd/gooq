package main

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gmeta"
	"github.com/gogf/gf/v2/util/gtag"

	"github.com/lanceadd/gooq/cmd/gooq-gen/internal/gen"
)

const (
	cliFolderName = `hack`
	cliConfigName = `gooq.yaml`
)

// appCmd 是命令对象：Index 是根行为，Init 自动成为子命令。
type appCmd struct {
	gmeta.Meta `name:"gooq-gen" brief:"{cGenBrief}" usage:"gooq-gen [OPTION]" eg:"{cGenEg}" ad:"{cAd}"`
}

const (
	cAd = `
ADDITIONAL
    Use "gooq-gen -h" for details about a command.
`
	cGenBrief = `generate gooq typed table files for all tables of a database`
	cGenEg    = `
gooq-gen -l "mysql:root:12345678@tcp(127.0.0.1:3306)/test"
gooq-gen -l "sqlite::@file(./test.db)"
`
	cInitBrief = `initialize hack/gooq.yaml and hack/template/{table,model,record}.tmpl for customization`
)

func init() {
	gtag.Sets(map[string]string{
		`cAd`:        cAd,
		`cGenBrief`:  cGenBrief,
		`cGenEg`:     cGenEg,
		`cInitBrief`: cInitBrief,
	})
}

type genInput struct {
	gmeta.Meta `name:"gooq-gen"`
	Debug      bool   `short:"d" name:"debug"   brief:"show internal detailed debugging information" orphan:"true"`
	Link       string `name:"link" short:"l" brief:"database link, like mysql:root:pass@tcp(127.0.0.1:3306)/db"`
	Path       string `name:"path" short:"p" brief:"directory path for generated files, default internal"`
}

type genOutput struct{}

func (c appCmd) Index(ctx context.Context, in genInput) (out *genOutput, err error) {
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

type initInput struct {
	gmeta.Meta `name:"init" brief:"{cInitBrief}"`
}

type initOutput struct{}

func (c appCmd) Init(ctx context.Context, in initInput) (out *initOutput, err error) {
	gen.Init()
	return
}
