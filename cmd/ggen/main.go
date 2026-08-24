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

	"github.com/gogf/gf/cmd/ggen/gfcmd"
	"github.com/gogf/gf/cmd/ggen/internal/utility/mlog"
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
		panic(gerror.New(`retrieve root command failed for "gf"`))
	}
	command.Run(ctx)
}
