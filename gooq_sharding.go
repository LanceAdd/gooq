package gooq

import "fmt"

// ShardingRule 按分片值算出完整物理表名。
type ShardingRule interface {
	TableName(value any) (string, error)
}

// ShardingSlot 是生成物里挂到 TableMeta 上的分片槽。未注册（rule 为空）时视为普通表。
type ShardingSlot struct {
	rule ShardingRule
}

func NewShardingSlot() *ShardingSlot {
	return &ShardingSlot{}
}

// Inject 注册分片实现。rule 为 nil 时 panic；重复注册覆盖。
func (s *ShardingSlot) Inject(rule ShardingRule) {
	if rule == nil {
		panic("gooq: sharding slot requires a rule")
	}
	s.rule = rule
}

// Resolve 返回渲染用的表名。未注册时是 logical 本身；注册后缺分片值或实现出错记入 rc。
// 同一个渲染上下文里每张表只解析一次，后续出现直接命中缓存。
func (s *ShardingSlot) Resolve(rc *RenderContext, logical string) string {
	if s.rule == nil {
		return logical
	}
	if name, ok := rc.shardNames[s]; ok {
		return name
	}
	name := logical
	if value := rc.ShardingValue(); value == nil {
		rc.Fail(fmt.Errorf("gooq: table %q is sharded; ShardingValue is required", logical))
	} else if resolved, err := s.rule.TableName(value); err != nil {
		rc.Fail(err)
	} else if resolved == "" {
		rc.Fail(fmt.Errorf("gooq: sharding rule for table %q returned an empty name", logical))
	} else {
		name = resolved
	}
	rc.cacheShardName(s, name)
	return name
}

func (rc *RenderContext) cacheShardName(slot *ShardingSlot, name string) {
	if rc.shardNames == nil {
		rc.shardNames = make(map[*ShardingSlot]string)
	}
	rc.shardNames[slot] = name
}
