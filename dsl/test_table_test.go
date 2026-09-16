// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件为单元测试提供手写测试表对象（与 gooq-gen 生成代码同构，不依赖数据库）。
package dsl

import (
	"github.com/lanceadd/gooq"
	"time"
)

type testUserTable struct {
	*gooq.TableBase
	ID        gooq.Field[int64]
	Name      gooq.Field[string]
	Age       gooq.Field[int]
	Status    gooq.Field[string]
	CreatedAt gooq.Field[time.Time]
	DeletedAt gooq.Field[time.Time]
}

var testUserMeta = &gooq.TableMeta{
	TableName: "user",
	Fields: []gooq.FieldMeta{
		{ColumnName: "id", LocalType: gooq.LocalTypeInt64, Primary: true, AutoIncrement: true},
		{ColumnName: "name", LocalType: gooq.LocalTypeString},
		{ColumnName: "age", LocalType: gooq.LocalTypeInt},
		{ColumnName: "status", LocalType: gooq.LocalTypeString},
		{ColumnName: "created_at", LocalType: gooq.LocalTypeDatetime},
		{ColumnName: "deleted_at", LocalType: gooq.LocalTypeDatetime, SoftDelete: true},
	},
}

func newTestUserTable(alias string) *testUserTable {
	t := &testUserTable{TableBase: gooq.NewTableBase(testUserMeta)}
	if alias != "" {
		t.TableBase = t.TableBase.As(alias)
	}
	t.ID = gooq.NewFieldAt[int64](t.TableBase, "id")
	t.Name = gooq.NewFieldAt[string](t.TableBase, "name")
	t.Age = gooq.NewFieldAt[int](t.TableBase, "age")
	t.Status = gooq.NewFieldAt[string](t.TableBase, "status")
	t.CreatedAt = gooq.NewFieldAt[time.Time](t.TableBase, "created_at")
	t.DeletedAt = gooq.NewFieldAt[time.Time](t.TableBase, "deleted_at")
	return t
}

var testUser = newTestUserTable("")

func (t *testUserTable) As(alias string) *testUserTable {
	return newTestUserTable(alias)
}

func (t *testUserTable) Clone() *testUserTable {
	return newTestUserTable("")
}

// testFunc 是测试用的最小函数节点（渲染 NAME(field)）。
type testFunc struct {
	name  string
	field gooq.Expression
	alias string
}

func testCount(field gooq.Expression) *testFunc {
	return &testFunc{name: "COUNT", field: field}
}

func (f *testFunc) As(alias string) *testFunc {
	f.alias = alias
	return f
}

func (f *testFunc) Condition() (string, []any) {
	return f.Render(gooq.NewRenderContext(gooq.DialectMySQL))
}

func (f *testFunc) Render(rc *gooq.RenderContext) (string, []any) {
	fieldSQL, args := rc.Render(f.field)
	sql := f.name + "(" + fieldSQL + ")"
	if f.alias != "" {
		sql += " AS " + f.alias
	}
	return sql, args
}

func (f *testFunc) SubExpressions() []gooq.Expression {
	return []gooq.Expression{f.field}
}

type testRoleTable struct {
	*gooq.TableBase
	ID        gooq.Field[int64]
	Name      gooq.Field[string]
	Remark    gooq.Field[string]
	DeletedAt gooq.Field[time.Time]
}

var testRoleMeta = &gooq.TableMeta{
	TableName: "role",
	Fields: []gooq.FieldMeta{
		{ColumnName: "id", LocalType: gooq.LocalTypeInt64, Primary: true},
		{ColumnName: "name", LocalType: gooq.LocalTypeString, Unique: true},
		{ColumnName: "remark", LocalType: gooq.LocalTypeString},
		{ColumnName: "deleted_at", LocalType: gooq.LocalTypeDatetime, SoftDelete: true},
	},
}

func newTestRoleTable(alias string) *testRoleTable {
	t := &testRoleTable{TableBase: gooq.NewTableBase(testRoleMeta)}
	if alias != "" {
		t.TableBase = t.TableBase.As(alias)
	}
	t.ID = gooq.NewFieldAt[int64](t.TableBase, "id")
	t.Name = gooq.NewFieldAt[string](t.TableBase, "name")
	t.Remark = gooq.NewFieldAt[string](t.TableBase, "remark")
	t.DeletedAt = gooq.NewFieldAt[time.Time](t.TableBase, "deleted_at")
	return t
}

var testRole = newTestRoleTable("")

func (t *testRoleTable) As(alias string) *testRoleTable {
	return newTestRoleTable(alias)
}

func (t *testRoleTable) Clone() *testRoleTable {
	return newTestRoleTable("")
}

type testUserRoleTable struct {
	*gooq.TableBase
	ID     gooq.Field[int64]
	UserID gooq.Field[int64]
	RoleID gooq.Field[int64]
}

var testUserRoleMeta = &gooq.TableMeta{
	TableName: "user_role",
	Fields: []gooq.FieldMeta{
		{ColumnName: "id", LocalType: gooq.LocalTypeInt64, Primary: true},
		{ColumnName: "user_id", LocalType: gooq.LocalTypeInt64},
		{ColumnName: "role_id", LocalType: gooq.LocalTypeInt64},
	},
}

func newTestUserRoleTable(alias string) *testUserRoleTable {
	t := &testUserRoleTable{TableBase: gooq.NewTableBase(testUserRoleMeta)}
	if alias != "" {
		t.TableBase = t.TableBase.As(alias)
	}
	t.ID = gooq.NewFieldAt[int64](t.TableBase, "id")
	t.UserID = gooq.NewFieldAt[int64](t.TableBase, "user_id")
	t.RoleID = gooq.NewFieldAt[int64](t.TableBase, "role_id")
	return t
}

var testUserRole = newTestUserRoleTable("")

func (t *testUserRoleTable) As(alias string) *testUserRoleTable {
	return newTestUserRoleTable(alias)
}

func (t *testUserRoleTable) Clone() *testUserRoleTable {
	return newTestUserRoleTable("")
}

// schemaUserTable 是带 schema 的最小测试表（schema 渲染专用）。
type schemaUserTable struct {
	*gooq.TableBase
	ID   gooq.Field[int64]
	Name gooq.Field[string]
}

var schemaUserMeta = &gooq.TableMeta{
	TableName: "user",
	Schema:    "public",
	Fields: []gooq.FieldMeta{
		{ColumnName: "id", LocalType: gooq.LocalTypeInt64, Primary: true},
		{ColumnName: "name", LocalType: gooq.LocalTypeString},
	},
}

func newSchemaUserTable(alias string) *schemaUserTable {
	t := &schemaUserTable{TableBase: gooq.NewTableBase(schemaUserMeta)}
	if alias != "" {
		t.TableBase = t.TableBase.As(alias)
	}
	t.ID = gooq.NewFieldAt[int64](t.TableBase, "id")
	t.Name = gooq.NewFieldAt[string](t.TableBase, "name")
	return t
}

func (t *schemaUserTable) As(alias string) *schemaUserTable {
	return newSchemaUserTable(alias)
}

func (t *schemaUserTable) Clone() *schemaUserTable {
	return newSchemaUserTable("")
}
