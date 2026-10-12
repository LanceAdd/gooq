package db

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/container/gmap"
	"github.com/gogf/gf/v2/container/gtype"
)

const (
	// DefaultGroupName 是 Instance 不传名字时使用的组。
	DefaultGroupName = "default"
)

var (
	defaultGroup   = gtype.NewString(DefaultGroupName)
	groupConfigs   = gmap.NewKVMap[string, []Config](true)
	groupInstances = gmap.NewKVMap[string, *DB](true)
)

// SetDefaultGroup 选择 Instance 默认使用的配置组。名字为空时改回 default。
func SetDefaultGroup(name string) {
	if name == "" {
		name = DefaultGroupName
	}
	defaultGroup.Set(name)
}

// SetConfig 为一组注册一个或多个节点。
// 组名为空时使用默认组。Role 为空的节点视为主库。打开该组时至少要有一个主库。
// 配置先保存，Instance 再打开。再次调用会关掉已经打开的实例。
func SetConfig(group string, nodes ...Config) error {
	if group == "" {
		group = DefaultGroupName
	}
	if len(nodes) == 0 {
		return fmt.Errorf("gooq: empty database configuration for group %q", group)
	}
	for _, cfg := range nodes {
		if _, err := parseLink(cfg.Link); err != nil {
			return err
		}
	}
	clearGroupLogger(group)
	groupConfigs.Set(group, append([]Config(nil), nodes...))
	if database := groupInstances.Remove(group); database != nil {
		return database.Close()
	}
	return nil
}

// RemoveConfig 去掉一组配置，并关闭已经打开的实例。
func RemoveConfig(group string) error {
	if group == "" {
		group = DefaultGroupName
	}
	clearGroupLogger(group)
	groupConfigs.Remove(group)
	if database := groupInstances.Remove(group); database != nil {
		return database.Close()
	}
	return nil
}

// Instance 返回配置组对应的共享数据库。
// 不传名字时返回默认组，作用和 gdb.Instance 相同。
func Instance(name ...string) (*DB, error) {
	group := defaultGroup.Val()
	if len(name) > 0 && name[0] != "" {
		group = name[0]
	}
	if database := groupInstances.Get(group); database != nil {
		return database, nil
	}
	configs, found := groupConfigs.Search(group)
	if !found || len(configs) == 0 {
		if err := loadConfigOnce(context.Background()); err != nil {
			return nil, err
		}
		configs, found = groupConfigs.Search(group)
	}
	if !found || len(configs) == 0 {
		return nil, fmt.Errorf("gooq: empty database configuration for group %q", group)
	}
	database, err := openGroup(group, configs)
	if err != nil {
		return nil, err
	}
	if err = applyGroupLogger(database); err != nil {
		if closeErr := database.Close(); closeErr != nil {
			return nil, fmt.Errorf("%w; close: %v", err, closeErr)
		}
		return nil, err
	}
	actual := groupInstances.GetOrSet(group, database)
	if actual != database {
		if closeErr := database.Close(); closeErr != nil {
			return nil, closeErr
		}
	}
	return actual, nil
}
