package gen

import (
	"fmt"
	"strings"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

// TenantConfig 是 CLI 从 gooq.gen.tenant 读出后传入 Generate 的配置。
// Column 为空时不识别租户列。Tables 的空字符串表示这张表不标。
type TenantConfig struct {
	Column string
	Tables map[string]any
}

type tenantSettings struct {
	active bool
	column string
	tables map[string]string
}

func tenantSettingsFrom(cfg TenantConfig) (tenantSettings, error) {
	if cfg.Column == "" {
		return tenantSettings{}, nil
	}
	settings := tenantSettings{active: true, column: cfg.Column, tables: map[string]string{}}
	for name, value := range cfg.Tables {
		if value == nil {
			settings.tables[name] = ""
			continue
		}
		text, ok := value.(string)
		if !ok {
			return tenantSettings{}, fmt.Errorf("gooq: tenant column for table %q must be a string", name)
		}
		settings.tables[name] = text
	}
	return settings, nil
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
