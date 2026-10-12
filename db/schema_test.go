package db

import (
	"testing"

	"github.com/lanceadd/gooq"
)

func TestLocalTypeOf(t *testing.T) {
	cases := []struct {
		dbType string
		want   gooq.LocalType
	}{
		{"_int2", gooq.LocalTypeIntSlice},
		{"_int4", gooq.LocalTypeIntSlice},
		{"_int", gooq.LocalTypeIntSlice},
		{"_int8", gooq.LocalTypeInt64Slice},
		{"_text", gooq.LocalTypeStringSlice},
		{"_varchar", gooq.LocalTypeStringSlice},
		{"_bpchar", gooq.LocalTypeStringSlice},
		{"_bool", gooq.LocalTypeBoolSlice},
		{"_float4", gooq.LocalTypeFloat32Slice},
		{"_float8", gooq.LocalTypeFloat64Slice},
		{"_bytea", gooq.LocalTypeBytesSlice},
		{"_uuid", gooq.LocalTypeUUIDSlice},
		{"numeric", gooq.LocalTypeDecimal},
		{"decimal", gooq.LocalTypeDecimal},
		{"numeric(20,2)", gooq.LocalTypeDecimal},
		{"float4", gooq.LocalTypeFloat64},
		{"float8", gooq.LocalTypeFloat64},
		{"real", gooq.LocalTypeFloat64},
		{"double", gooq.LocalTypeFloat64},
		{"interval", gooq.LocalTypeString},
		{"inet", gooq.LocalTypeString},
		{"cidr", gooq.LocalTypeString},
		{"macaddr", gooq.LocalTypeString},
		{"int4range", gooq.LocalTypeString},
		{"_json", gooq.LocalTypeString},
		{"int unsigned", gooq.LocalTypeInt},
		{"bigint unsigned", gooq.LocalTypeInt64},
		{"int(10) unsigned", gooq.LocalTypeInt},
		{"int unsigned zerofill", gooq.LocalTypeInt},
		{"bigint", gooq.LocalTypeInt64},
	}
	for _, c := range cases {
		if got := LocalTypeOf(c.dbType); got != c.want {
			t.Errorf("LocalTypeOf(%q) = %q, want %q", c.dbType, got, c.want)
		}
	}
}
