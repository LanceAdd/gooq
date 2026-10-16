package gooq

import "fmt"

// ShardingTableMeta 是渲染某张分片表时交给规则的表信息。
// LogicalName 是逻辑表名，例如 user。ShardNames 是生成时代码里记下的物理分表名，供规则自己使用。
// 库不要求 TableName 的返回值落在这份清单里。
// 同一个 ShardingRule 实例可以注册到多张分片表。实现必须无状态，不要把表名存进规则自己。
type ShardingTableMeta struct {
	LogicalName string
	ShardNames  []string
}

// ShardingRule 按表信息和分片值算出物理表名。
// 返回值是单个未加引号的表名，例如 user_03。库负责加引号。
// schema 仍用 TableMeta 上原来的值，不要写进返回值。
// 同一个实例可以注册到多张分片表。实现必须无状态，不要把表名存进规则自己。
type ShardingRule interface {
	TableName(table ShardingTableMeta, value any) (string, error)
}

// ResolveTableName 返回渲染用的表名。表未分片时是 logical 本身。
// 表对象自己调用过 ShardBy 时用那个值；否则只有当前主表能用查询上的 ShardBy。
// 同一个表对象在本次查询的缓存里只解析一次。
func ResolveTableName(rc *RenderContext, table Table, logical string) string {
	if table == nil {
		return logical
	}
	meta := table.Meta()
	if meta == nil || meta.Sharding == nil {
		return logical
	}
	base := TableBaseOf(table)
	if base != nil {
		if name, ok := rc.shardNames[base]; ok {
			return name
		}
	}
	name := logical
	if value, ok := shardValueFor(rc, base); !ok {
		if base != nil && rc.mainTable == base {
			rc.Fail(fmt.Errorf("gooq: table %q is sharded; ShardBy is required", logical))
		} else {
			rc.Fail(fmt.Errorf("gooq: table %q is sharded; call ShardBy or Physical", logical))
		}
	} else if resolved, err := meta.Sharding.TableName(ShardingTableMeta{LogicalName: meta.TableName, ShardNames: meta.ShardNames}, value); err != nil {
		rc.Fail(err)
	} else if resolved == "" {
		rc.Fail(fmt.Errorf("gooq: sharding rule for table %q returned an empty name", logical))
	} else {
		name = resolved
	}
	if base != nil {
		rc.cacheShardName(base, name)
	}
	return name
}

func shardValueFor(rc *RenderContext, base *TableBase) (any, bool) {
	if base != nil && base.hasShardValue {
		return base.shardValue, true
	}
	if base != nil && rc.mainTable == base && rc.queryHasShard {
		return rc.queryShard, true
	}
	return nil, false
}

func (rc *RenderContext) cacheShardName(base *TableBase, name string) {
	if rc.shardNames == nil {
		rc.shardNames = make(map[*TableBase]string)
	}
	rc.shardNames[base] = name
}
