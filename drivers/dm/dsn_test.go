package dm

import (
	"strings"
	"testing"
	"time"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

func TestDMDSN(t *testing.T) {
	dsn, err := dmDriver{}.DSN(db.ParsedLink{
		User: "user",
		Pass: "pass",
		Host: "localhost",
		Port: "5236",
		Name: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dsn != "dm://user:pass@localhost:5236/test?schema=test" {
		t.Fatalf("dsn = %s", dsn)
	}
	withExtra, err := dmDriver{}.DSN(db.ParsedLink{
		User:  "user",
		Pass:  "pass",
		Host:  "localhost",
		Name:  "test",
		Extra: "compress=1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if withExtra != "dm://user:pass@localhost:5236/test?schema=test&compress=1" {
		t.Fatalf("extra dsn = %s", withExtra)
	}
	if _, err = (dmDriver{}).DSN(db.ParsedLink{User: "user", Host: "localhost"}); err == nil {
		t.Fatal("missing database name")
	}
	if _, err = (dmDriver{}).DSN(db.ParsedLink{User: "user", Name: "test"}); err == nil {
		t.Fatal("missing host")
	}
}

func TestDMConfigureSession(t *testing.T) {
	dsn, err := dmDriver{}.ConfigureSession(
		"dm://user:pass@localhost:5236/test?schema=test",
		db.Config{Charset: "utf8", Timezone: "Asia/Shanghai"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if dsn != "dm://user:pass@localhost:5236/test?schema=test&charset=utf8&timeZone=480" {
		t.Fatalf("session dsn = %s", dsn)
	}
	base := "dm://user:pass@localhost:5236/test?schema=test"
	if _, err = (dmDriver{}).ConfigureSession(base, db.Config{Timezone: "Invalid/Timezone"}); err == nil {
		t.Fatal("invalid timezone")
	}
	if _, err = (dmDriver{}).ConfigureSession(base, db.Config{Timezone: "Pacific/Kiritimati"}); err == nil || !strings.Contains(err.Error(), "minutes") {
		t.Fatalf("offset error = %v", err)
	}
}

func TestDMTimezoneOffset(t *testing.T) {
	cases := []struct {
		timezone string
		now      time.Time
		want     int
	}{
		{"Asia/Shanghai", time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), 480},
		{"America/New_York", time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), -300},
		{"America/New_York", time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC), -240},
		{"Etc/GMT-12", time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), 720},
		{"Etc/GMT+12", time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), -720},
	}
	for _, c := range cases {
		got, err := dmTimezoneOffset(c.timezone, c.now)
		if err != nil {
			t.Fatalf("%s: %v", c.timezone, err)
		}
		if got != c.want {
			t.Fatalf("%s offset = %d, want %d", c.timezone, got, c.want)
		}
	}
}

func TestDMBindTime(t *testing.T) {
	stamp := time.Date(2026, time.March, 2, 3, 4, 5, 0, time.UTC)
	if got := (dmDriver{}).Bind(stamp); got != "2026-03-02 03:04:05" {
		t.Fatalf("bind = %#v", got)
	}
	if got := (dmDriver{}).Bind(time.Time{}); got != nil {
		t.Fatalf("zero = %#v", got)
	}
	if got := (dmDriver{}).Bind((*time.Time)(nil)); got != nil {
		t.Fatalf("nil = %#v", got)
	}
	if got := (dmDriver{}).Bind("keep"); got != "keep" {
		t.Fatalf("string = %#v", got)
	}
}

func TestDMSchemaSQL(t *testing.T) {
	sqlText, err := dmDriver{}.SchemaSQL("app")
	if err != nil {
		t.Fatal(err)
	}
	if sqlText != `SET SCHEMA "APP"` {
		t.Fatalf("schema sql = %s", sqlText)
	}
}

func TestTableNameCandidates(t *testing.T) {
	if got := strings.Join(tableNameCandidates("A_TABLES"), ","); got != "A_TABLES" {
		t.Fatalf("upper = %s", got)
	}
	if got := strings.Join(tableNameCandidates(`"A_tables"`), ","); got != "A_TABLES,A_tables" {
		t.Fatalf("mixed = %s", got)
	}
}

func TestLocalTypeOf(t *testing.T) {
	cases := []struct {
		dbType string
		want   gooq.LocalType
	}{
		{"VARCHAR(128)", gooq.LocalTypeString},
		{"NUMBER(20,2)", gooq.LocalTypeDecimal},
		{"BIT", gooq.LocalTypeBool},
		{"TIMESTAMP(6)", gooq.LocalTypeDatetime},
		{"TIMESTAMP WITH TIME ZONE", gooq.LocalTypeDatetime},
		{"INTERVAL DAY TO SECOND(2)", gooq.LocalTypeString},
		{"DOUBLE PRECISION", gooq.LocalTypeFloat64},
		{"BLOB", gooq.LocalTypeBytes},
		{"BIGINT(8)", gooq.LocalTypeInt64},
	}
	for _, c := range cases {
		if got := localTypeOf(c.dbType); got != c.want {
			t.Errorf("localTypeOf(%q) = %q, want %q", c.dbType, got, c.want)
		}
	}
}

func TestColumnType(t *testing.T) {
	if got := columnType("VARCHAR", "128"); got != "VARCHAR(128)" {
		t.Fatalf("type = %s", got)
	}
	if got := columnType("CLOB", ""); got != "CLOB" {
		t.Fatalf("type = %s", got)
	}
}
