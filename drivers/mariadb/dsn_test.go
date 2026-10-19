package mariadb

import (
	"net"
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"

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

func TestMariaDBIPv6AndTimezone(t *testing.T) {
	withPort, err := mariadbDriver{}.DSN(db.ParsedLink{
		User: "root", Pass: "secret", Protocol: "tcp", Host: "::1", Port: "3307", Name: "app",
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := mysqldriver.ParseDSN(withPort)
	if err != nil || cfg.Addr != "[::1]:3307" {
		t.Fatalf("addr %s err %v", cfg.Addr, err)
	}
	withoutPort, err := mariadbDriver{}.DSN(db.ParsedLink{
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
	session := mariadbDriver{}.ApplySession(withoutPort+"&loc=", db.Config{Timezone: "Asia/Shanghai"})
	if strings.Contains(session, "Asia") {
		t.Fatalf("empty loc overwritten: %s", session)
	}
}
