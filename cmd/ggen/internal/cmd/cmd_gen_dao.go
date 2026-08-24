// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

package cmd

import (
	_ "github.com/gogf/gf/contrib/drivers/clickhouse/v2"
	_ "github.com/gogf/gf/contrib/drivers/mssql/v2"
	_ "github.com/gogf/gf/contrib/drivers/mysql/v2"
	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"

	"github.com/gogf/gf/cmd/ggen/internal/cmd/gendao"
)

// 使用 PostgreSQL/DM 等驱动时，在下方取消对应行注释并执行 go mod tidy：
// import (
//     _ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
//     _ "github.com/gogf/gf/contrib/drivers/dm/v2"
// )

type (
	cGenDao = gendao.CGenDao
)
