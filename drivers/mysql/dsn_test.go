package mysql

import (
	"net"
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"

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

func TestMySQLIPv6AndTimezone(t *testing.T) {
	withPort, err := mysqlDriver{}.DSN(db.ParsedLink{
		User: "root", Pass: "secret", Protocol: "tcp", Host: "::1", Port: "3307", Name: "app",
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := mysqldriver.ParseDSN(withPort)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "[::1]:3307" {
		t.Fatalf("addr %s dsn %s", cfg.Addr, withPort)
	}

	withoutPort, err := mysqlDriver{}.DSN(db.ParsedLink{
		User: "root", Pass: "secret", Protocol: "tcp", Host: "::1", Name: "app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(withoutPort, "tcp(::1)/") || strings.Contains(withoutPort, "[::1]:") {
		t.Fatalf("dsn %s", withoutPort)
	}
	cfg, err = mysqldriver.ParseDSN(withoutPort)
	if err != nil {
		t.Fatal(err)
	}
	host, port, err := net.SplitHostPort(cfg.Addr)
	if err != nil || host != "::1" || port != "3306" {
		t.Fatalf("addr %s err %v", cfg.Addr, err)
	}

	unixDSN, err := mysqlDriver{}.DSN(db.ParsedLink{
		User: "root", Pass: "secret", Protocol: "unix", Host: "/tmp/mysql.sock", Name: "app",
	})
	if err != nil || !strings.Contains(unixDSN, "unix(/tmp/mysql.sock)") {
		t.Fatalf("unix %s %v", unixDSN, err)
	}
	if _, err = (mysqlDriver{}).DSN(db.ParsedLink{Protocol: "unix", Host: "/tmp/mysql.sock", Port: "1", Name: "app"}); err == nil {
		t.Fatal("unix with port was accepted")
	}

	session := mysqlDriver{}.ApplySession(withoutPort+"&loc=", db.Config{Timezone: "Asia/Shanghai"})
	if strings.Contains(session, "Asia") {
		t.Fatalf("empty loc was overwritten: %s", session)
	}
	session = mysqlDriver{}.ApplySession(withoutPort, db.Config{Timezone: "Asia/Shanghai"})
	if !strings.Contains(session, "loc=Asia%2FShanghai") {
		t.Fatalf("timezone %s", session)
	}
}
