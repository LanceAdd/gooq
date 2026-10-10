package db

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/container/gmap"
	"github.com/gogf/gf/v2/container/gtype"
)

const (
	// DefaultGroupName is the group used when Instance is called without a name.
	DefaultGroupName = "default"
)

var (
	defaultGroup   = gtype.NewString(DefaultGroupName)
	groupConfigs   = gmap.NewKVMap[string, []Config](true)
	groupInstances = gmap.NewKVMap[string, *DB](true)
)

// SetDefaultGroup selects which configuration group Instance uses by default.
func SetDefaultGroup(name string) {
	if name == "" {
		name = DefaultGroupName
	}
	defaultGroup.Set(name)
}

// SetConfig registers one or more nodes for a group.
// An empty group name means the default group. The first master is required.
// A node with an empty Role is a master. A later Instance call opens the group once.
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
	groupConfigs.Set(group, append([]Config(nil), nodes...))
	if database := groupInstances.Remove(group); database != nil {
		return database.Close()
	}
	return nil
}

// RemoveConfig drops a group and closes its open instance.
func RemoveConfig(group string) error {
	if group == "" {
		group = DefaultGroupName
	}
	groupConfigs.Remove(group)
	if database := groupInstances.Remove(group); database != nil {
		return database.Close()
	}
	return nil
}

// Instance returns the shared database for a configuration group.
// With no name, it returns the default group, the same role as gdb.Instance.
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
	actual := groupInstances.GetOrSet(group, database)
	if actual != database {
		if closeErr := database.Close(); closeErr != nil {
			return nil, closeErr
		}
	}
	return actual, nil
}
