package dsl

import (
	"strings"
	"testing"

	"github.com/lanceadd/gooq"
)

func TestBuildErrReachesSelectEntries(t *testing.T) {
	bad := Select(testUser.Id).From(testUser).Limit(-1)
	inner := Select(testUser.Id).From(bad.As("inner")).Limit(-1)
	cases := []struct {
		name  string
		query interface{ ToSql(...gooq.Dialect) (string, []any, error) }
	}{
		{"derived", Select(testUser.Id).From(bad.As("t"))},
		{"in", Select(testUser.Id).From(testUser).Where(testUser.Id.InExpr(bad))},
		{"exists", Select(testUser.Id).From(testUser).Where(gooq.Exists(bad))},
		{"with", Select(testUser.Id).From(testUser).With("c", bad)},
		{"union", Select(testUser.Id).From(testUser).Union(bad)},
		{"insert", InsertFrom(testUser, bad)},
		{"nested", Select(testUser.Id).From(inner.As("outer"))},
	}
	for _, tc := range cases {
		sqlText, _, err := tc.query.ToSql(gooq.DialectMySQL)
		if err == nil || sqlText != "" || !strings.Contains(err.Error(), "LIMIT") {
			t.Fatalf("%s sql %q err %v", tc.name, sqlText, err)
		}
	}
}
