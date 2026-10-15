package gooq

import (
	"fmt"
	"strings"
	"testing"
)

type testShardingRule struct {
	name string
	err  error
}

func (r testShardingRule) TableName(any) (string, error) {
	return r.name, r.err
}

type countingShardingRule struct {
	calls *int
}

func (r countingShardingRule) TableName(value any) (string, error) {
	*r.calls++
	return fmt.Sprintf("user_%02d", value.(int64)%16), nil
}

func shardedTestMeta(rule ShardingRule) *TableMeta {
	return &TableMeta{TableName: "user", Sharding: rule}
}

func TestResolveShardedNameUnregistered(t *testing.T) {
	if got := ResolveShardedName(NewRenderContext(DialectMySQL), shardedTestMeta(nil), "user"); got != "user" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveShardedName(t *testing.T) {
	meta := shardedTestMeta(testShardingRule{name: "user_03"})

	rc := NewRenderContext(DialectMySQL)
	if got := ResolveShardedName(rc, meta, "user"); got != "user" || rc.Err() == nil || !strings.Contains(rc.Err().Error(), "ShardingValue is required") {
		t.Fatalf("missing value got %q err %v", got, rc.Err())
	}

	rc = NewRenderContext(DialectMySQL)
	rc.SetShardingValue(int64(19))
	if got := ResolveShardedName(rc, meta, "user"); got != "user_03" || rc.Err() != nil {
		t.Fatalf("resolve got %q err %v", got, rc.Err())
	}

	boom := fmt.Errorf("rule boom")
	broken := shardedTestMeta(testShardingRule{err: boom})
	rc = NewRenderContext(DialectMySQL)
	rc.SetShardingValue(int64(1))
	if got := ResolveShardedName(rc, broken, "user"); got != "user" || rc.Err() != boom {
		t.Fatalf("rule error got %q err %v", got, rc.Err())
	}

	empty := shardedTestMeta(testShardingRule{})
	rc = NewRenderContext(DialectMySQL)
	rc.SetShardingValue(int64(1))
	if got := ResolveShardedName(rc, empty, "user"); got != "user" || rc.Err() == nil || !strings.Contains(rc.Err().Error(), "empty name") {
		t.Fatalf("empty name got %q err %v", got, rc.Err())
	}
}

func TestResolveShardedNameOncePerContext(t *testing.T) {
	calls := 0
	meta := shardedTestMeta(countingShardingRule{calls: &calls})
	rc := NewRenderContext(DialectMySQL)
	rc.SetShardingValue(int64(19))
	for i := 0; i < 3; i++ {
		if got := ResolveShardedName(rc, meta, "user"); got != "user_03" {
			t.Fatalf("got %q", got)
		}
	}
	if calls != 1 {
		t.Fatalf("rule called %d times", calls)
	}

	missing := shardedTestMeta(countingShardingRule{calls: &calls})
	rc = NewRenderContext(DialectMySQL)
	for i := 0; i < 2; i++ {
		if got := ResolveShardedName(rc, missing, "user"); got != "user" {
			t.Fatalf("missing value got %q", got)
		}
	}
	if rc.Err() == nil || calls != 1 {
		t.Fatalf("err %v calls %d", rc.Err(), calls)
	}
}

func TestRenderContextShardingValueKeepsFirst(t *testing.T) {
	rc := NewRenderContext(DialectMySQL)
	rc.SetShardingValue(int64(1))
	rc.SetShardingValue(int64(2))
	if got := rc.ShardingValue(); got != int64(1) {
		t.Fatalf("kept %v", got)
	}
	fresh := NewRenderContext(DialectMySQL)
	fresh.SetShardingValue(nil)
	fresh.SetShardingValue(int64(5))
	if got := fresh.ShardingValue(); got != int64(5) {
		t.Fatalf("nil did not pass through, got %v", got)
	}
}
