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

func TestShardingSlotInjectRequiresRule(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for a nil rule")
		}
	}()
	NewShardingSlot().Inject(nil)
}

func TestShardingSlotOverwrite(t *testing.T) {
	slot := NewShardingSlot()
	slot.Inject(testShardingRule{name: "first"})
	slot.Inject(testShardingRule{name: "second"})
	rc := NewRenderContext(DialectMySQL)
	rc.SetShardingValue(int64(1))
	if got := slot.Resolve(rc, "user"); got != "second" || rc.Err() != nil {
		t.Fatalf("got %q err %v", got, rc.Err())
	}
}

func TestShardingSlotResolve(t *testing.T) {
	slot := NewShardingSlot()
	if got := slot.Resolve(NewRenderContext(DialectMySQL), "user"); got != "user" {
		t.Fatalf("unregistered slot resolved to %q", got)
	}
	slot.Inject(testShardingRule{name: "user_03"})

	rc := NewRenderContext(DialectMySQL)
	if got := slot.Resolve(rc, "user"); got != "user" || rc.Err() == nil || !strings.Contains(rc.Err().Error(), "ShardingValue is required") {
		t.Fatalf("missing value got %q err %v", got, rc.Err())
	}

	rc = NewRenderContext(DialectMySQL)
	rc.SetShardingValue(int64(19))
	if got := slot.Resolve(rc, "user"); got != "user_03" || rc.Err() != nil {
		t.Fatalf("resolve got %q err %v", got, rc.Err())
	}

	boom := fmt.Errorf("rule boom")
	broken := NewShardingSlot()
	broken.Inject(testShardingRule{err: boom})
	rc = NewRenderContext(DialectMySQL)
	rc.SetShardingValue(int64(1))
	if got := broken.Resolve(rc, "user"); got != "user" || rc.Err() != boom {
		t.Fatalf("rule error got %q err %v", got, rc.Err())
	}

	empty := NewShardingSlot()
	empty.Inject(testShardingRule{})
	rc = NewRenderContext(DialectMySQL)
	rc.SetShardingValue(int64(1))
	if got := empty.Resolve(rc, "user"); got != "user" || rc.Err() == nil || !strings.Contains(rc.Err().Error(), "empty name") {
		t.Fatalf("empty name got %q err %v", got, rc.Err())
	}
}

func TestShardingSlotResolvesOncePerContext(t *testing.T) {
	calls := 0
	slot := NewShardingSlot()
	slot.Inject(countingShardingRule{calls: &calls})
	rc := NewRenderContext(DialectMySQL)
	rc.SetShardingValue(int64(19))
	for i := 0; i < 3; i++ {
		if got := slot.Resolve(rc, "user"); got != "user_03" {
			t.Fatalf("got %q", got)
		}
	}
	if calls != 1 {
		t.Fatalf("rule called %d times", calls)
	}

	missing := NewShardingSlot()
	missing.Inject(countingShardingRule{calls: &calls})
	rc = NewRenderContext(DialectMySQL)
	for i := 0; i < 2; i++ {
		if got := missing.Resolve(rc, "user"); got != "user" {
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
