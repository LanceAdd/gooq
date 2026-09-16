// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件为单元测试提供手写测试表对象（与 gooq-gen 生成代码同构，不依赖数据库）。
package gooq

import (
	"time"
)

type testUserTable struct {
	*TableBase
	ID        Field[int64]
	Name      Field[string]
	Age       Field[int]
	Status    Field[string]
	CreatedAt Field[time.Time]
	DeletedAt Field[time.Time]
}

var testUserMeta = &TableMeta{
	TableName: "user",
	Fields: []FieldMeta{
		{ColumnName: "id", LocalType: LocalTypeInt64, Primary: true, AutoIncrement: true},
		{ColumnName: "name", LocalType: LocalTypeString},
		{ColumnName: "age", LocalType: LocalTypeInt},
		{ColumnName: "status", LocalType: LocalTypeString},
		{ColumnName: "created_at", LocalType: LocalTypeDatetime},
		{ColumnName: "deleted_at", LocalType: LocalTypeDatetime, SoftDelete: true},
	},
}

func newTestUserTable(alias string) *testUserTable {
	t := &testUserTable{TableBase: NewTableBase(testUserMeta)}
	if alias != "" {
		t.TableBase = t.TableBase.As(alias)
	}
	t.ID = NewFieldAt[int64](t.TableBase, "id")
	t.Name = NewFieldAt[string](t.TableBase, "name")
	t.Age = NewFieldAt[int](t.TableBase, "age")
	t.Status = NewFieldAt[string](t.TableBase, "status")
	t.CreatedAt = NewFieldAt[time.Time](t.TableBase, "created_at")
	t.DeletedAt = NewFieldAt[time.Time](t.TableBase, "deleted_at")
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
	field Expression
	alias string
}

func testCount(field Expression) *testFunc {
	return &testFunc{name: "COUNT", field: field}
}

func (f *testFunc) As(alias string) *testFunc {
	f.alias = alias
	return f
}

func (f *testFunc) Condition() (string, []any) {
	return f.Render(NewRenderContext(DialectMySQL))
}

func (f *testFunc) Render(rc *RenderContext) (string, []any) {
	fieldSQL, args := rc.Render(f.field)
	sql := f.name + "(" + fieldSQL + ")"
	if f.alias != "" {
		sql += " AS " + f.alias
	}
	return sql, args
}

func (f *testFunc) SubExpressions() []Expression {
	return []Expression{f.field}
}

type testRoleTable struct {
	*TableBase
	ID        Field[int64]
	Name      Field[string]
	Remark    Field[string]
	DeletedAt Field[time.Time]
}

var testRoleMeta = &TableMeta{
	TableName: "role",
	Fields: []FieldMeta{
		{ColumnName: "id", LocalType: LocalTypeInt64, Primary: true},
		{ColumnName: "name", LocalType: LocalTypeString, Unique: true},
		{ColumnName: "remark", LocalType: LocalTypeString},
		{ColumnName: "deleted_at", LocalType: LocalTypeDatetime, SoftDelete: true},
	},
}

func newTestRoleTable(alias string) *testRoleTable {
	t := &testRoleTable{TableBase: NewTableBase(testRoleMeta)}
	if alias != "" {
		t.TableBase = t.TableBase.As(alias)
	}
	t.ID = NewFieldAt[int64](t.TableBase, "id")
	t.Name = NewFieldAt[string](t.TableBase, "name")
	t.Remark = NewFieldAt[string](t.TableBase, "remark")
	t.DeletedAt = NewFieldAt[time.Time](t.TableBase, "deleted_at")
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
	*TableBase
	ID     Field[int64]
	UserID Field[int64]
	RoleID Field[int64]
}

var testUserRoleMeta = &TableMeta{
	TableName: "user_role",
	Fields: []FieldMeta{
		{ColumnName: "id", LocalType: LocalTypeInt64, Primary: true},
		{ColumnName: "user_id", LocalType: LocalTypeInt64},
		{ColumnName: "role_id", LocalType: LocalTypeInt64},
	},
}

func newTestUserRoleTable(alias string) *testUserRoleTable {
	t := &testUserRoleTable{TableBase: NewTableBase(testUserRoleMeta)}
	if alias != "" {
		t.TableBase = t.TableBase.As(alias)
	}
	t.ID = NewFieldAt[int64](t.TableBase, "id")
	t.UserID = NewFieldAt[int64](t.TableBase, "user_id")
	t.RoleID = NewFieldAt[int64](t.TableBase, "role_id")
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
	*TableBase
	ID   Field[int64]
	Name Field[string]
}

var schemaUserMeta = &TableMeta{
	TableName: "user",
	Schema:    "public",
	Fields: []FieldMeta{
		{ColumnName: "id", LocalType: LocalTypeInt64, Primary: true},
		{ColumnName: "name", LocalType: LocalTypeString},
	},
}

func newSchemaUserTable(alias string) *schemaUserTable {
	t := &schemaUserTable{TableBase: NewTableBase(schemaUserMeta)}
	if alias != "" {
		t.TableBase = t.TableBase.As(alias)
	}
	t.ID = NewFieldAt[int64](t.TableBase, "id")
	t.Name = NewFieldAt[string](t.TableBase, "name")
	return t
}

func (t *schemaUserTable) As(alias string) *schemaUserTable {
	return newSchemaUserTable(alias)
}

func (t *schemaUserTable) Clone() *schemaUserTable {
	return newSchemaUserTable("")
}
