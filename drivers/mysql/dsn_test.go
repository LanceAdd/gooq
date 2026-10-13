package mysql

import (
	"testing"

	"github.com/lanceadd/gooq/db"
)

func TestMySQLDSNWithoutPort(t *testing.T) {
	dsn, err := mysqlDriver{}.DSN(db.ParsedLink{
		User: "root", Pass: "secret", Protocol: "tcp", Host: "db.internal", Name: "app", Extra: "charset=utf8mb4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dsn != "root:secret@tcp(db.internal)/app?charset=utf8mb4&parseTime=true" {
		t.Fatalf("dsn = %s", dsn)
	}
}
