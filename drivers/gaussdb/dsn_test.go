package gaussdb

import (
	"strings"
	"testing"

	"github.com/lanceadd/gooq/db"
)

func TestGaussDBDSN(t *testing.T) {
	dsn, err := gaussdbDriver{}.DSN(db.ParsedLink{
		Host:  "127.0.0.1",
		Port:  "8000",
		User:  "gaussdb",
		Pass:  "a b",
		Name:  "app",
		Extra: "sslmode=require",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dsn, "password='a b'") {
		t.Fatalf("password was not quoted: %s", dsn)
	}
	if !strings.Contains(dsn, "port=8000") || !strings.Contains(dsn, "sslmode=require") {
		t.Fatalf("dsn = %s", dsn)
	}
	if strings.Count(dsn, "sslmode=") != 1 {
		t.Fatalf("sslmode repeated: %s", dsn)
	}
	withCharset := gaussdbDriver{}.ApplySession(dsn, db.Config{Charset: "UTF8"})
	if !strings.Contains(withCharset, "client_encoding=UTF8") {
		t.Fatalf("charset missing: %s", withCharset)
	}
}
