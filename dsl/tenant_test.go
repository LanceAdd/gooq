package dsl

import (
	"context"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/test/gtest"
	"github.com/google/uuid"

	"github.com/lanceadd/gooq"
)

func tenantTable(name string, localType gooq.LocalType) (*gooq.TableBase, gooq.Field[int64], gooq.Field[string]) {
	meta := &gooq.TableMeta{
		TableName: name,
		Fields: []gooq.FieldMeta{
			{ColumnName: "id", LocalType: gooq.LocalTypeInt64, Primary: true},
			{ColumnName: "tenant_id", LocalType: localType, Tenant: true},
			{ColumnName: "name", LocalType: gooq.LocalTypeString},
		},
	}
	base := gooq.NewTableBase(meta)
	return base, gooq.NewFieldAt[int64](base, "id"), gooq.NewFieldAt[string](base, "name")
}

func TestDsl_Tenant_Select(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		user, id, name := tenantTable("user", gooq.LocalTypeInt64)
		sql, args, err := Select(id, name).From(user).Where(name.Eq("a")).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id`, `user`.`name` FROM `user` WHERE `user`.`name` = ? AND `user`.`tenant_id` = ?")
		t.AssertEQ(args, []any{"a", int64(7)})

		sql, args, err = Select(id).From(user).Tenant(int64(1), int64(2)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`tenant_id` IN (?, ?)")
		t.AssertEQ(args, []any{int64(1), int64(2)})

		_, _, err = Select(id).From(user).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "tenant value is required"), true)

		sql, args, err = Select(id).From(user).Where(user.Field("tenant_id").Eq(int64(9))).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`tenant_id` = ? AND `user`.`tenant_id` = ?")
		t.AssertEQ(args, []any{int64(9), int64(7)})

		sql, _, err = Select(id).From(user).UnTenant().ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user`")

		_, _, err = Select(id).From(user).Tenant(1).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "expects int64"), true)
	})
}

func TestDsl_Tenant_JoinAndSubquery(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		user, userID, _ := tenantTable("user", gooq.LocalTypeInt64)
		order, orderID, _ := tenantTable("order", gooq.LocalTypeInt64)
		user2 := user.As("u2")

		sql, args, err := Select(userID).From(user).
			LeftJoin(order).On(orderID.EqExpr(userID)).
			Tenant(int64(7)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` LEFT JOIN `order` ON `order`.`id` = `user`.`id` AND `order`.`tenant_id` = ? WHERE `user`.`tenant_id` = ?")
		t.AssertEQ(args, []any{int64(7), int64(7)})

		sql, _, err = Select(userID).From(user).
			InnerJoin(order).On(orderID.EqExpr(userID)).
			Tenant(int64(7)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` INNER JOIN `order` ON `order`.`id` = `user`.`id` WHERE `user`.`tenant_id` = ? AND `order`.`tenant_id` = ?")

		sql, args, err = Select(userID).From(user).
			InnerJoin(user2).On(gooq.NewFieldAt[int64](user2, "id").EqExpr(userID)).
			Tenant(int64(7)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(strings.Contains(sql, "`u2`.`tenant_id` = ?"), true)
		t.Assert(strings.Contains(sql, "`user`.`tenant_id` = ?"), true)
		t.AssertEQ(args, []any{int64(7), int64(7)})

		inner := Select(orderID).From(order)
		sql, args, err = Select(userID).From(user).Where(userID.InExpr(inner)).Tenant(int64(5)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`id` IN (SELECT `order`.`id` FROM `order` WHERE `order`.`tenant_id` = ?) AND `user`.`tenant_id` = ?")
		t.AssertEQ(args, []any{int64(5), int64(5)})

		inner = Select(orderID).From(order).UnTenant()
		sql, args, err = Select(userID).From(user).Where(userID.InExpr(inner)).Tenant(int64(5)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`id` IN (SELECT `order`.`id` FROM `order`) AND `user`.`tenant_id` = ?")
		t.AssertEQ(args, []any{int64(5)})

		ctx := WithTenant(context.Background(), int64(8))
		outer := Select(userID).From(user).Where(userID.InExpr(Select(orderID).From(order))).UnTenant()
		outer.stampTenant(ctx)
		sql, args, err = outer.ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`id` IN (SELECT `order`.`id` FROM `order` WHERE `order`.`tenant_id` = ?)")
		t.AssertEQ(args, []any{int64(8)})
	})
}

func TestDsl_Tenant_ContextAndCache(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		user, id, _ := tenantTable("user", gooq.LocalTypeInt64)
		ctx := WithTenant(context.Background(), int64(3))
		got, ok := TenantFromContext(ctx)
		t.Assert(ok, true)
		t.AssertEQ(got, int64(3))

		query := Select(id).From(user)
		query.stampTenant(ctx)
		sql, args, err := query.ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE `user`.`tenant_id` = ?")
		t.AssertEQ(args, []any{int64(3)})

		query.Cache(CacheOption{Name: "user-by-id"})
		key, err := query.cacheKey("rows", sql, args)
		t.AssertNil(err)
		t.Assert(key, "user-by-id")

		open := Select(id).From(user).UnTenant()
		open.stampTenant(ctx)
		open.Cache(CacheOption{Name: "user-by-id"})
		key, err = open.cacheKey("rows", "SELECT 1", nil)
		t.AssertNil(err)
		t.Assert(key, "user-by-id")

		bare := Select(id).From(user).Tenant()
		bare.stampTenant(ctx)
		sql, args, err = bare.ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.AssertEQ(args, []any{int64(3)})

		query.stampTenant(WithTenant(context.Background(), int64(9)))
		_, args, err = query.ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.AssertEQ(args, []any{int64(9)})

		query.stampTenant(context.Background())
		_, _, err = query.ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "tenant value is required"), true)
		key, err = query.cacheKey("rows", "SELECT 1", nil)
		t.AssertNil(err)
		t.Assert(key, "user-by-id")

		updated := Update(user).Set(gooq.NewFieldAt[string](user, "name"), "a")
		updated.stampTenant(ctx)
		updated.stampTenant(context.Background())
		_, _, err = updated.ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "tenant value is required"), true)

		inner := Select(id).From(user)
		outer := Select(id).From(user).Where(id.InExpr(inner)).Tenant(int64(5))
		outer.stampTenant(ctx)
		_, args, err = outer.ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.AssertEQ(args, []any{int64(5), int64(5)})
	})
}

func TestDsl_Tenant_DML(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		user, id, name := tenantTable("user", gooq.LocalTypeInt64)
		sql, args, err := Update(user).Set(name, "a").Where(id.Eq(int64(1))).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "UPDATE `user` SET `name` = ? WHERE `user`.`id` = ? AND `user`.`tenant_id` = ?")
		t.AssertEQ(args, []any{"a", int64(1), int64(7)})

		_, _, err = Update(user).Set(user.Field("tenant_id"), int64(9)).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "must equal"), true)

		sql, _, err = Update(user).Set(user.Field("tenant_id"), int64(7)).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(strings.Contains(sql, "`tenant_id` = ?"), true)

		_, _, err = Update(user).Set(user.Field("tenant_id"), int64(7)).Tenant(int64(7), int64(8)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "multiple tenant values"), true)

		ctx := WithTenant(context.Background(), int64(4))
		updated := Update(user).Set(name, "b").Where(id.Eq(int64(1)))
		updated.stampTenant(ctx)
		sql, args, err = updated.ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(strings.Contains(sql, "`user`.`tenant_id` = ?"), true)
		t.AssertEQ(args, []any{"b", int64(1), int64(4)})

		order, orderID, _ := tenantTable("order", gooq.LocalTypeInt64)
		sql, _, err = Update(user).Set(name, "a").LeftJoin(order).On(orderID.EqExpr(id)).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(strings.Contains(sql, "LEFT JOIN `order` ON `order`.`id` = `user`.`id` AND `order`.`tenant_id` = ?"), true)
		t.Assert(strings.Contains(sql, "WHERE `user`.`tenant_id` = ?"), true)

		sql, args, err = Delete(user).Where(id.Eq(int64(1))).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "DELETE FROM `user` WHERE `user`.`id` = ? AND `user`.`tenant_id` = ?")
		t.AssertEQ(args, []any{int64(1), int64(7)})

		sql, args, err = Insert(user).Columns(name).Values("a").Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO `user` (`name`) VALUES (?)")
		t.AssertEQ(args, []any{"a"})

		sub := Select(name).From(user)
		sql, args, err = InsertFrom(user, sub).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(strings.Contains(sql, "WHERE `user`.`tenant_id` = ?"), true)
		t.AssertEQ(args, []any{int64(7)})

		_, _, err = Insert(user).Columns(name).Values("a").
			OnConflictKey(id).DoUpdate(user.Field("tenant_id"), int64(9)).
			Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "must equal"), true)

		sql, _, err = Insert(user).Columns(name).Values("a").
			OnConflictKey(id).DoNothing().
			Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(strings.Contains(sql, "INSERT"), true)
		t.Assert(strings.Contains(sql, "tenant_id"), false)

		batch, batchArgs, err := Update(user).Keys(id).ValuesOf(
			assignRow{setCol("id", int64(1)), setCol("name", "a")},
			assignRow{setCol("id", int64(2)), setCol("name", "b")},
		).Tenant(int64(7)).renderBatchDML(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(len(batch), 2)
		t.Assert(batch[0], "UPDATE `user` SET `name` = ? WHERE `id` = ? AND `user`.`tenant_id` = ?")
		t.AssertEQ(batchArgs[0], []any{"a", int64(1), int64(7)})

		stamped := Update(user).Keys(id).ValuesOf(
			assignRow{setCol("id", int64(1)), setCol("name", "a")},
			assignRow{setCol("id", int64(2)), setCol("name", "b")},
		)
		stamped.stampTenant(WithTenant(context.Background(), int64(6)))
		batch, batchArgs, err = stamped.renderBatchDML(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(batch[0], "UPDATE `user` SET `name` = ? WHERE `id` = ? AND `user`.`tenant_id` = ?")
		t.AssertEQ(batchArgs[0], []any{"a", int64(1), int64(6)})

		open := Update(user).Keys(id).ValuesOf(
			assignRow{setCol("id", int64(1)), setCol("name", "a")},
			assignRow{setCol("id", int64(2)), setCol("name", "b")},
		).UnTenant()
		batch, _, err = open.renderBatchDML(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(batch[0], "UPDATE `user` SET `name` = ? WHERE `id` = ?")

		_, _, err = Update(user).Keys(id).ValuesOf(
			assignRow{setCol("id", int64(1)), setCol("tenant_id", int64(9)), setCol("name", "a")},
			assignRow{setCol("id", int64(2)), setCol("name", "b")},
		).Tenant(int64(7)).renderBatchDML(gooq.DialectMySQL)
		t.AssertNE(err, nil)
	})
}

func TestDsl_Tenant_ValueTypes(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		base := gooq.NewTableBase(&gooq.TableMeta{
			TableName: "user",
			Fields: []gooq.FieldMeta{
				{ColumnName: "id", LocalType: gooq.LocalTypeInt64, Primary: true},
				{ColumnName: "tenant_id", LocalType: gooq.LocalTypeInt64Bytes, Tenant: true},
			},
		})
		id := gooq.NewFieldAt[int64](base, "id")
		_, _, err := Select(id).From(base).Tenant(int64(1)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		_, _, err = Select(id).From(base).Tenant(int(1)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)

		text := gooq.NewTableBase(&gooq.TableMeta{
			TableName: "user",
			Fields: []gooq.FieldMeta{
				{ColumnName: "id", LocalType: gooq.LocalTypeInt64, Primary: true},
				{ColumnName: "tenant_id", LocalType: gooq.LocalTypeUUID, Tenant: true},
			},
		})
		_, _, err = Select(gooq.NewFieldAt[int64](text, "id")).From(text).Tenant(uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		_, _, err = Select(gooq.NewFieldAt[int64](text, "id")).From(text).Tenant("6ba7b810-9dad-11d1-80b4-00c04fd430c8").ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)

		bad := gooq.NewTableBase(&gooq.TableMeta{
			TableName: "user",
			Fields: []gooq.FieldMeta{
				{ColumnName: "id", LocalType: gooq.LocalTypeInt64, Primary: true},
				{ColumnName: "tenant_id", LocalType: gooq.LocalTypeBool, Tenant: true},
			},
		})
		_, _, err = Select(gooq.NewFieldAt[int64](bad, "id")).From(bad).Tenant(true).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "unsupported type"), true)
	})
}
