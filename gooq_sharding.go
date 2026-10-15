package gooq

import "fmt"

// ShardingRule 按分片值算出完整物理表名。
type ShardingRule interface {
	TableName(value any) (string, error)
}

// ResolveShardedName 返回渲染用的表名。表未分片（Sharding 为 nil）时是 logical 本身；
// 分片表缺分片值或实现出错记入 rc。同一个渲染上下文里每张表只解析一次，后续出现直接命中缓存。
func ResolveShardedName(rc *RenderContext, meta *TableMeta, logical string) string {
	rule := meta.Sharding
	if rule == nil {
		return logical
	}
	if name, ok := rc.shardNames[meta]; ok {
		return name
	}
	name := logical
	if value := rc.ShardingValue(); value == nil {
		rc.Fail(fmt.Errorf("gooq: table %q is sharded; ShardingValue is required", logical))
	} else if resolved, err := rule.TableName(value); err != nil {
		rc.Fail(err)
	} else if resolved == "" {
		rc.Fail(fmt.Errorf("gooq: sharding rule for table %q returned an empty name", logical))
	} else {
		name = resolved
	}
	rc.cacheShardName(meta, name)
	return name
}

func (rc *RenderContext) cacheShardName(meta *TableMeta, name string) {
	if rc.shardNames == nil {
		rc.shardNames = make(map[*TableMeta]string)
	}
	rc.shardNames[meta] = name
}
