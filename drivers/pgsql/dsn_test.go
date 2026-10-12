package pgsql

import (
	"strings"
	"testing"

	"github.com/lanceadd/gooq/db"
)

func TestDSNQuotesAndExtras(t *testing.T) {
	dsn, err := postgresDriver{}.DSN(db.ParsedLink{
		Host:  "127.0.0.1",
		Port:  "5432",
		User:  "gooq",
		Pass:  "a b",
		Name:  "gooq",
		Extra: "application_name=gooq_app&sslmode=require",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dsn, "password='a b'") {
		t.Fatalf("password was not quoted: %s", dsn)
	}
	if !strings.Contains(dsn, "sslmode=require") {
		t.Fatalf("sslmode was not overridden: %s", dsn)
	}
	if !strings.Contains(dsn, "application_name=gooq_app") {
		t.Fatalf("application_name missing: %s", dsn)
	}
	if strings.Count(dsn, "sslmode=") != 1 {
		t.Fatalf("sslmode repeated: %s", dsn)
	}
	withCharset := postgresDriver{}.ApplySession(dsn, db.Config{Charset: "UTF8"})
	if !strings.Contains(withCharset, "client_encoding=UTF8") {
		t.Fatalf("charset missing: %s", withCharset)
	}
}
