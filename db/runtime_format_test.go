package db

import (
	"strings"
	"testing"
	"time"
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
