package mariadb

import (
	"testing"

	"github.com/lanceadd/gooq/db"
)

func TestMariaDBDSN(t *testing.T) {
	dsn, err := mariadbDriver{}.DSN(db.ParsedLink{
		User:     "root",
		Pass:     "secret",
		Protocol: "tcp",
		Host:     "127.0.0.1",
		Port:     "3306",
		Name:     "app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dsn != "root:secret@tcp(127.0.0.1:3306)/app?parseTime=true" {
		t.Fatalf("dsn = %s", dsn)
	}
	dsn, err = mariadbDriver{}.DSN(db.ParsedLink{
		User: "root", Pass: "secret", Protocol: "tcp", Host: "db.internal", Name: "app", Extra: "charset=utf8mb4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dsn != "root:secret@tcp(db.internal)/app?charset=utf8mb4&parseTime=true" {
		t.Fatalf("dsn without port = %s", dsn)
	}
}
