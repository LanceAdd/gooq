package gen

import (
	"context"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/test/gtest"
	"github.com/gogf/gf/v2/text/gstr"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

func TestGen_TenantSettings(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		absent, err := parseTenantSettings([]byte("gooq:\n  gen:\n    table:\n      path: internal\n"))
		t.AssertNil(err)
		t.Assert(absent.active, false)

		empty, err := parseTenantSettings([]byte("gooq:\n  gen:\n    tenant:\n"))
		t.AssertNil(err)
		t.Assert(empty.active, false)

		tablesOnly, err := parseTenantSettings([]byte("gooq:\n  gen:\n    tenant:\n      tables:\n        dict: \"\"\n"))
		t.AssertNil(err)
		t.Assert(tablesOnly.active, false)

		emptyColumn, err := parseTenantSettings([]byte("gooq:\n  gen:\n    tenant:\n      column: \"\"\n      tables:\n        dict: \"\"\n"))
		t.AssertNil(err)
		t.Assert(emptyColumn.active, false)

		present, err := parseTenantSettings([]byte("gooq:\n  gen:\n    tenant:\n      column: tenant_id\n      tables:\n        dict: \"\"\n"))
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

		_, err = tenantColumn("shop_order", columns, tenantSettings{active: true, tables: map[string]string{"shop_order": "org_id"}})
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
		cfg := tenantSettings{active: true, column: "tenant_id"}

		content, err := generateTableContent(context.Background(), database, tableSpec{Physical: "user", Logical: "user"}, "table", columns, cfg)
		t.AssertNil(err)
		t.Assert(gstr.Contains(content, "Tenant: true"), true)

		cfg.tables = map[string]string{"user": ""}
		disabled, err := generateTableContent(context.Background(), database, tableSpec{Physical: "user", Logical: "user"}, "table", columns, cfg)
		t.AssertNil(err)
		t.Assert(gstr.Contains(disabled, "Tenant: true"), false)
	})
}
