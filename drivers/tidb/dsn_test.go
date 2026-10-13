package tidb

import (
	"strings"
	"testing"

	"github.com/lanceadd/gooq/db"
)

func TestTiDBDSN(t *testing.T) {
	dsn, err := tidbDriver{}.DSN(db.ParsedLink{
		User:     "root",
		Pass:     "secret",
		Protocol: "tcp",
		Host:     "127.0.0.1",
		Port:     "4000",
		Name:     "app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dsn != "root:secret@tcp(127.0.0.1:4000)/app?parseTime=true" {
		t.Fatalf("dsn = %s", dsn)
	}
	dsn, err = tidbDriver{}.DSN(db.ParsedLink{
		User: "root", Pass: "secret", Protocol: "tcp", Host: "db.internal", Name: "app", Extra: "charset=utf8mb4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dsn != "root:secret@tcp(db.internal)/app?charset=utf8mb4&parseTime=true" {
		t.Fatalf("dsn without port = %s", dsn)
	}
	withSession := tidbDriver{}.ApplySession(dsn, db.Config{Charset: "utf8mb4", Timezone: "Local"})
	if !strings.Contains(withSession, "charset=utf8mb4") || !strings.Contains(withSession, "loc=Local") {
		t.Fatalf("session dsn = %s", withSession)
	}
}
