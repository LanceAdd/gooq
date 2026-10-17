package gen

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/test/gtest"
	"github.com/gogf/gf/v2/text/gstr"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

func TestGen_TenantSettings(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		absent, err := tenantSettingsFrom(TenantConfig{})
		t.AssertNil(err)
		t.Assert(absent.active, false)

		emptyColumn, err := tenantSettingsFrom(TenantConfig{Tables: map[string]any{"dict": ""}})
		t.AssertNil(err)
		t.Assert(emptyColumn.active, false)

		_, err = tenantSettingsFrom(TenantConfig{Column: "tenant_id", Tables: map[string]any{"dict": 1}})
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "must be a string"), true)

		present, err := tenantSettingsFrom(TenantConfig{Column: "tenant_id", Tables: map[string]any{"dict": ""}})
		t.AssertNil(err)
		t.Assert(present.active, true)
		t.Assert(present.column, "tenant_id")

		columns := []db.Column{
			{Name: "id", LocalType: gooq.LocalTypeInt64},
			{Name: "tenantId", LocalType: gooq.LocalTypeInt64},
			{Name: "name", LocalType: gooq.LocalTypeString},
		}
		got, err := tenantColumn("user", columns, present)
		t.AssertNil(err)
		t.Assert(got != nil && got.Name == "tenantId", true)

		disabled, err := tenantColumn("dict", columns, present)
		t.AssertNil(err)
		t.Assert(disabled == nil, true)

		_, err = tenantColumn("shop_order", columns, tenantSettings{active: true, column: "tenant_id", tables: map[string]string{"shop_order": "org_id"}})
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "org_id"), true)

		floatCol := []db.Column{{Name: "tenant_id", LocalType: gooq.LocalTypeFloat64}}
		_, err = tenantColumn("user", floatCol, tenantSettings{active: true, column: "tenant_id"})
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "unsupported type"), true)

		none, err := tenantColumn("user", columns, tenantSettings{})
		t.AssertNil(err)
		t.Assert(none == nil, true)
	})
}

func TestGen_TenantFieldGenerated(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		database, err := db.Open(db.Config{Link: "sqlite::@file(:memory:)"})
		t.AssertNil(err)
		defer func() { t.AssertNil(database.Close()) }()
		columns := []db.Column{
			{Name: "id", LocalType: gooq.LocalTypeInt64, Primary: true},
			{Name: "tenant_id", LocalType: gooq.LocalTypeInt64},
			{Name: "name", LocalType: gooq.LocalTypeString},
		}
		cfg, err := tenantSettingsFrom(TenantConfig{Column: "tenant_id"})
		t.AssertNil(err)

		content, err := generateTableContent(context.Background(), database, tableSpec{Physical: "user", Logical: "user"}, "table", columns, cfg)
		t.AssertNil(err)
		t.Assert(gstr.Contains(content, "Tenant: true"), true)

		cfg.tables = map[string]string{"user": ""}
		disabled, err := generateTableContent(context.Background(), database, tableSpec{Physical: "user", Logical: "user"}, "table", columns, cfg)
		t.AssertNil(err)
		t.Assert(gstr.Contains(disabled, "Tenant: true"), false)
	})
}

func TestGenerate_TenantIgnoresCwd(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		root := t.TempDir()
		dbPath := filepath.Join(root, "tenant.db")
		outDir := filepath.Join(root, "out")
		t.AssertNil(gfile.Mkdir(outDir))
		t.AssertNil(gfile.Mkdir(filepath.Join(root, "hack")))
		t.AssertNil(gfile.PutContents(filepath.Join(root, "hack", "gooq.yaml"), "gooq:\n  gen:\n    tenant:\n      column: tenant_id\n"))
		database, err := db.Open(db.Config{Link: "sqlite::@file(" + filepath.ToSlash(dbPath) + ")"})
		t.AssertNil(err)
		_, err = database.Exec(context.Background(), `CREATE TABLE user (id INTEGER PRIMARY KEY, tenant_id INTEGER NOT NULL, name TEXT)`)
		t.AssertNil(err)
		t.AssertNil(database.Close())

		t.Chdir(root)
		Generate(context.Background(), Input{Link: "sqlite::@file(" + filepath.ToSlash(dbPath) + ")", Path: outDir})
		plain := gfile.GetContents(filepath.Join(outDir, "table", "user.go"))
		t.Assert(gstr.Contains(plain, "Tenant: true"), false)

		Generate(context.Background(), Input{
			Link:   "sqlite::@file(" + filepath.ToSlash(dbPath) + ")",
			Path:   outDir,
			Tenant: TenantConfig{Column: "tenant_id"},
		})
		marked := gfile.GetContents(filepath.Join(outDir, "table", "user.go"))
		t.Assert(gstr.Contains(marked, "Tenant: true"), true)
	})
}
