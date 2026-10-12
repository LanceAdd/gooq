package gooq

import (
	"strings"
	"testing"

	"github.com/gogf/gf/v2/test/gtest"
)

type bracketDialect struct {
	MySQLDialect
}

func (bracketDialect) Name() Dialect { return "bracket" }

func (bracketDialect) Quote(ident string) string { return "[" + ident + "]" }

func TestDialectSpec_BuiltinAndOverride(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		mysql, err := DialectSpecOf(DialectMySQL)
		t.AssertNil(err)
		t.Assert(mysql.Quote("user"), "`user`")
		t.Assert(mysql.Placeholder(1), "?")

		sqlite, err := DialectSpecOf(DialectSQLite)
		t.AssertNil(err)
		rc := NewRenderContext(DialectSQLite)
		sqlText, err := sqlite.RenderUpsert(rc, UpsertClause{
			ConflictCols: []string{"id"},
			Updates:      map[string]any{"name": "x"},
		})
		t.AssertNil(err)
		t.Assert(strings.Contains(sqlText, "ON CONFLICT"), true)
		t.Assert(strings.Contains(sqlText, "ON DUPLICATE KEY"), false)

		_, err = DialectSpecOf("missing")
		t.AssertNE(err, nil)

		RegisterDialect(bracketDialect{})
		spec, err := DialectSpecOf("bracket")
		t.AssertNil(err)
		t.Assert(spec.Quote("user"), "[user]")
		t.Assert(spec.InsertKeyword(true), "INSERT IGNORE")
	})
}

func TestRenderExcluded(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		cases := []struct {
			dialect Dialect
			want    string
		}{
			{DialectMySQL, "VALUES(`name`)"},
			{DialectPgsql, `EXCLUDED."name"`},
			{DialectSQLite, `EXCLUDED."name"`},
			{DialectDM, `T2."NAME"`},
		}
		for _, c := range cases {
			sql, args := Excluded(testUser.Name).(Renderer).Render(NewRenderContext(c.dialect))
			t.Assert(sql, c.want)
			t.Assert(len(args), 0)
		}
		sql, args := Excluded(testUser.Name).(Renderer).Render(NewRenderContext("missing"))
		t.Assert(sql, "")
		t.Assert(len(args), 0)
	})
}
