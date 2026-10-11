package gooq

import (
	"database/sql"
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gtime"
)

func TestAssignCell(t *testing.T) {
	created := NewField[time.Time]("item", "created_at")
	name := NewField[string]("item", "name")
	row := Record{
		"created_at": "2026-10-10 08:30:00",
		"name":       "john",
	}

	got, err := Value(row, created)
	if err != nil {
		t.Fatal(err)
	}
	if got.Format("2006-01-02 15:04:05") != "2026-10-10 08:30:00" {
		t.Fatalf("time %s", got)
	}
	if Get(row, created).Format("2006-01-02 15:04:05") != "2026-10-10 08:30:00" {
		t.Fatal("Get lost the time")
	}

	text, err := Value(row, name)
	if err != nil || text != "john" {
		t.Fatalf("name %q err=%v", text, err)
	}

	var when time.Time
	if err = Assign(&when, "not-a-time"); err == nil || !when.IsZero() {
		t.Fatalf("bad time err=%v value=%v", err, when)
	}
	if !Get(Record{"created_at": "not-a-time"}, created).IsZero() {
		t.Fatal("Get returned a parsed invalid time")
	}

	var gt gtime.Time
	if err = Assign(&gt, "2026-10-10 08:30:00"); err != nil {
		t.Fatal(err)
	}
	if gt.Format("Y-m-d H:i:s") != "2026-10-10 08:30:00" {
		t.Fatalf("gtime %s", gt.String())
	}

	var ns sql.NullString
	if err = Assign(&ns, "john"); err != nil || !ns.Valid || ns.String != "john" {
		t.Fatalf("null string %+v err=%v", ns, err)
	}
	ns = sql.NullString{String: "keep", Valid: true}
	if err = Assign(&ns, nil); err != nil || ns.Valid {
		t.Fatalf("null string from NULL %+v err=%v", ns, err)
	}

	var nt sql.NullTime
	if err = Assign(&nt, "2026-10-10 08:30:00"); err != nil || !nt.Valid {
		t.Fatalf("null time %+v err=%v", nt, err)
	}
	if nt.Time.Format("2006-01-02 15:04:05") != "2026-10-10 08:30:00" {
		t.Fatalf("null time value %s", nt.Time)
	}

	var n int
	if err = Assign(&n, int64(7)); err != nil || n != 7 {
		t.Fatalf("int %d err=%v", n, err)
	}
	var textPtr *string
	if err = Assign(&textPtr, nil); err != nil || textPtr != nil {
		t.Fatalf("nil string pointer %v err=%v", textPtr, err)
	}

	column, err := Column(Result{row, {"created_at": nil}}, created)
	if err != nil || len(column) != 2 || column[0].IsZero() || !column[1].IsZero() {
		t.Fatalf("column %#v err=%v", column, err)
	}

	if cell := Cell(row, "name"); cell.String() != "john" || cell.IsNil() {
		t.Fatalf("cell %v", cell)
	}
	if !Cell(Record{"name": nil}, "name").IsNil() || !Cell(nil, "name").IsNil() {
		t.Fatal("NULL cell was not nil")
	}
	if Cell(row, "created_at").Time().Format("2006-01-02 15:04:05") != "2026-10-10 08:30:00" {
		t.Fatal("cell time")
	}

	var user struct{ Name string }
	if err = CheckScan(&user); err != nil {
		t.Fatal(err)
	}
	if err = CheckScan(&when); err == nil {
		t.Fatal("Scan accepted time.Time")
	}
	if err = CheckValue(&user); err == nil {
		t.Fatal("Value accepted a row struct")
	}
	if err = CheckValue(&when); err != nil {
		t.Fatal(err)
	}
	var times []time.Time
	if err = CheckScan(&times); err == nil {
		t.Fatal("Scan accepted []time.Time")
	}
	if err = CheckColumn(&times); err != nil {
		t.Fatal(err)
	}
	var users []struct{ Name string }
	if err = CheckColumn(&users); err == nil {
		t.Fatal("Column accepted a row slice")
	}
}
