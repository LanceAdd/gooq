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

func (r shardUserRule) TableName(table gooq.ShardingTableMeta, value any) (string, error) {
	id, ok := value.(int64)
	if !ok {
		return "", fmt.Errorf("shard key must be int64, got %T", value)
	}
	return fmt.Sprintf("%s%02d", r.prefix, id%r.shards), nil
}

type countingShardRule struct {
	calls *int
}

func (r countingShardRule) TableName(table gooq.ShardingTableMeta, value any) (string, error) {
	*r.calls++
	return fmt.Sprintf("shard_user_%02d", value.(int64)%16), nil
}

func TestShardingResolvesOncePerStatement(t *testing.T) {
	calls := 0
	u := newShardUserTable(&gooq.TableMeta{TableName: "shard_user", Sharding: countingShardRule{calls: &calls}, Fields: shardUserFields}, "")
	sql, _, err := Select(u.Id, u.Name).From(u).Where(u.Id.Eq(int64(19))).ShardBy(int64(19)).ToSql(gooq.DialectPgsql)
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

func (t *shardUserTable) ShardBy(value any) *shardUserTable {
	next := newShardUserTable(t.Meta(), t.AliasName())
	next.TableBase.SetShard(value)
	return next
}

func (t *shardUserTable) Physical(name string) *shardUserTable {
	if name == "" {
		panic("gooq: Physical requires a non-empty table name")
	}
	meta := *t.Meta()
	meta.TableName = name
	meta.Sharding = nil
	return newShardUserTable(&meta, t.AliasName())
}

func (t *shardUserTable) As(alias string) *shardUserTable {
	next := newShardUserTable(t.Meta(), alias)
	t.TableBase.CopyShard(next.TableBase)
	return next
}

func (t *shardUserTable) Clone() *shardUserTable {
	next := newShardUserTable(t.Meta(), "")
	t.TableBase.CopyShard(next.TableBase)
	return next
}

func TestShardingSelectPhysicalName(t *testing.T) {
	u := newShardUserTable(&gooq.TableMeta{TableName: "shard_user", Sharding: shardUserRule{prefix: "shard_user_", shards: 16}, Fields: shardUserFields}, "")
	sql, _, err := Select(u.Id, u.Name).From(u).Where(u.Id.Eq(int64(19))).ShardBy(int64(19)).ToSql(gooq.DialectPgsql)
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
	if err == nil || !strings.Contains(err.Error(), "ShardBy is required") {
		t.Fatalf("err %v", err)
	}
}

func TestShardingRuleError(t *testing.T) {
	u := newShardUserTable(&gooq.TableMeta{TableName: "shard_user", Sharding: shardUserRule{prefix: "shard_user_", shards: 16}, Fields: shardUserFields}, "")
	_, _, err := Select(u.Id).From(u).ShardBy("bad").ToSql(gooq.DialectPgsql)
	if err == nil || !strings.Contains(err.Error(), "shard key must be int64") {
		t.Fatalf("err %v", err)
	}
}

func TestShardingUnregisteredKeepsLogicalName(t *testing.T) {
	u := newShardUserTable(&gooq.TableMeta{TableName: "shard_user", Fields: shardUserFields}, "")
	sql, _, err := Select(u.Id).From(u).ShardBy(int64(19)).ToSql(gooq.DialectPgsql)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `FROM "shard_user"`) || strings.Contains(sql, "shard_user_03") {
		t.Fatalf("sql %s", sql)
	}
}

func TestShardingSubquerySharesValue(t *testing.T) {
	u := newShardUserTable(&gooq.TableMeta{TableName: "shard_user", Sharding: shardUserRule{prefix: "shard_user_", shards: 16}, Fields: shardUserFields}, "")
	sub := Select(u.Id).From(u).Where(u.Id.Eq(int64(19))).Limit(1).ShardBy(int64(19))
	sql, _, err := Select(u.Name).From(u).Where(u.Id.InExpr(sub)).ShardBy(int64(19)).ToSql(gooq.DialectPgsql)
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
	sql, _, err := Select(u.Id).From(u).ShardBy(int64(19)).UnionAll(Select(other.Id).From(other).ShardBy(int64(19))).ToSql(gooq.DialectPgsql)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(sql, `FROM "shard_user_03"`); got != 2 {
		t.Fatalf("sql %s", sql)
	}
}

func TestShardingDMLPhysicalName(t *testing.T) {
	u := newShardUserTable(&gooq.TableMeta{TableName: "shard_user", Sharding: shardUserRule{prefix: "shard_user_", shards: 16}, Fields: shardUserFields}, "")
	sql, _, err := Update(u).Set(u.Name, "x").Where(u.Id.Eq(int64(19))).ShardBy(int64(19)).ToSql(gooq.DialectPgsql)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `UPDATE "shard_user_03"`) {
		t.Fatalf("sql %s", sql)
	}
	if _, _, err = Insert(u).Columns(u.Name).Values("x").ShardBy(int64(19)).ToSql(gooq.DialectPgsql); err != nil {
		t.Fatal(err)
	} else if sql, _, err = Insert(u).Columns(u.Name).Values("x").ShardBy(int64(19)).ToSql(gooq.DialectPgsql); !strings.Contains(sql, `INSERT INTO "shard_user_03"`) {
		t.Fatalf("sql %s", sql)
	}
}

func TestShardingNamedEscape(t *testing.T) {
	base := newShardUserTable(&gooq.TableMeta{TableName: "shard_user", Sharding: shardUserRule{prefix: "shard_user_", shards: 16}, Fields: shardUserFields}, "")
	u := base.Physical("shard_user_05")
	sql, _, err := Select(u.Id).From(u).ToSql(gooq.DialectPgsql)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `FROM "shard_user_05"`) {
		t.Fatalf("sql %s", sql)
	}
	aliased := u.Physical("shard_user_07").As("t")
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

func TestShardingJoinNeedsOwnShard(t *testing.T) {
	user := newShardUserTable(&gooq.TableMeta{TableName: "user", Sharding: shardUserRule{prefix: "user_", shards: 16}, Fields: shardUserFields}, "")
	role := newShardUserTable(&gooq.TableMeta{TableName: "role", Sharding: shardUserRule{prefix: "role_", shards: 16}, Fields: shardUserFields}, "")
	_, _, err := Select(user.Id, role.Id).From(user).InnerJoin(role).On(role.Id.EqExpr(user.Id)).ShardBy(int64(19)).ToSql(gooq.DialectPgsql)
	if err == nil || !strings.Contains(err.Error(), `table "role" is sharded; call ShardBy or Physical`) {
		t.Fatalf("err %v", err)
	}
	r := role.ShardBy(int64(19))
	sql, _, err := Select(user.Name, r.Name).From(user).InnerJoin(r).On(r.Id.EqExpr(user.Id)).ShardBy(int64(19)).ToSql(gooq.DialectPgsql)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `"user_03"`) || !strings.Contains(sql, `"role_03"`) {
		t.Fatalf("sql %s", sql)
	}
}

func TestShardingJoinPlainTableIgnoresQueryShard(t *testing.T) {
	user := newShardUserTable(&gooq.TableMeta{TableName: "user", Sharding: shardUserRule{prefix: "user_", shards: 16}, Fields: shardUserFields}, "")
	role := newShardUserTable(&gooq.TableMeta{TableName: "role", Fields: shardUserFields}, "")
	sql, _, err := Select(user.Id, role.Id).From(user).InnerJoin(role).On(role.Id.EqExpr(user.Id)).ShardBy(int64(19)).ToSql(gooq.DialectPgsql)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `"user_03"`) || !strings.Contains(sql, `"role"`) || strings.Contains(sql, "role_") {
		t.Fatalf("sql %s", sql)
	}
}

func TestShardingAsKeepsShardValue(t *testing.T) {
	role := newShardUserTable(&gooq.TableMeta{TableName: "role", Sharding: shardUserRule{prefix: "role_", shards: 16}, Fields: shardUserFields}, "")
	r := role.ShardBy(int64(19)).As("r")
	sql, _, err := Select(r.Id).From(r).ToSql(gooq.DialectPgsql)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `FROM "role_03" AS "r"`) {
		t.Fatalf("sql %s", sql)
	}
	if _, _, err = Select(role.Id).From(role).ToSql(gooq.DialectPgsql); err == nil {
		t.Fatal("original table kept the shard value")
	}
}

func TestShardingSubqueryDoesNotInherit(t *testing.T) {
	u := newShardUserTable(&gooq.TableMeta{TableName: "shard_user", Sharding: shardUserRule{prefix: "shard_user_", shards: 16}, Fields: shardUserFields}, "")
	sub := Select(u.Id).From(u)
	_, _, err := Select(u.Name).From(u).Where(u.Id.InExpr(sub)).ShardBy(int64(19)).ToSql(gooq.DialectPgsql)
	if err == nil || !strings.Contains(err.Error(), "ShardBy is required") {
		t.Fatalf("err %v", err)
	}
}

func TestShardingBatchDMLRequiresShardBy(t *testing.T) {
	u := newShardUserTable(&gooq.TableMeta{TableName: "shard_user", Sharding: shardUserRule{prefix: "shard_user_", shards: 16}, Fields: shardUserFields}, "")
	rows := []gooq.ValueSource{
		assignRow{setCol("id", int64(1)), setCol("name", "a")},
		assignRow{setCol("id", int64(2)), setCol("name", "b")},
	}
	sqls, _, err := Update(u).Keys(u.Id).ValuesOf(rows...).renderBatchDML(gooq.DialectPgsql)
	if err == nil || !strings.Contains(err.Error(), "ShardBy is required") || sqls != nil {
		t.Fatalf("update err %v sqls %v", err, sqls)
	}
	sqls, _, err = Update(u).Keys(u.Id).ValuesOf(rows...).ShardBy(int64(19)).renderBatchDML(gooq.DialectPgsql)
	if err != nil {
		t.Fatal(err)
	}
	if len(sqls) != 2 || !strings.Contains(sqls[0], `UPDATE "shard_user_03"`) {
		t.Fatalf("sqls %v", sqls)
	}
	sqls, _, err = Delete(u).Keys(u.Id).ValuesOf(assignRow{setCol("id", int64(1))}).renderBatchDML(gooq.DialectPgsql)
	if err == nil || !strings.Contains(err.Error(), "ShardBy is required") || sqls != nil {
		t.Fatalf("delete err %v sqls %v", err, sqls)
	}
}

func TestShardingGroupedCountUsesInnerShard(t *testing.T) {
	u := newShardUserTable(&gooq.TableMeta{TableName: "shard_user", Sharding: shardUserRule{prefix: "shard_user_", shards: 16}, Fields: shardUserFields}, "")
	q := Select(u.Name, gooq.Raw("count(*)")).From(u).Where(u.Id.Eq(int64(19))).Group(u.Name).ShardBy(int64(19))
	sql, _, err := q.countSQL(gooq.DialectPgsql)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `FROM "shard_user_03"`) || !strings.Contains(sql, "COUNT(*)") {
		t.Fatalf("sql %s", sql)
	}
}
