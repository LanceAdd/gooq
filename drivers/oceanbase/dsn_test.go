package oceanbase

import (
	"net"
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"

	"github.com/lanceadd/gooq/db"
)

func TestOceanBaseDSN(t *testing.T) {
	dsn, err := oceanbaseDriver{}.DSN(db.ParsedLink{
		User:     "root",
		Pass:     "secret",
		Protocol: "tcp",
		Host:     "127.0.0.1",
		Port:     "2881",
		Name:     "app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dsn != "root:secret@tcp(127.0.0.1:2881)/app?parseTime=true" {
		t.Fatalf("dsn = %s", dsn)
	}
	dsn, err = oceanbaseDriver{}.DSN(db.ParsedLink{
		User: "root", Pass: "secret", Protocol: "tcp", Host: "db.internal", Name: "app", Extra: "charset=utf8mb4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dsn != "root:secret@tcp(db.internal)/app?charset=utf8mb4&parseTime=true" {
		t.Fatalf("dsn without port = %s", dsn)
	}
	withSession := oceanbaseDriver{}.ApplySession(dsn, db.Config{Charset: "utf8mb4", Timezone: "Local"})
	if !strings.Contains(withSession, "charset=utf8mb4") || !strings.Contains(withSession, "loc=Local") {
		t.Fatalf("session dsn = %s", withSession)
	}
}

func TestOceanBaseIPv6AndTimezone(t *testing.T) {
	withPort, err := oceanbaseDriver{}.DSN(db.ParsedLink{
		User: "root", Pass: "secret", Protocol: "tcp", Host: "::1", Port: "3307", Name: "app",
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := mysqldriver.ParseDSN(withPort)
	if err != nil || cfg.Addr != "[::1]:3307" {
		t.Fatalf("addr %s err %v", cfg.Addr, err)
	}
	withoutPort, err := oceanbaseDriver{}.DSN(db.ParsedLink{
		User: "root", Pass: "secret", Protocol: "tcp", Host: "::1", Name: "app",
	})
	if err != nil || !strings.Contains(withoutPort, "tcp(::1)/") {
		t.Fatalf("dsn %s %v", withoutPort, err)
	}
	cfg, err = mysqldriver.ParseDSN(withoutPort)
	if err != nil {
		t.Fatal(err)
	}
	host, port, err := net.SplitHostPort(cfg.Addr)
	if err != nil || host != "::1" || port != "3306" {
		t.Fatalf("addr %s err %v", cfg.Addr, err)
	}
	session := oceanbaseDriver{}.ApplySession(withoutPort+"&loc=", db.Config{Timezone: "Asia/Shanghai"})
	if strings.Contains(session, "Asia") {
		t.Fatalf("empty loc overwritten: %s", session)
	}
}
