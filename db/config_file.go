package db

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/container/gtype"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/util/gconv"
)

var configLoaded = gtype.NewBool(false)

type configNode struct {
	Link             string
	Role             string
	Weight           int
	Debug            bool
	DryRun           bool
	Charset          string
	Timezone         string
	MaxOpen          int
	MaxIdle          int
	MaxLifetime      time.Duration
	MaxIdleTime      time.Duration
	MaxOpenConnCount int
	MaxIdleConnCount int
	MaxConnLifeTime  time.Duration
	MaxIdleConnTime  time.Duration
}

// LoadConfig reads the database node from a GoFrame config.
// With no config argument it uses gcfg.Instance(). Existing groups are left unchanged.
// The logger key under database is skipped.
func LoadConfig(ctx context.Context, source ...*gcfg.Config) error {
	if ctx == nil {
		ctx = context.Background()
	}
	config := gcfg.Instance()
	if len(source) > 0 && source[0] != nil {
		config = source[0]
	}
	value, err := config.Get(ctx, "database")
	if err != nil || value == nil || value.IsEmpty() {
		return nil
	}
	for group, raw := range value.Map() {
		if strings.EqualFold(group, "logger") {
			continue
		}
		nodes, err := configsFromRaw(raw)
		if err != nil {
			return err
		}
		if len(nodes) == 0 || groupConfigs.Contains(group) {
			continue
		}
		if err = SetConfig(group, nodes...); err != nil {
			return err
		}
	}
	return nil
}

func loadConfigOnce(ctx context.Context) error {
	if configLoaded.Val() {
		return nil
	}
	if err := LoadConfig(ctx); err != nil {
		return err
	}
	configLoaded.Set(true)
	return nil
}

func configsFromRaw(raw any) ([]Config, error) {
	if asMap, ok := raw.(map[string]any); ok && hasLink(asMap) {
		cfg, err := configFromMap(asMap)
		if err != nil {
			return nil, err
		}
		return []Config{cfg}, nil
	}
	items := gconv.Interfaces(raw)
	nodes := make([]Config, 0, len(items))
	for _, item := range items {
		asMap := gconv.Map(item)
		if !hasLink(asMap) {
			continue
		}
		cfg, err := configFromMap(asMap)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, cfg)
	}
	return nodes, nil
}

func hasLink(value map[string]any) bool {
	for key := range value {
		if strings.EqualFold(key, "link") {
			return true
		}
	}
	return false
}

func configFromMap(value map[string]any) (Config, error) {
	var node configNode
	if err := gconv.Scan(value, &node); err != nil {
		return Config{}, err
	}
	cfg := Config{
		Link:            node.Link,
		Role:            Role(node.Role),
		Weight:          node.Weight,
		Debug:           node.Debug,
		DryRun:          node.DryRun,
		Charset:         node.Charset,
		Timezone:        node.Timezone,
		MaxOpenConns:    node.MaxOpen,
		MaxIdleConns:    node.MaxIdle,
		ConnMaxLifetime: node.MaxLifetime,
		MaxIdleTime:     node.MaxIdleTime,
	}
	if cfg.MaxOpenConns == 0 {
		cfg.MaxOpenConns = node.MaxOpenConnCount
	}
	if cfg.MaxIdleConns == 0 {
		cfg.MaxIdleConns = node.MaxIdleConnCount
	}
	if cfg.ConnMaxLifetime == 0 {
		cfg.ConnMaxLifetime = node.MaxConnLifeTime
	}
	if cfg.MaxIdleTime == 0 {
		cfg.MaxIdleTime = node.MaxIdleConnTime
	}
	return cfg, nil
}
