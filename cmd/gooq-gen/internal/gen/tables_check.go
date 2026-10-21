package gen

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// TablesCheckConfig 是 CLI 从 gooq.tables.check 读出后传入生成和 tables 命令的配置。
type TablesCheckConfig struct {
	Ignore []string
}

// ParseTablesCheckConfig 严格校验 gooq.tables.check 节点。只接受 ignore。
func ParseTablesCheckConfig(raw any) (TablesCheckConfig, error) {
	if raw == nil {
		return TablesCheckConfig{}, nil
	}
	node, ok := raw.(map[string]any)
	if !ok {
		return TablesCheckConfig{}, fmt.Errorf("gooq: gooq.tables.check must be a map")
	}
	var unknown []string
	for key := range node {
		if key != "ignore" {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return TablesCheckConfig{}, fmt.Errorf("gooq: gooq.tables.check has unknown field %q", unknown[0])
	}
	raw, ok = node["ignore"]
	if !ok || raw == nil {
		return TablesCheckConfig{}, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return TablesCheckConfig{}, fmt.Errorf("gooq: gooq.tables.check.ignore must be a list")
	}
	var cfg TablesCheckConfig
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return TablesCheckConfig{}, fmt.Errorf("gooq: gooq.tables.check.ignore entries must be strings")
		}
		pattern := strings.TrimSpace(text)
		if pattern == "" {
			return TablesCheckConfig{}, fmt.Errorf("gooq: gooq.tables.check.ignore contains an empty pattern")
		}
		if _, err := path.Match(pattern, ""); err != nil {
			return TablesCheckConfig{}, fmt.Errorf("gooq: gooq.tables.check.ignore has invalid pattern %q", pattern)
		}
		cfg.Ignore = append(cfg.Ignore, pattern)
	}
	return cfg, nil
}

// Ignored 报告表名是否匹配 ignore 中的任一模式。
func (c TablesCheckConfig) Ignored(name string) bool {
	for _, pattern := range c.Ignore {
		if matched, _ := path.Match(pattern, name); matched {
			return true
		}
	}
	return false
}
