// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件为单元测试提供手写测试表对象（与 ggen 生成代码同构，不依赖数据库）。
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

func newTestUserTable() *testUserTable {
	t := &testUserTable{TableBase: NewTableBase(&TableMeta{
		TableName: "user",
		Fields: []FieldMeta{
			{ColumnName: "id", LocalType: LocalTypeInt64, Primary: true, AutoIncrement: true},
			{ColumnName: "name", LocalType: LocalTypeString},
			{ColumnName: "age", LocalType: LocalTypeInt},
			{ColumnName: "status", LocalType: LocalTypeString},
			{ColumnName: "created_at", LocalType: LocalTypeDatetime},
			{ColumnName: "deleted_at", LocalType: LocalTypeDatetime, SoftDelete: true},
		},
	})}
	t.ID = NewFieldAt[int64](t.TableBase, "id")
	t.Name = NewFieldAt[string](t.TableBase, "name")
	t.Age = NewFieldAt[int](t.TableBase, "age")
	t.Status = NewFieldAt[string](t.TableBase, "status")
	t.CreatedAt = NewFieldAt[time.Time](t.TableBase, "created_at")
	t.DeletedAt = NewFieldAt[time.Time](t.TableBase, "deleted_at")
	return t
}

var testUser = newTestUserTable()

func (t *testUserTable) As(alias string) *testUserTable {
	newT := *t
	newT.TableBase = t.TableBase.As(alias)
	newT.ID.BindTable(newT.TableBase)
	newT.Name.BindTable(newT.TableBase)
	newT.Age.BindTable(newT.TableBase)
	newT.Status.BindTable(newT.TableBase)
	newT.CreatedAt.BindTable(newT.TableBase)
	newT.DeletedAt.BindTable(newT.TableBase)
	return &newT
}

func (t *testUserTable) Clone() *testUserTable {
	newT := *t
	newT.TableBase = t.TableBase.Clone()
	newT.ID.BindTable(newT.TableBase)
	newT.Name.BindTable(newT.TableBase)
	newT.Age.BindTable(newT.TableBase)
	newT.Status.BindTable(newT.TableBase)
	newT.CreatedAt.BindTable(newT.TableBase)
	newT.DeletedAt.BindTable(newT.TableBase)
	return &newT
}

type testRoleTable struct {
	*TableBase
	ID        Field[int64]
	Name      Field[string]
	Remark    Field[string]
	DeletedAt Field[time.Time]
}

func newTestRoleTable() *testRoleTable {
	t := &testRoleTable{TableBase: NewTableBase(&TableMeta{
		TableName: "role",
		Fields: []FieldMeta{
			{ColumnName: "id", LocalType: LocalTypeInt64, Primary: true},
			{ColumnName: "name", LocalType: LocalTypeString, Unique: true},
			{ColumnName: "remark", LocalType: LocalTypeString},
			{ColumnName: "deleted_at", LocalType: LocalTypeDatetime, SoftDelete: true},
		},
	})}
	t.ID = NewFieldAt[int64](t.TableBase, "id")
	t.Name = NewFieldAt[string](t.TableBase, "name")
	t.Remark = NewFieldAt[string](t.TableBase, "remark")
	t.DeletedAt = NewFieldAt[time.Time](t.TableBase, "deleted_at")
	return t
}

var testRole = newTestRoleTable()

func (t *testRoleTable) As(alias string) *testRoleTable {
	newT := *t
	newT.TableBase = t.TableBase.As(alias)
	newT.ID.BindTable(newT.TableBase)
	newT.Name.BindTable(newT.TableBase)
	newT.Remark.BindTable(newT.TableBase)
	newT.DeletedAt.BindTable(newT.TableBase)
	return &newT
}

func (t *testRoleTable) Clone() *testRoleTable {
	newT := *t
	newT.TableBase = t.TableBase.Clone()
	newT.ID.BindTable(newT.TableBase)
	newT.Name.BindTable(newT.TableBase)
	newT.Remark.BindTable(newT.TableBase)
	newT.DeletedAt.BindTable(newT.TableBase)
	return &newT
}

type testUserRoleTable struct {
	*TableBase
	ID     Field[int64]
	UserID Field[int64]
	RoleID Field[int64]
}

func newTestUserRoleTable() *testUserRoleTable {
	t := &testUserRoleTable{TableBase: NewTableBase(&TableMeta{
		TableName: "user_role",
		Fields: []FieldMeta{
			{ColumnName: "id", LocalType: LocalTypeInt64, Primary: true},
			{ColumnName: "user_id", LocalType: LocalTypeInt64},
			{ColumnName: "role_id", LocalType: LocalTypeInt64},
		},
	})}
	t.ID = NewFieldAt[int64](t.TableBase, "id")
	t.UserID = NewFieldAt[int64](t.TableBase, "user_id")
	t.RoleID = NewFieldAt[int64](t.TableBase, "role_id")
	return t
}

var testUserRole = newTestUserRoleTable()

func (t *testUserRoleTable) As(alias string) *testUserRoleTable {
	newT := *t
	newT.TableBase = t.TableBase.As(alias)
	newT.ID.BindTable(newT.TableBase)
	newT.UserID.BindTable(newT.TableBase)
	newT.RoleID.BindTable(newT.TableBase)
	return &newT
}

func (t *testUserRoleTable) Clone() *testUserRoleTable {
	newT := *t
	newT.TableBase = t.TableBase.Clone()
	newT.ID.BindTable(newT.TableBase)
	newT.UserID.BindTable(newT.TableBase)
	newT.RoleID.BindTable(newT.TableBase)
	return &newT
}

// schemaUserTable 是带 schema 的最小测试表（schema 渲染专用）。
type schemaUserTable struct {
	*TableBase
	ID   Field[int64]
	Name Field[string]
}

func newSchemaUserTable() *schemaUserTable {
	t := &schemaUserTable{TableBase: NewTableBase(&TableMeta{
		TableName: "user",
		Schema:    "public",
		Fields: []FieldMeta{
			{ColumnName: "id", LocalType: LocalTypeInt64, Primary: true},
			{ColumnName: "name", LocalType: LocalTypeString},
		},
	})}
	t.ID = NewFieldAt[int64](t.TableBase, "id")
	t.Name = NewFieldAt[string](t.TableBase, "name")
	return t
}

func (t *schemaUserTable) As(alias string) *schemaUserTable {
	newT := *t
	newT.TableBase = t.TableBase.As(alias)
	newT.ID.BindTable(newT.TableBase)
	newT.Name.BindTable(newT.TableBase)
	return &newT
}

func (t *schemaUserTable) Clone() *schemaUserTable {
	newT := *t
	newT.TableBase = t.TableBase.Clone()
	newT.ID.BindTable(newT.TableBase)
	newT.Name.BindTable(newT.TableBase)
	return &newT
}
