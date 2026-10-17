package gen

import (
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/encoding/gyaml"
	"github.com/gogf/gf/v2/os/gfile"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

type tenantSettings struct {
	active bool
	column string
	tables map[string]string
}

func loadTenantSettings() (tenantSettings, error) {
	dir := hackDirPath()
	if dir == "" {
		return tenantSettings{}, nil
	}
	path := gfile.Join(dir, configFileName)
	if !gfile.Exists(path) {
		return tenantSettings{}, nil
	}
	return parseTenantSettings(gfile.GetBytes(path))
}

func parseTenantSettings(content []byte) (tenantSettings, error) {
	if len(strings.TrimSpace(string(content))) == 0 {
		return tenantSettings{}, nil
	}
	var doc map[string]any
	if err := gyaml.DecodeTo(content, &doc); err != nil {
		return tenantSettings{}, fmt.Errorf("gooq: parse gooq.yaml: %w", err)
	}
	genNode, ok := nestedMap(doc, "gooq", "gen")
	if !ok {
		return tenantSettings{}, nil
	}
	raw, exists := genNode["tenant"]
	if !exists || raw == nil {
		return tenantSettings{}, nil
	}
	node, ok := raw.(map[string]any)
	if !ok {
		return tenantSettings{}, fmt.Errorf("gooq: gooq.gen.tenant must be a map")
	}
	column := ""
	if rawColumn, ok := node["column"]; ok && rawColumn != nil {
		text, ok := rawColumn.(string)
		if !ok {
			return tenantSettings{}, fmt.Errorf("gooq: gooq.gen.tenant.column must be a string")
		}
		column = text
	}
	if column == "" {
		return tenantSettings{}, nil
	}
	cfg := tenantSettings{active: true, column: column, tables: map[string]string{}}
	rawTables, ok := node["tables"]
	if !ok || rawTables == nil {
		return cfg, nil
	}
	tables, ok := rawTables.(map[string]any)
	if !ok {
		return tenantSettings{}, fmt.Errorf("gooq: gooq.gen.tenant.tables must be a map")
	}
	for name, value := range tables {
		if value == nil {
			cfg.tables[name] = ""
			continue
		}
		text, ok := value.(string)
		if !ok {
			return tenantSettings{}, fmt.Errorf("gooq: tenant column for table %q must be a string", name)
		}
		cfg.tables[name] = text
	}
	return cfg, nil
}

func tenantColumn(logical string, columns []db.Column, cfg tenantSettings) (*db.Column, error) {
	if !cfg.active {
		return nil, nil
	}
	override, disabled, err := tenantOverride(logical, cfg.tables)
	if err != nil {
		return nil, err
	}
	if disabled {
		return nil, nil
	}
	name := cfg.column
	explicit := override != ""
	if explicit {
		name = override
	}
	matched := matchColumns(columns, name)
	if len(matched) > 1 {
		return nil, fmt.Errorf("gooq: table %q has more than one column matching tenant column %q", logical, name)
	}
	if len(matched) == 0 {
		if explicit {
			return nil, fmt.Errorf("gooq: table %q has no tenant column %q", logical, name)
		}
		return nil, nil
	}
	column := matched[0]
	if !gooq.TenantColumnAllowed(column.LocalType) {
		return nil, fmt.Errorf("gooq: table %q tenant column %q has unsupported type %q", logical, column.Name, column.LocalType)
	}
	return &column, nil
}

func tenantOverride(logical string, tables map[string]string) (column string, disabled bool, err error) {
	found := false
	for name, value := range tables {
		if !strings.EqualFold(name, logical) {
			continue
		}
		if found {
			return "", false, fmt.Errorf("gooq: tenant tables has duplicate entries for %q", logical)
		}
		found = true
		column = value
	}
	if !found {
		return "", false, nil
	}
	if column == "" {
		return "", true, nil
	}
	return column, false, nil
}

func matchColumns(columns []db.Column, name string) []db.Column {
	folded := foldColumn(name)
	var matched []db.Column
	for _, column := range columns {
		if foldColumn(column.Name) == folded {
			matched = append(matched, column)
		}
	}
	return matched
}

func foldColumn(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), "_", "")
}

func nestedMap(root map[string]any, keys ...string) (map[string]any, bool) {
	current := root
	for _, key := range keys {
		raw, ok := current[key]
		if !ok || raw == nil {
			return nil, false
		}
		next, ok := raw.(map[string]any)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}
