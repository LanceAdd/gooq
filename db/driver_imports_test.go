// This external test package imports the built-in drivers so package db tests can open
// connections without an import cycle between db and the driver packages.

package db_test

import (
	_ "github.com/lanceadd/gooq/drivers/mysql"
	_ "github.com/lanceadd/gooq/drivers/pgsql"
	_ "github.com/lanceadd/gooq/drivers/sqlite"
)
