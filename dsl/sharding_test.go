package dsl

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lanceadd/gooq"
)

type shardUserTable struct {
	*gooq.TableBase
	Id   gooq.Field[int64]
	Name gooq.Field[string]
}

var shardUserFields = []gooq.FieldMeta{
	{ColumnName: "id", LocalType: gooq.LocalTypeInt64},
	{ColumnName: "name", LocalType: gooq.LocalTypeString},
}

func newShardUserTable(meta *gooq.TableMeta, alias string) *shardUserTable {
	t := &shardUserTable{TableBase: gooq.NewTableBase(meta)}
	if alias != "" {
		t.TableBase = t.TableBase.As(alias)
	}
	t.Id = gooq.NewFieldAt[int64](t.TableBase, "id")
	t.Name = gooq.NewFieldAt[string](t.TableBase, "name")
	return t
}

type shardUserRule struct {
	prefix string
	shards int64
}

func (r shardUserRule) TableName(value any) (string, error) {
	id, ok := value.(int64)
	if !ok {
		return "", fmt.Errorf("shard key must be int64, got %T", value)
	}
	return fmt.Sprintf("%s%02d", r.prefix, id%r.shards), nil
}

type countingShardRule struct {
	calls *int
}

func (r countingShardRule) TableName(value any) (string, error) {
	*r.calls++
	return fmt.Sprintf("shard_user_%02d", value.(int64)%16), nil
}

func TestShardingResolvesOncePerStatement(t *testing.T) {
	calls := 0
	u := newShardUserTable(&gooq.TableMeta{TableName: "shard_user", Sharding: countingShardRule{calls: &calls}, Fields: shardUserFields}, "")
	sql, _, err := Select(u.Id, u.Name).From(u).Where(u.Id.Eq(int64(19))).ShardingValue(int64(19)).ToSql(gooq.DialectPgsql)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(sql, `"shard_user_03"`); got != 4 {
		t.Fatalf("sql %s", sql)
	}
	if calls != 1 {
		t.Fatalf("rule called %d times", calls)
	}
}

func (t *shardUserTable) Named(name string) *shardUserTable {
	if name == "" {
		panic("gooq: Named requires a non-empty table name")
	}
	meta := *t.Meta()
	meta.TableName = name
	meta.Sharding = nil
	return newShardUserTable(&meta, t.AliasName())
}

func (t *shardUserTable) As(alias string) *shardUserTable { return newShardUserTable(t.Meta(), alias) }

func (t *shardUserTable) Clone() *shardUserTable { return newShardUserTable(t.Meta(), "") }

func TestShardingSelectPhysicalName(t *testing.T) {
	u := newShardUserTable(&gooq.TableMeta{TableName: "shard_user", Sharding: shardUserRule{prefix: "shard_user_", shards: 16}, Fields: shardUserFields}, "")
	sql, _, err := Select(u.Id, u.Name).From(u).Where(u.Id.Eq(int64(19))).ShardingValue(int64(19)).ToSql(gooq.DialectPgsql)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `FROM "shard_user_03"`) {
		t.Fatalf("sql %s", sql)
	}
	if !strings.Contains(sql, `"shard_user_03"."id"`) {
		t.Fatalf("sql %s", sql)
	}
}

func TestShardingMissingValue(t *testing.T) {
	u := newShardUserTable(&gooq.TableMeta{TableName: "shard_user", Sharding: shardUserRule{prefix: "shard_user_", shards: 16}, Fields: shardUserFields}, "")
	_, _, err := Select(u.Id).From(u).ToSql(gooq.DialectPgsql)
	if err == nil || !strings.Contains(err.Error(), "ShardingValue is required") {
		t.Fatalf("err %v", err)
	}
}

func TestShardingRuleError(t *testing.T) {
	u := newShardUserTable(&gooq.TableMeta{TableName: "shard_user", Sharding: shardUserRule{prefix: "shard_user_", shards: 16}, Fields: shardUserFields}, "")
	_, _, err := Select(u.Id).From(u).ShardingValue("bad").ToSql(gooq.DialectPgsql)
	if err == nil || !strings.Contains(err.Error(), "shard key must be int64") {
		t.Fatalf("err %v", err)
	}
}

func TestShardingUnregisteredKeepsLogicalName(t *testing.T) {
	u := newShardUserTable(&gooq.TableMeta{TableName: "shard_user", Fields: shardUserFields}, "")
	sql, _, err := Select(u.Id).From(u).ShardingValue(int64(19)).ToSql(gooq.DialectPgsql)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `FROM "shard_user"`) || strings.Contains(sql, "shard_user_03") {
		t.Fatalf("sql %s", sql)
	}
}

func TestShardingSubquerySharesValue(t *testing.T) {
	u := newShardUserTable(&gooq.TableMeta{TableName: "shard_user", Sharding: shardUserRule{prefix: "shard_user_", shards: 16}, Fields: shardUserFields}, "")
	sub := Select(u.Id).From(u).Where(u.Id.Eq(int64(19))).Limit(1)
	sql, _, err := Select(u.Name).From(u).Where(u.Id.InExpr(sub)).ShardingValue(int64(19)).ToSql(gooq.DialectPgsql)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(sql, `"shard_user_03"`); got < 2 {
		t.Fatalf("sql %s", sql)
	}
}

func TestShardingUnionSharesValue(t *testing.T) {
	u := newShardUserTable(&gooq.TableMeta{TableName: "shard_user", Sharding: shardUserRule{prefix: "shard_user_", shards: 16}, Fields: shardUserFields}, "")
	other := newShardUserTable(&gooq.TableMeta{TableName: "shard_user", Sharding: shardUserRule{prefix: "shard_user_", shards: 16}, Fields: shardUserFields}, "")
	sql, _, err := Select(u.Id).From(u).ShardingValue(int64(19)).UnionAll(Select(other.Id).From(other)).ToSql(gooq.DialectPgsql)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(sql, `FROM "shard_user_03"`); got != 2 {
		t.Fatalf("sql %s", sql)
	}
}

func TestShardingDMLPhysicalName(t *testing.T) {
	u := newShardUserTable(&gooq.TableMeta{TableName: "shard_user", Sharding: shardUserRule{prefix: "shard_user_", shards: 16}, Fields: shardUserFields}, "")
	sql, _, err := Update(u).Set(u.Name, "x").Where(u.Id.Eq(int64(19))).ShardingValue(int64(19)).ToSql(gooq.DialectPgsql)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `UPDATE "shard_user_03"`) {
		t.Fatalf("sql %s", sql)
	}
	if _, _, err = Insert(u).Columns(u.Name).Values("x").ShardingValue(int64(19)).ToSql(gooq.DialectPgsql); err != nil {
		t.Fatal(err)
	} else if sql, _, err = Insert(u).Columns(u.Name).Values("x").ShardingValue(int64(19)).ToSql(gooq.DialectPgsql); !strings.Contains(sql, `INSERT INTO "shard_user_03"`) {
		t.Fatalf("sql %s", sql)
	}
}

func TestShardingNamedEscape(t *testing.T) {
	base := newShardUserTable(&gooq.TableMeta{TableName: "shard_user", Sharding: shardUserRule{prefix: "shard_user_", shards: 16}, Fields: shardUserFields}, "")
	u := base.Named("shard_user_05")
	sql, _, err := Select(u.Id).From(u).ToSql(gooq.DialectPgsql)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `FROM "shard_user_05"`) {
		t.Fatalf("sql %s", sql)
	}
	aliased := u.Named("shard_user_07").As("t")
	sql, _, err = Select(aliased.Id).From(aliased).ToSql(gooq.DialectPgsql)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `FROM "shard_user_07" AS "t"`) || !strings.Contains(sql, `"t"."id"`) {
		t.Fatalf("sql %s", sql)
	}
	cloned := u.Clone()
	sql, _, err = Select(cloned.Id).From(cloned).ToSql(gooq.DialectPgsql)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `FROM "shard_user_05"`) {
		t.Fatalf("sql %s", sql)
	}
}
