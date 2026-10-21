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

		t.Assert(mysql.RenderExists(nil, "SELECT 1"), "SELECT EXISTS (SELECT 1)")
		dm, err := DialectSpecOf(DialectDM)
		t.AssertNil(err)
		t.Assert(dm.RenderExists(nil, "SELECT 1"), "SELECT COUNT(*) FROM (SELECT 1) WHERE ROWNUM = 1")

		RegisterDialect(bracketDialect{})
		spec, err := DialectSpecOf("bracket")
		t.AssertNil(err)
		t.Assert(spec.Quote("user"), "[user]")
		t.Assert(spec.InsertKeyword(true), "INSERT IGNORE")
	})
}

func TestRenderCreateTableLike(t *testing.T) {
	cases := []struct {
		dialect Dialect
		want    string
	}{
		{DialectMySQL, "CREATE TABLE IF NOT EXISTS `employee_3` LIKE `employee_1`"},
		{DialectMariaDB, "CREATE TABLE IF NOT EXISTS `employee_3` LIKE `employee_1`"},
		{DialectTiDB, "CREATE TABLE IF NOT EXISTS `employee_3` LIKE `employee_1`"},
		{DialectOceanBase, "CREATE TABLE IF NOT EXISTS `employee_3` LIKE `employee_1`"},
		{DialectPgsql, `CREATE TABLE IF NOT EXISTS "employee_3" (LIKE "employee_1" INCLUDING ALL)`},
		{DialectGaussDB, `CREATE TABLE IF NOT EXISTS "employee_3" (LIKE "employee_1" INCLUDING ALL)`},
	}
	for _, c := range cases {
		spec, err := DialectSpecOf(c.dialect)
		if err != nil {
			t.Fatal(err)
		}
		renderer, ok := spec.(TableLikeRenderer)
		if !ok {
			t.Fatalf("%s does not implement TableLikeRenderer", c.dialect)
		}
		got, err := renderer.RenderCreateTableLike(NewRenderContext(c.dialect), "employee_1", "employee_3")
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Fatalf("%s: %s", c.dialect, got)
		}
	}
	for _, d := range []Dialect{DialectSQLite, DialectDM} {
		spec, err := DialectSpecOf(d)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := spec.(TableLikeRenderer); ok {
			t.Fatalf("%s should not support CREATE TABLE LIKE", d)
		}
	}
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
			rc := NewRenderContext(c.dialect)
			sql := Excluded(testUser.Name).Render(rc)
			t.Assert(sql, c.want)
			t.Assert(len(rc.Args()), 0)
		}
		rc := NewRenderContext("missing")
		sql := Excluded(testUser.Name).Render(rc)
		t.Assert(sql, "")
		t.Assert(len(rc.Args()), 0)
	})
}
