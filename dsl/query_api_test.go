package dsl

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
	_ "github.com/lanceadd/gooq/drivers/sqlite"
	"github.com/lanceadd/gooq/fn"
)

func TestRowExecutesSingleRowSQL(t *testing.T) {
	fe := &fakeExecutor{}
	b := Select(testUser.Id).From(testUser).Offset(4)
	b.executor = fe
	record, err := b.Row(context.Background())
	if err != nil || record != nil {
		t.Fatalf("row=%v err=%v", record, err)
	}
	want, _, err := Select(testUser.Id).From(testUser).Offset(4).oneRowSQL(gooq.DialectMySQL)
	if err != nil {
		t.Fatal(err)
	}
	if fe.lastSql != want {
		t.Fatalf("executed %s", fe.lastSql)
	}
	unchanged, _, err := b.ToSql(gooq.DialectMySQL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(unchanged, "18446744073709551615") {
		t.Fatalf("builder changed: %s", unchanged)
	}

	fe.calls = nil
	wide := Select(testUser.Id).From(testUser).Page(2, 10)
	wide.executor = fe
	if _, err = wide.Row(context.Background()); err == nil || len(fe.calls) != 0 {
		t.Fatalf("err=%v calls=%v", err, fe.calls)
	}
}

func TestQueryShapeRejectsBeforeExecution(t *testing.T) {
	ctx := context.Background()
	var id int64
	if err := Select(testUser.Name).From(testUser).Page(2, 10).Value(ctx, &id); err == nil {
		t.Fatal("expected Value to reject a multi-row page")
	}
	if _, err := Select(testUser.Id).From(testUser).Page(2, 10).Row(ctx); err == nil {
		t.Fatal("expected Row to reject a multi-row page")
	}
}

type memCache struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (c *memCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.data[key]
	return value, ok, nil
}

func (c *memCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		c.data = map[string][]byte{}
	}
	c.data[key] = append([]byte(nil), value...)
	return nil
}

func (c *memCache) Remove(context.Context, string) error { return nil }

func (c *memCache) Removes(context.Context, []string) error { return nil }

func (c *memCache) has(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.data[key]
	return ok
}

func TestQueryAPIExecution(t *testing.T) {
	ctx := context.Background()
	group := "query-api"
	path := filepath.Join(t.TempDir(), "query.db")
	err := db.SetConfig(group, db.Config{Link: "sqlite::@file(" + filepath.ToSlash(path) + ")"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.RemoveConfig(group); err != nil {
			t.Error(err)
		}
	})
	database, err := db.Instance(group)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(ctx, `
CREATE TABLE user (
    id INTEGER PRIMARY KEY,
    name TEXT,
    age INTEGER,
    status TEXT,
    created_at TEXT,
    deleted_at TEXT
)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(ctx, `INSERT INTO user(id, name, age) VALUES (1, 'ann', 20), (2, 'bob', 20), (3, 'cy', 30)`)
	if err != nil {
		t.Fatal(err)
	}

	base := func() *SelectBuilder {
		return Select(testUser.Name).From(testUser).Order(testUser.Id.Asc()).DB(group)
	}
	var name string
	query := base()
	if err = query.Value(ctx, &name); err != nil || name != "ann" {
		t.Fatalf("value %q err=%v", name, err)
	}
	plain, _, err := query.ToSql(gooq.DialectSQLite)
	if err != nil || strings.Contains(plain, "LIMIT") {
		t.Fatalf("builder changed: %s err=%v", plain, err)
	}

	if err = base().Offset(1).Value(ctx, &name); err != nil || name != "bob" {
		t.Fatalf("offset value %q err=%v", name, err)
	}
	if err = base().Page(3, 1).Value(ctx, &name); err != nil || name != "cy" {
		t.Fatalf("page value %q err=%v", name, err)
	}
	if err = base().Page(2, 10).Value(ctx, &name); err == nil {
		t.Fatal("expected page value to fail")
	}
	if _, err = Select(testUser.Id, testUser.Name).From(testUser).Order(testUser.Id.Asc()).DB(group).Row(ctx); err != nil {
		t.Fatal(err)
	}
	if row, err := Select(testUser.Id).From(testUser).Where(testUser.Id.Eq(9)).DB(group).Row(ctx); err != nil || row != nil {
		t.Fatalf("missing row=%v err=%v", row, err)
	}
	if err = Select(testUser.Name).From(testUser).Where(testUser.Id.Eq(9)).DB(group).Value(ctx, &name); err != sql.ErrNoRows {
		t.Fatalf("missing value err=%v", err)
	}

	var names []string
	if err = base().Column(ctx, &names); err != nil || len(names) != 3 || names[2] != "cy" {
		t.Fatalf("column %v err=%v", names, err)
	}
	names = nil
	if err = base().Page(2, 1).Column(ctx, &names); err != nil || len(names) != 1 || names[0] != "bob" {
		t.Fatalf("page column %v err=%v", names, err)
	}
	rows, err := Select(testUser.Id).From(testUser).Order(testUser.Id.Asc()).Page(2, 1).DB(group).Rows(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("page rows %v err=%v", rows, err)
	}

	var id int64
	if err = Select(testUser.Id, testUser.Name).From(testUser).DB(group).Value(ctx, &id); err == nil || !strings.Contains(err.Error(), "expected 1") {
		t.Fatalf("wide value err=%v", err)
	}
	if err = SelectFrom(testUser).DB(group).Column(ctx, &names); err == nil || !strings.Contains(err.Error(), "expected 1") {
		t.Fatalf("star column err=%v", err)
	}
	if _, err = Select(testUser.Id, testUser.Name).From(testUser).DB(group).ColumnAndCount(ctx, &names); err == nil || !strings.Contains(err.Error(), "expected 1") {
		t.Fatalf("wide column count err=%v", err)
	}
	if err = Select(gooq.Raw("name, age")).From(testUser).DB(group).Value(ctx, &name); err == nil || !strings.Contains(err.Error(), "expected 1") {
		t.Fatalf("raw value err=%v", err)
	}
	if err = Select(gooq.Raw("name, age")).From(testUser).Where(testUser.Id.Eq(9)).DB(group).Value(ctx, &name); err == nil || err == sql.ErrNoRows {
		t.Fatalf("empty raw value err=%v", err)
	}
	if err = Select(gooq.Raw("name, age")).From(testUser).DB(group).Column(ctx, &names); err == nil {
		t.Fatal("expected raw column to fail")
	}

	counted, err := Select(testUser.Id).From(testUser).Page(2, 10).DB(group).Count(ctx)
	if err != nil || counted != 3 {
		t.Fatalf("count %d err=%v", counted, err)
	}
	found, err := Select(testUser.Id).From(testUser).Page(2, 10).DB(group).Exists(ctx)
	if err != nil || !found {
		t.Fatalf("exists %v err=%v", found, err)
	}
	found, err = Select(testUser.Id).From(testUser).Where(testUser.Id.Eq(9)).Page(2, 10).DB(group).Exists(ctx)
	if err != nil || found {
		t.Fatalf("missing exists %v err=%v", found, err)
	}

	grouped := Select(testUser.Age).From(testUser).Group(testUser.Age).Having(gooq.Gt(fn.Count(testUser.Id), 1)).DB(group)
	if counted, err = grouped.Count(ctx); err != nil || counted != 1 {
		t.Fatalf("group count %d err=%v", counted, err)
	}
	if found, err = grouped.Exists(ctx); err != nil || !found {
		t.Fatalf("group exists %v err=%v", found, err)
	}
	emptyGroup := Select(testUser.Age).From(testUser).Where(testUser.Age.Eq(30)).Group(testUser.Age).Having(gooq.Gt(fn.Count(testUser.Id), 1)).DB(group)
	if counted, err = emptyGroup.Count(ctx); err != nil || counted != 0 {
		t.Fatalf("empty group count %d err=%v", counted, err)
	}
	if found, err = emptyGroup.Exists(ctx); err != nil || found {
		t.Fatalf("empty group exists %v err=%v", found, err)
	}

	union := Select(testUser.Id).From(testUser).Where(testUser.Age.Eq(30)).
		UnionAll(Select(testUser.Id).From(testUser).Where(testUser.Age.Eq(99))).
		Page(2, 10).DB(group)
	if counted, err = union.Count(ctx); err != nil || counted != 1 {
		t.Fatalf("union count %d err=%v", counted, err)
	}
	if found, err = union.Exists(ctx); err != nil || !found {
		t.Fatalf("union exists %v err=%v", found, err)
	}

	names = nil
	total, err := base().Page(2, 1).ColumnAndCount(ctx, &names)
	if err != nil || total != 3 || len(names) != 1 || names[0] != "bob" {
		t.Fatalf("column count total=%d names=%v err=%v", total, names, err)
	}

	cache := &memCache{}
	previous := GetCacheAdapter()
	SetCacheAdapter(cache)
	t.Cleanup(func() { SetCacheAdapter(previous) })
	cached := Select(testUser.Name).From(testUser).Where(testUser.Id.Eq(1)).DB(group).Cache(CacheOption{Duration: time.Minute})
	limited, args, err := cached.oneRowSQL(gooq.DialectSQLite)
	if err != nil {
		t.Fatal(err)
	}
	key, err := cached.cacheKey("value", limited, args)
	if err != nil {
		t.Fatal(err)
	}
	plain, plainArgs, err := cached.ToSql(gooq.DialectSQLite)
	if err != nil {
		t.Fatal(err)
	}
	plainKey, err := cached.cacheKey("value", plain, plainArgs)
	if err != nil {
		t.Fatal(err)
	}
	if key == plainKey {
		t.Fatal("value cache key ignored LIMIT 1")
	}
	if err = cached.Value(ctx, &name); err != nil || name != "ann" {
		t.Fatalf("cached value %q err=%v", name, err)
	}
	if !cache.has(key) || cache.has(plainKey) {
		t.Fatalf("cached keys %+v", cache.data)
	}
	if _, err = database.Exec(ctx, `DELETE FROM user WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	name = ""
	if err = cached.Value(ctx, &name); err != nil || name != "ann" {
		t.Fatalf("cache hit %q err=%v", name, err)
	}
}

type prefixCodec struct{}

func (prefixCodec) Marshal(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append([]byte{'X'}, raw...), nil
}

func (prefixCodec) Unmarshal(data []byte, dest any) error {
	if len(data) == 0 || data[0] != 'X' {
		return fmt.Errorf("missing codec prefix")
	}
	return json.Unmarshal(data[1:], dest)
}

func (prefixCodec) UnmarshalRecord(data []byte, dest any) error {
	if len(data) == 0 || data[0] != 'X' {
		return fmt.Errorf("missing codec prefix")
	}
	decoder := json.NewDecoder(bytes.NewReader(data[1:]))
	decoder.UseNumber()
	return decoder.Decode(dest)
}

type byteCache struct {
	mu   sync.Mutex
	str  map[string][]byte
	hash map[string]map[string][]byte
}

func (c *byteCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.str[key]
	return value, ok, nil
}

func (c *byteCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.str == nil {
		c.str = map[string][]byte{}
	}
	c.str[key] = append([]byte(nil), value...)
	return nil
}

func (c *byteCache) Remove(context.Context, string) error { return nil }

func (c *byteCache) Removes(context.Context, []string) error { return nil }

func (c *byteCache) HGet(_ context.Context, key, field string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.hash[key][field]
	return value, ok, nil
}

func (c *byteCache) HGetAll(_ context.Context, key string) (map[string][]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string][]byte{}
	for field, value := range c.hash[key] {
		out[field] = value
	}
	return out, nil
}

func (c *byteCache) HSet(_ context.Context, key, field string, value []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hash == nil {
		c.hash = map[string]map[string][]byte{}
	}
	if c.hash[key] == nil {
		c.hash[key] = map[string][]byte{}
	}
	c.hash[key][field] = append([]byte(nil), value...)
	return nil
}

func (c *byteCache) HDel(context.Context, string, ...string) error { return nil }

func TestCacheCodecUsedByQuery(t *testing.T) {
	ctx := context.Background()
	group := "cache-codec"
	path := filepath.Join(t.TempDir(), "codec.db")
	err := db.SetConfig(group, db.Config{Link: "sqlite::@file(" + filepath.ToSlash(path) + ")"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.RemoveConfig(group) })
	database, err := db.Instance(group)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(ctx, `
CREATE TABLE user (
    id INTEGER PRIMARY KEY,
    name TEXT,
    age INTEGER,
    status TEXT,
    created_at TEXT,
    deleted_at TEXT
)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Exec(ctx, `INSERT INTO user(id, name) VALUES (1, 'ann')`); err != nil {
		t.Fatal(err)
	}

	cache := &byteCache{}
	previousAdapter := GetCacheAdapter()
	previousHash := GetHashCacheAdapter()
	SetCacheAdapter(cache)
	SetHashCacheAdapter(cache)
	SetCacheCodec(prefixCodec{})
	t.Cleanup(func() {
		SetCacheAdapter(previousAdapter)
		SetHashCacheAdapter(previousHash)
		SetCacheCodec(nil)
	})

	option := CacheOption{Duration: time.Minute}
	var name string
	query := Select(testUser.Name).From(testUser).Where(testUser.Id.Eq(1)).DB(group).Cache(option)
	if err = query.Value(ctx, &name); err != nil || name != "ann" {
		t.Fatalf("value %q err=%v", name, err)
	}
	stored, _, err := cache.Get(ctx, mustCacheKey(t, query, "value"))
	if err != nil || len(stored) == 0 || stored[0] != 'X' || string(stored[1:]) != `"ann"` {
		t.Fatalf("value payload %q err=%v", stored, err)
	}
	if _, err = database.Exec(ctx, `DELETE FROM user WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	name = ""
	if err = query.Value(ctx, &name); err != nil || name != "ann" {
		t.Fatalf("value hit %q err=%v", name, err)
	}

	if _, err = database.Exec(ctx, `INSERT INTO user(id, name) VALUES (1, 'ann')`); err != nil {
		t.Fatal(err)
	}
	rowQuery := Select(testUser.Id, testUser.Name).From(testUser).Where(testUser.Id.Eq(1)).DB(group).Cache(option)
	row, err := rowQuery.Row(ctx)
	if err != nil || row["name"] != "ann" {
		t.Fatalf("row %#v err=%v", row, err)
	}
	stored, _, err = cache.Get(ctx, mustCacheKey(t, rowQuery, "row"))
	if err != nil || len(stored) == 0 || stored[0] != 'X' {
		t.Fatalf("row payload %q err=%v", stored, err)
	}
	if _, err = database.Exec(ctx, `DELETE FROM user WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	row, err = rowQuery.Row(ctx)
	if err != nil || row["name"] != "ann" {
		t.Fatalf("row hit %#v err=%v", row, err)
	}
	id, err := db.Value(row, testUser.Id)
	if err != nil || id != int64(1) {
		t.Fatalf("row id %#v err=%v", id, err)
	}
}

func TestCacheIsolatesConfigGroups(t *testing.T) {
	ctx := context.Background()
	cache := &memCache{}
	previous := GetCacheAdapter()
	SetCacheAdapter(cache)
	t.Cleanup(func() { SetCacheAdapter(previous) })
	option := CacheOption{Duration: time.Minute}
	for _, item := range []struct {
		group string
		name  string
	}{
		{group: "cache-group-a", name: "ann"},
		{group: "cache-group-b", name: "bob"},
	} {
		path := filepath.Join(t.TempDir(), item.group+".db")
		err := db.SetConfig(item.group, db.Config{Link: "sqlite::@file(" + filepath.ToSlash(path) + ")"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.RemoveConfig(item.group) })
		database, err := db.Instance(item.group)
		if err != nil {
			t.Fatal(err)
		}
		_, err = database.Exec(ctx, `
CREATE TABLE user (
    id INTEGER PRIMARY KEY,
    name TEXT,
    age INTEGER,
    status TEXT,
    created_at TEXT,
    deleted_at TEXT
)`)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = database.Exec(ctx, `INSERT INTO user(id, name) VALUES (1, '`+item.name+`')`); err != nil {
			t.Fatal(err)
		}
	}
	var name string
	err := Select(testUser.Name).From(testUser).Where(testUser.Id.Eq(1)).DB("cache-group-a").Cache(option).Value(ctx, &name)
	if err != nil || name != "ann" {
		t.Fatalf("group a %q err=%v", name, err)
	}
	name = ""
	err = Select(testUser.Name).From(testUser).Where(testUser.Id.Eq(1)).DB("cache-group-b").Cache(option).Value(ctx, &name)
	if err != nil || name != "bob" {
		t.Fatalf("group b %q err=%v", name, err)
	}
}

func mustCacheKey(t *testing.T, b *SelectBuilder, kind string) string {
	t.Helper()
	sql, args, err := b.oneRowSQL(gooq.DialectSQLite)
	if err != nil {
		t.Fatal(err)
	}
	key, err := b.cacheKey(kind, sql, args)
	if err != nil {
		t.Fatal(err)
	}
	return key
}
