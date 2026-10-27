package db

import (
	"strings"
	"testing"
	"time"

	"github.com/lanceadd/gooq"
)

func TestFormatSQL(t *testing.T) {
	when := time.Date(2026, 10, 26, 14, 23, 9, 700277500, time.UTC)
	cases := []struct {
		name  string
		query string
		args  []any
		want  string
	}{
		{
			name:  "mysql positional",
			query: "SELECT id FROM user WHERE id = ? AND name = ?",
			args:  []any{int64(7), "admin"},
			want: "SELECT id FROM user WHERE id = ? AND name = ?\n" +
				"  args (2):\n" +
				"    1: 7\n" +
				"    2: \"admin\"",
		},
		{
			name:  "mysql literal question mark",
			query: "SELECT '?' AS q FROM user WHERE id = ?",
			args:  []any{int64(7)},
			want: "SELECT '?' AS q FROM user WHERE id = ?\n" +
				"  args (1):\n" +
				"    1: 7",
		},
		{
			name:  "pgsql numbered",
			query: `SELECT "id" FROM "user" WHERE "id" = $1 AND "created_at" = $2`,
			args:  []any{int64(7), when},
			want: `SELECT "id" FROM "user" WHERE "id" = $1 AND "created_at" = $2` + "\n" +
				"  args (2):\n" +
				"    1: 7\n" +
				"    2: 2026-10-26T14:23:09.7002775Z",
		},
		{
			name:  "pgsql json operator",
			query: `SELECT "id" FROM "doc" WHERE "data" ? 'key' AND "id" = $1`,
			args:  []any{int64(7)},
			want: `SELECT "id" FROM "doc" WHERE "data" ? 'key' AND "id" = $1` + "\n" +
				"  args (1):\n" +
				"    1: 7",
		},
		{
			name:  "null and bytes",
			query: "SELECT $1, $2",
			args:  []any{nil, []byte{0x01, 0x02}},
			want: "SELECT $1, $2\n" +
				"  args (2):\n" +
				"    1: NULL\n" +
				"    2: <2 bytes> 0x0102",
		},
		{
			name:  "no args",
			query: "SELECT 1",
			want:  "SELECT 1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatSQL(tc.query, tc.args); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatInlineSQL(t *testing.T) {
	when := time.Date(2026, 10, 26, 14, 23, 9, 700277500, time.UTC)
	mysql := gooq.LookupDialect(gooq.DialectMySQL)
	pgsql := gooq.LookupDialect(gooq.DialectPgsql)
	if mysql == nil || pgsql == nil {
		t.Fatal("builtin dialects are not registered")
	}
	cases := []struct {
		name  string
		spec  gooq.DialectSpec
		query string
		args  []any
		want  string
	}{
		{
			name:  "mysql positional",
			spec:  mysql,
			query: "SELECT id FROM user WHERE id = ? AND name = ?",
			args:  []any{int64(7), "ad'min"},
			want:  "SELECT id FROM user WHERE id = 7 AND name = 'ad''min'",
		},
		{
			name:  "mysql escapes backslash",
			spec:  mysql,
			query: "SELECT ?",
			args:  []any{`a\b`},
			want:  `SELECT 'a\\b'`,
		},
		{
			name:  "pgsql numbered",
			spec:  pgsql,
			query: `SELECT "id" FROM "user" WHERE "id" = $1 AND "created_at" = $2`,
			args:  []any{int64(7), when},
			want:  `SELECT "id" FROM "user" WHERE "id" = 7 AND "created_at" = '2026-10-26T14:23:09.7002775Z'`,
		},
		{
			name:  "pgsql repeated number",
			spec:  pgsql,
			query: `SELECT $1, $1, $2`,
			args:  []any{"x", int64(7)},
			want:  `SELECT 'x', 'x', 7`,
		},
		{
			name:  "pgsql json operator",
			spec:  pgsql,
			query: `SELECT "id" FROM "doc" WHERE "data" ? 'key' AND "id" = $1`,
			args:  []any{int64(7)},
			want:  `SELECT "id" FROM "doc" WHERE "data" ? 'key' AND "id" = 7`,
		},
		{
			name:  "pgsql missing arg keeps placeholder",
			spec:  pgsql,
			query: `SELECT $1, $2`,
			args:  []any{int64(7)},
			want:  `SELECT 7, $2`,
		},
		{
			name:  "null bytes bool",
			spec:  pgsql,
			query: `SELECT $1, $2, $3, $4`,
			args:  []any{nil, []byte{0x01, 0x02}, true, false},
			want:  `SELECT NULL, <2 bytes> 0x0102, TRUE, FALSE`,
		},
		{
			name:  "no args",
			spec:  pgsql,
			query: "SELECT 1",
			want:  "SELECT 1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := formatInlineSQL(tc.query, tc.args, tc.spec)
			if !ok {
				t.Fatal("formatInlineSQL reported unsupported placeholders")
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatArgTruncatesLongValues(t *testing.T) {
	limit := maxArgLength + 1
	value := strings.Repeat("a", limit)

	got := formatSQL("SELECT $1, $2, $3", []any{value, []byte(value), []string{value}})
	if !strings.Contains(got, `(truncated)`) {
		t.Fatalf("got %q, want truncated values", got)
	}
	if strings.Count(got, "(truncated)") != 3 {
		t.Fatalf("got %q, want 3 truncated values", got)
	}
}
