// 这个外部测试包导入内置驱动，让 db 包的测试可以打开连接，
// 同时避免 db 和驱动包之间的导入循环。

package db_test

import (
	_ "github.com/lanceadd/gooq/drivers/dm"
	_ "github.com/lanceadd/gooq/drivers/gaussdb"
	_ "github.com/lanceadd/gooq/drivers/mariadb"
	_ "github.com/lanceadd/gooq/drivers/mysql"
	_ "github.com/lanceadd/gooq/drivers/oceanbase"
	_ "github.com/lanceadd/gooq/drivers/pgsql"
	_ "github.com/lanceadd/gooq/drivers/sqlite"
	_ "github.com/lanceadd/gooq/drivers/tidb"
)
