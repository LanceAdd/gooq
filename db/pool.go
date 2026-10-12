package db

import (
	"context"
	"database/sql"
)

// PoolStats 是一个连接池的 database/sql 统计和它的角色。
type PoolStats struct {
	Role  Role
	Stats sql.DBStats
}

// Ping 检查全部主库和从库连接池。
func (d *DB) Ping(ctx context.Context) error {
	if err := d.PingMaster(ctx); err != nil {
		return err
	}
	return d.PingSlave(ctx)
}

// PingMaster 检查全部主库连接池。
func (d *DB) PingMaster(ctx context.Context) error {
	return pingNodes(ctx, d.masters)
}

// PingSlave 检查全部从库连接池。没有从库时返回 nil。
func (d *DB) PingSlave(ctx context.Context) error {
	return pingNodes(ctx, d.slaves)
}

func pingNodes(ctx context.Context, nodes []node) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for _, item := range nodes {
		if err := item.sql.PingContext(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Stats 返回每个主库和从库连接池的统计。
func (d *DB) Stats() []PoolStats {
	out := make([]PoolStats, 0, len(d.masters)+len(d.slaves))
	for _, item := range d.masters {
		out = append(out, PoolStats{Role: RoleMaster, Stats: item.sql.Stats()})
	}
	for _, item := range d.slaves {
		out = append(out, PoolStats{Role: RoleSlave, Stats: item.sql.Stats()})
	}
	return out
}
