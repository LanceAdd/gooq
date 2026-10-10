package gooq

import "testing"

func TestPropertyAssignment(t *testing.T) {
	var unset Property[int]
	if _, ok := unset.Assignment("age"); ok {
		t.Fatal("unset property produced an assignment")
	}
	if !unset.IsUnset() || unset.IsSet() || unset.IsNull() || unset.Get() != 0 {
		t.Fatalf("unset property: %+v", unset)
	}
	if value, ok := unset.Value(); ok || value != 0 {
		t.Fatalf("unset value: %v ok=%v", value, ok)
	}
	if cleared := Unset[int](); !cleared.IsUnset() {
		t.Fatal("Unset did not clear the cell")
	}

	set := Set(0)
	item, ok := set.Assignment("age")
	if !ok || item.Null || item.Column != "age" || item.Value != 0 {
		t.Fatalf("set property: %+v ok=%v", item, ok)
	}
	if !set.IsSet() || set.IsUnset() || set.IsNull() {
		t.Fatal("set property has the wrong state")
	}
	if value, ok := set.Value(); !ok || value != 0 {
		t.Fatalf("set value: %v ok=%v", value, ok)
	}

	null := Null[string]()
	item, ok = null.Assignment("name")
	if !ok || !item.Null || item.Column != "name" || null.Get() != "" {
		t.Fatalf("null property: %+v ok=%v", item, ok)
	}
	if !null.IsNull() || null.IsSet() || null.IsUnset() {
		t.Fatal("null property has the wrong state")
	}
	if value, ok := null.Value(); ok || value != "" {
		t.Fatalf("null value: %q ok=%v", value, ok)
	}
}
