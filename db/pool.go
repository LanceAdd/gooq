package db

import (
	"context"
	"database/sql"
)

// PoolStats is one pool's database/sql statistics and its role.
type PoolStats struct {
	Role  Role
	Stats sql.DBStats
}

// Ping checks every master and slave pool.
func (d *DB) Ping(ctx context.Context) error {
	if err := d.PingMaster(ctx); err != nil {
		return err
	}
	return d.PingSlave(ctx)
}

// PingMaster checks every master pool.
func (d *DB) PingMaster(ctx context.Context) error {
	return pingNodes(ctx, d.masters)
}

// PingSlave checks every slave pool. It returns nil when no slave is configured.
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

// Stats returns statistics for each master and slave pool.
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
