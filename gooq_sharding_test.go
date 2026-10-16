package gooq

import (
	"fmt"
	"strings"
	"testing"
)

type testShardingRule struct {
	name    string
	err     error
	got     any
	logical string
	shards  []string
}

func (r *testShardingRule) TableName(table ShardingTableMeta, value any) (string, error) {
	r.logical = table.LogicalName
	r.shards = table.ShardNames
	r.got = value
	return r.name, r.err
}

type countingShardingRule struct {
	calls *int
}

func (r countingShardingRule) TableName(table ShardingTableMeta, value any) (string, error) {
	*r.calls++
	return fmt.Sprintf("user_%02d", value.(int64)%16), nil
}

func shardedTestTable(rule ShardingRule) *TableBase {
	return NewTableBase(&TableMeta{TableName: "user", Sharding: rule})
}

func TestResolveTableNameUnregistered(t *testing.T) {
	table := NewTableBase(&TableMeta{TableName: "user"})
	if got := ResolveTableName(NewRenderContext(DialectMySQL), table, "user"); got != "user" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveTableName(t *testing.T) {
	rule := &testShardingRule{name: "user_03"}
	table := shardedTestTable(rule)
	rc := NewRenderContext(DialectMySQL)
	rc.SetMainTable(table)
	if got := ResolveTableName(rc, table, "user"); got != "user" || rc.Err() == nil || !strings.Contains(rc.Err().Error(), "ShardBy is required") {
		t.Fatalf("missing value got %q err %v", got, rc.Err())
	}

	rc = NewRenderContext(DialectMySQL)
	rc.SwapQueryShard(true, int64(19))
	rc.SetMainTable(table)
	if got := ResolveTableName(rc, table, "user"); got != "user_03" || rc.Err() != nil {
		t.Fatalf("resolve got %q err %v", got, rc.Err())
	}
	if rule.logical != "user" {
		t.Fatalf("logical %q", rule.logical)
	}

	boom := fmt.Errorf("rule boom")
	brokenRule := &testShardingRule{err: boom}
	broken := shardedTestTable(brokenRule)
	rc = NewRenderContext(DialectMySQL)
	rc.SwapQueryShard(true, int64(1))
	rc.SetMainTable(broken)
	if got := ResolveTableName(rc, broken, "user"); got != "user" || rc.Err() != boom {
		t.Fatalf("rule error got %q err %v", got, rc.Err())
	}

	emptyRule := &testShardingRule{}
	empty := shardedTestTable(emptyRule)
	rc = NewRenderContext(DialectMySQL)
	rc.SwapQueryShard(true, int64(1))
	rc.SetMainTable(empty)
	if got := ResolveTableName(rc, empty, "user"); got != "user" || rc.Err() == nil || !strings.Contains(rc.Err().Error(), "empty name") {
		t.Fatalf("empty name got %q err %v", got, rc.Err())
	}
}

func TestResolveTableNameOncePerObject(t *testing.T) {
	calls := 0
	table := shardedTestTable(countingShardingRule{calls: &calls})
	rc := NewRenderContext(DialectMySQL)
	rc.SwapQueryShard(true, int64(19))
	rc.SetMainTable(table)
	for i := 0; i < 3; i++ {
		if got := ResolveTableName(rc, table, "user"); got != "user_03" {
			t.Fatalf("got %q", got)
		}
	}
	if calls != 1 {
		t.Fatalf("rule called %d times", calls)
	}

	missing := shardedTestTable(countingShardingRule{calls: &calls})
	rc = NewRenderContext(DialectMySQL)
	rc.SetMainTable(missing)
	for i := 0; i < 2; i++ {
		if got := ResolveTableName(rc, missing, "user"); got != "user" {
			t.Fatalf("missing value got %q", got)
		}
	}
	if rc.Err() == nil || calls != 1 {
		t.Fatalf("err %v calls %d", rc.Err(), calls)
	}
}

func TestResolveTableNameNilShardIsSet(t *testing.T) {
	rule := &testShardingRule{name: "user_nil"}
	table := shardedTestTable(rule)
	table.SetShard(nil)
	rc := NewRenderContext(DialectMySQL)
	if got := ResolveTableName(rc, table, "user"); got != "user_nil" || rc.Err() != nil {
		t.Fatalf("got %q err %v", got, rc.Err())
	}
	if rule.got != nil {
		t.Fatalf("rule got %v", rule.got)
	}
}

func TestResolveTableNameIgnoresQueryShardForOtherTable(t *testing.T) {
	main := shardedTestTable(&testShardingRule{name: "user_03"})
	other := shardedTestTable(&testShardingRule{name: "role_03"})
	rc := NewRenderContext(DialectMySQL)
	rc.SwapQueryShard(true, int64(19))
	rc.SetMainTable(main)
	if got := ResolveTableName(rc, other, "role"); got != "role" || rc.Err() == nil || !strings.Contains(rc.Err().Error(), "call ShardBy or Physical") {
		t.Fatalf("got %q err %v", got, rc.Err())
	}
}
