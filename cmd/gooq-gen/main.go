package main

import (
	"context"
	_ "time/tzdata"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/gcmd"
	"github.com/gogf/gf/v2/os/gctx"
	"github.com/gogf/gf/v2/os/gfile"

	"github.com/lanceadd/gooq/cmd/gooq-gen/internal/mlog"
	_ "github.com/lanceadd/gooq/drivers/dm"
	_ "github.com/lanceadd/gooq/drivers/gaussdb"
	_ "github.com/lanceadd/gooq/drivers/mariadb"
	_ "github.com/lanceadd/gooq/drivers/mysql"
	_ "github.com/lanceadd/gooq/drivers/oceanbase"
	_ "github.com/lanceadd/gooq/drivers/pgsql"
	_ "github.com/lanceadd/gooq/drivers/sqlite"
	_ "github.com/lanceadd/gooq/drivers/tidb"
)

func main() {
	ctx := gctx.GetInitCtx()
	command, err := gcmd.NewFromObject(appCmd{})
	if err != nil {
		mlog.Fatalf(`%+v`, err)
	}
	if command == nil {
		mlog.Fatalf(`retrieve root command failed for "gooq-gen"`)
	}
	runCommand(ctx, command)
}

// runCommand 优先使用 hack/gooq.yaml 作为配置来源，并把 panic 收成日志。
func runCommand(ctx context.Context, command *gcmd.Command) {
	defer func() {
		if err := recover(); err != nil {
			mlog.Print(err)
		}
	}()

	if path, _ := gfile.Search(cliFolderName); path != "" {
		if adapter, ok := g.Cfg().GetAdapter().(*gcfg.AdapterFile); ok {
			if err := adapter.SetPath(path); err != nil {
				mlog.Fatal(err)
			}
			adapter.SetFileName(cliConfigName)
		}
	}
	if err := command.RunWithError(ctx); err != nil {
		mlog.Fatalf(`%+v`, err)
	}
}
