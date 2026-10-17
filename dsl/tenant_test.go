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
		t.Assert(sql, "SELECT `user`.`id`, `user`.`name` FROM `user` WHERE (`user`.`name` = ?) AND `user`.`tenant_id` = ?")
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
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE (`user`.`tenant_id` = ?) AND `user`.`tenant_id` = ?")
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
		t.Assert(sql, "SELECT `user`.`id` FROM `user` LEFT JOIN `order` ON (`order`.`id` = `user`.`id`) AND `order`.`tenant_id` = ? WHERE `user`.`tenant_id` = ?")
		t.AssertEQ(args, []any{int64(7), int64(7)})

		sql, _, err = Select(userID).From(user).
			InnerJoin(order).On(orderID.EqExpr(userID)).
			Tenant(int64(7)).
			ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` INNER JOIN `order` ON (`order`.`id` = `user`.`id`) WHERE `user`.`tenant_id` = ? AND `order`.`tenant_id` = ?")

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
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE (`user`.`id` IN (SELECT `order`.`id` FROM `order` WHERE `order`.`tenant_id` = ?)) AND `user`.`tenant_id` = ?")
		t.AssertEQ(args, []any{int64(5), int64(5)})

		inner = Select(orderID).From(order).UnTenant()
		sql, args, err = Select(userID).From(user).Where(userID.InExpr(inner)).Tenant(int64(5)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE (`user`.`id` IN (SELECT `order`.`id` FROM `order`)) AND `user`.`tenant_id` = ?")
		t.AssertEQ(args, []any{int64(5)})

		ctx := WithTenant(context.Background(), int64(8))
		outer := Select(userID).From(user).Where(userID.InExpr(Select(orderID).From(order))).UnTenant()
		outer.stampTenant(ctx)
		sql, args, err = outer.ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE (`user`.`id` IN (SELECT `order`.`id` FROM `order` WHERE `order`.`tenant_id` = ?))")
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
		t.Assert(sql, "UPDATE `user` SET `name` = ? WHERE (`user`.`id` = ?) AND `user`.`tenant_id` = ?")
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
		t.Assert(strings.Contains(sql, "LEFT JOIN `order` ON (`order`.`id` = `user`.`id`) AND `order`.`tenant_id` = ?"), true)
		t.Assert(strings.Contains(sql, "WHERE `user`.`tenant_id` = ?"), true)

		sql, args, err = Delete(user).Where(id.Eq(int64(1))).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "DELETE FROM `user` WHERE (`user`.`id` = ?) AND `user`.`tenant_id` = ?")
		t.AssertEQ(args, []any{int64(1), int64(7)})

		_, _, err = Insert(user).Columns(name).Values("a").Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "must set tenant column"), true)

		tenantCol := user.Field("tenant_id")
		sql, args, err = Insert(user).Columns(name, tenantCol).Values("a", int64(7)).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO `user` (`name`, `tenant_id`) VALUES (?, ?)")
		t.AssertEQ(args, []any{"a", int64(7)})

		sub := Select(name).From(user)
		_, _, err = InsertFrom(user, sub).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "must set tenant column"), true)

		sub = Select(user.Field("tenant_id"), name).From(user)
		sql, args, err = InsertFrom(user, sub).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(strings.Contains(sql, "WHERE `user`.`tenant_id` = ?"), true)
		t.AssertEQ(args, []any{int64(7)})

		_, _, err = Insert(user).Columns(name, user.Field("tenant_id")).Values("a", int64(7)).
			OnConflictKey(id).DoUpdate(user.Field("tenant_id"), int64(9)).
			Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "must equal"), true)

		_, _, err = Insert(user).Columns(name).Values("a").
			OnConflictKey(id).DoNothing().
			Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "must set tenant column"), true)

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

func plainTable(name string) *gooq.TableBase {
	return gooq.NewTableBase(&gooq.TableMeta{
		TableName: name,
		Fields: []gooq.FieldMeta{
			{ColumnName: "id", LocalType: gooq.LocalTypeInt64, Primary: true},
			{ColumnName: "name", LocalType: gooq.LocalTypeString},
		},
	})
}

func TestDsl_Tenant_RightAndFullJoin(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		user, userID, _ := tenantTable("user", gooq.LocalTypeInt64)
		order, orderID, _ := tenantTable("order", gooq.LocalTypeInt64)
		dict := plainTable("dict")
		pay := plainTable("pay")
		dictID := gooq.NewFieldAt[int64](dict, "id")
		payID := gooq.NewFieldAt[int64](pay, "id")

		_, _, err := Select(userID).From(user).RightJoin(dict).On(dictID.EqExpr(userID)).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), `tenant table "user"`), true)

		_, _, err = Select(orderID).From(dict).RightJoin(order).On(orderID.EqExpr(dictID)).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), `tenant table "order"`), true)

		_, _, err = Select(payID).From(dict).InnerJoin(order).On(orderID.EqExpr(dictID)).RightJoin(pay).On(payID.EqExpr(dictID)).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), `tenant table "order"`), true)
		t.Assert(strings.Contains(err.Error(), "RIGHT JOIN"), true)

		_, _, err = Select(userID).From(user).FullJoin(dict).On(dictID.EqExpr(userID)).Tenant(int64(7)).ToSql(gooq.DialectPgsql)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "FULL JOIN"), true)

		_, _, err = Select(dictID).From(dict).FullJoin(order).On(orderID.EqExpr(dictID)).Tenant(int64(7)).ToSql(gooq.DialectPgsql)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), `tenant table "order"`), true)

		sql, _, err := Select(dictID).From(dict).RightJoin(pay).On(payID.EqExpr(dictID)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(strings.Contains(sql, "RIGHT JOIN `pay`"), true)
		t.Assert(strings.Contains(sql, "tenant_id"), false)

		sql, _, err = Select(userID).From(user).RightJoin(dict).On(dictID.EqExpr(userID)).UnTenant().ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(strings.Contains(sql, "RIGHT JOIN `dict`"), true)
		t.Assert(strings.Contains(sql, "tenant_id"), false)

		_, _, err = Update(user).Set(gooq.NewFieldAt[string](user, "name"), "a").RightJoin(dict).On(dictID.EqExpr(userID)).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "RIGHT JOIN"), true)

		derived := Select(orderID).From(order).As("o")
		sql, args, err := Update(dict).Set(gooq.NewFieldAt[string](dict, "name"), "a").RightJoin(derived).On(derived.Field("id").Eq(dictID)).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(strings.Contains(sql, "RIGHT JOIN (SELECT"), true)
		t.Assert(strings.Contains(sql, "`order`.`tenant_id` = ?"), true)
		t.AssertEQ(args, []any{"a", int64(7)})
	})
}

func TestDsl_Tenant_ParenthesesAndOrder(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		user, id, _ := tenantTable("user", gooq.LocalTypeInt64)
		order, _, _ := tenantTable("order", gooq.LocalTypeInt64)

		sql, _, err := Select(id).From(user).Where(gooq.Raw("`user`.`id` = 1 OR `user`.`id` = 2")).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user` WHERE (`user`.`id` = 1 OR `user`.`id` = 2) AND `user`.`tenant_id` = ?")

		sql, _, err = Select(id).From(user).LeftJoin(order).On(gooq.Raw("`order`.`id` = `user`.`id` OR `order`.`id` = 0")).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(strings.Contains(sql, "ON (`order`.`id` = `user`.`id` OR `order`.`id` = 0) AND `order`.`tenant_id` = ?"), true)

		sql, _, err = Update(user).Set(gooq.NewFieldAt[string](user, "name"), "a").Where(gooq.Raw("`user`.`id` = 1 OR `user`.`id` = 2")).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(strings.Contains(sql, "WHERE (`user`.`id` = 1 OR `user`.`id` = 2) AND `user`.`tenant_id` = ?"), true)

		sql, _, err = Select(id).From(user).UnTenant().Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(strings.Contains(sql, "`user`.`tenant_id` = ?"), true)

		sql, _, err = Select(id).From(user).Tenant(int64(7)).UnTenant().ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "SELECT `user`.`id` FROM `user`")
	})
}

func TestDsl_Tenant_CountInheritsContext(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		user, userID, _ := tenantTable("user", gooq.LocalTypeInt64)
		order, orderID, _ := tenantTable("order", gooq.LocalTypeInt64)
		ctx := WithTenant(context.Background(), int64(8))
		grouped := Select(userID).From(user).Where(userID.InExpr(Select(orderID).From(order))).Group(userID).UnTenant()
		grouped.stampTenant(ctx)

		sql, args, err := grouped.ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(strings.Contains(sql, "`order`.`tenant_id` = ?"), true)
		t.AssertEQ(args, []any{int64(8)})

		countSQL, countArgs, err := grouped.countSQL(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(strings.Contains(countSQL, "`order`.`tenant_id` = ?"), true)
		t.AssertEQ(countArgs, []any{int64(8)})

		existsSQL, existsArgs, err := grouped.existsSQL(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(strings.Contains(existsSQL, "`order`.`tenant_id` = ?"), true)
		t.AssertEQ(existsArgs, []any{int64(8)})

		_, err = grouped.compositeCacheKey(gooq.DialectMySQL, "rows")
		t.AssertNil(err)

		inner := Select(orderID).From(order)
		inner.stampTenant(ctx)
		outer := Select(userID).From(user).Where(userID.InExpr(inner)).UnTenant()
		_, _, err = outer.ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "tenant value is required"), true)
	})
}

func TestDsl_Tenant_InsertRules(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		user, _, name := tenantTable("user", gooq.LocalTypeInt64)
		tenantCol := user.Field("tenant_id")

		_, _, err := Insert(user).ValuesOf(assignRow{setCol("name", "a"), nullCol("tenant_id")}).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "cannot be NULL"), true)

		_, _, err = Insert(user).Columns(name, tenantCol).Values("a", gooq.Raw("7")).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "current tenant value"), true)

		_, _, err = Insert(user).Columns(name, tenantCol).Values("a", int(7)).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "expects int64"), true)

		_, _, err = Insert(user).Columns(name, tenantCol).Values("a", int64(9)).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "must equal"), true)

		_, _, err = Insert(user).Columns(name, tenantCol).Values("a", int64(7)).Values("b", int64(8)).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "must equal"), true)

		_, _, err = Insert(user).Columns(name, tenantCol).Values("a", int64(7)).Tenant(int64(7), int64(8)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "single tenant value"), true)

		_, _, err = Insert(user).Columns(name, tenantCol).Values("a", int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "tenant value is required"), true)

		sql, _, err := Insert(user).Columns(name).Values("a").UnTenant().ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(sql, "INSERT INTO `user` (`name`) VALUES (?)")

		_, _, err = InsertFrom(user, Select().From(user)).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "named column"), true)

		_, _, err = InsertFrom(user, Select(gooq.Raw("tenant_id"), name).From(user)).Tenant(int64(7)).ToSql(gooq.DialectMySQL)
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "named column"), true)

		sql, _, err = InsertFrom(user, Select(name).From(user).UnTenant()).UnTenant().ToSql(gooq.DialectMySQL)
		t.AssertNil(err)
		t.Assert(strings.Contains(sql, "INSERT INTO"), true)
	})
}
