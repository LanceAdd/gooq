package db

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/container/gmap"
	"github.com/gogf/gf/v2/container/gtype"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/glog"
	"github.com/gogf/gf/v2/util/gconv"
)

var (
	configLoaded       = gtype.NewBool(false)
	groupLoggerConfigs = gmap.NewKVMap[string, map[string]any](true)
)

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
// database.logger configures the SQL logger for every group loaded from this file.
// debug on a node turns SQL logging on for that group.
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
	rawGroups := value.Map()
	loggerCfg := loggerConfig(rawGroups)
	if len(loggerCfg) > 0 {
		if err = glog.New().SetConfigWithMap(loggerCfg); err != nil {
			return err
		}
	}
	for group, raw := range rawGroups {
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
		if len(loggerCfg) > 0 {
			groupLoggerConfigs.Set(group, loggerCfg)
		}
	}
	return nil
}

func loggerConfig(groups map[string]any) map[string]any {
	for group, raw := range groups {
		if strings.EqualFold(group, "logger") {
			return gconv.Map(raw)
		}
	}
	return nil
}

func clearGroupLogger(group string) {
	groupLoggerConfigs.Remove(group)
}

func applyGroupLogger(database *DB) error {
	if database == nil || database.logger == nil {
		return nil
	}
	cfg := groupLoggerConfigs.Get(database.group)
	if len(cfg) == 0 {
		return nil
	}
	return database.logger.SetConfigWithMap(cfg)
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
