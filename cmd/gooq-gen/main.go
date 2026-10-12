// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

package main

import (
	_ "time/tzdata"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gctx"

	"github.com/lanceadd/gooq/cmd/gooq-gen/gfcmd"
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
	var (
		ctx          = gctx.GetInitCtx()
		command, err = gfcmd.GetCommand(ctx)
	)

	if err != nil {
		mlog.Fatalf(`%+v`, err)
	}
	if command == nil {
		panic(gerror.New(`retrieve root command failed for "gooq-gen"`))
	}
	command.Run(ctx)
}
