package pgsql

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/lanceadd/gooq/db"
)

func TestDecodeArrays(t *testing.T) {
	driver := postgresDriver{}
	cases := []struct {
		dbType string
		text   string
		want   any
	}{
		{"_INT4", "{1,2}", []int{1, 2}},
		{"_INT8", "{3,4}", []int64{3, 4}},
		{"_TEXT", "{a,b}", []string{"a", "b"}},
		{"_TEXT", `{"a'b","c"}`, []string{"a'b", "c"}},
		{"_BOOL", "{t,f}", []bool{true, false}},
		{"_FLOAT4", "{1.5,2}", []float32{1.5, 2}},
		{"_FLOAT8", "{1.5,2.5}", []float64{1.5, 2.5}},
		{"_BYTEA", `{"\\x6869"}`, [][]byte{{0x68, 0x69}}},
		{"_UUID", "{6ba7b810-9dad-11d1-80b4-00c04fd430c8}", []uuid.UUID{uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")}},
		{"_INT4", "{}", []int{}},
	}
	for _, c := range cases {
		got := driver.Decode(db.ColumnValue{DBType: c.dbType, Value: []byte(c.text)})
		if !reflect.DeepEqual(got, c.want) {
			t.Fatalf("Decode %s %s = %#v, want %#v", c.dbType, c.text, got, c.want)
		}
	}
	kept := driver.Decode(db.ColumnValue{DBType: "TEXT", Value: []byte("plain")})
	if _, ok := kept.(db.ColumnValue); !ok {
		t.Fatalf("non-array decode = %#v", kept)
	}
}

func TestBindArrays(t *testing.T) {
	driver := postgresDriver{}
	if _, ok := driver.Bind([]int{1}).(*pq.Int64Array); !ok {
		t.Fatal("[]int was not bound as int64 array")
	}
	if _, ok := driver.Bind([]string{"a"}).(*pq.StringArray); !ok {
		t.Fatal("[]string was not bound as string array")
	}
	bound := driver.Bind([]uuid.UUID{uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")})
	generic, ok := bound.(pq.GenericArray)
	if !ok {
		t.Fatal("[]uuid.UUID was not bound as GenericArray")
	}
	if _, ok = generic.A.([]uuid.UUID); !ok {
		t.Fatalf("generic payload %#v", generic.A)
	}
	raw := []byte{1, 2}
	if !bytes.Equal(driver.Bind(raw).([]byte), raw) {
		t.Fatal("[]byte should stay bytea")
	}
}
