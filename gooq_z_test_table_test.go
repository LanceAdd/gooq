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

var testUser = &testUserTable{
	TableBase: NewTableBase(&TableMeta{
		TableName: "user",
		Fields: []FieldMeta{
			{ColumnName: "id", LocalType: LocalTypeInt64, Primary: true, AutoIncrement: true},
			{ColumnName: "name", LocalType: LocalTypeString},
			{ColumnName: "age", LocalType: LocalTypeInt},
			{ColumnName: "status", LocalType: LocalTypeString},
			{ColumnName: "created_at", LocalType: LocalTypeDatetime},
			{ColumnName: "deleted_at", LocalType: LocalTypeDatetime, SoftDelete: true},
		},
	}),
	ID:        NewField[int64]("user", "id"),
	Name:      NewField[string]("user", "name"),
	Age:       NewField[int]("user", "age"),
	Status:    NewField[string]("user", "status"),
	CreatedAt: NewField[time.Time]("user", "created_at"),
	DeletedAt: NewField[time.Time]("user", "deleted_at"),
}

func (t *testUserTable) As(alias string) *testUserTable {
	newT := *t
	newT.TableBase = t.TableBase.As(alias)
	return &newT
}

func (t *testUserTable) Clone() *testUserTable {
	newT := *t
	newT.TableBase = t.TableBase.Clone()
	return &newT
}

type testRoleTable struct {
	*TableBase
	ID        Field[int64]
	Name      Field[string]
	Remark    Field[string]
	DeletedAt Field[time.Time]
}

var testRole = &testRoleTable{
	TableBase: NewTableBase(&TableMeta{
		TableName: "role",
		Fields: []FieldMeta{
			{ColumnName: "id", LocalType: LocalTypeInt64, Primary: true},
			{ColumnName: "name", LocalType: LocalTypeString, Unique: true},
			{ColumnName: "remark", LocalType: LocalTypeString},
			{ColumnName: "deleted_at", LocalType: LocalTypeDatetime, SoftDelete: true},
		},
	}),
	ID:        NewField[int64]("role", "id"),
	Name:      NewField[string]("role", "name"),
	Remark:    NewField[string]("role", "remark"),
	DeletedAt: NewField[time.Time]("role", "deleted_at"),
}

func (t *testRoleTable) As(alias string) *testRoleTable {
	newT := *t
	newT.TableBase = t.TableBase.As(alias)
	return &newT
}

func (t *testRoleTable) Clone() *testRoleTable {
	newT := *t
	newT.TableBase = t.TableBase.Clone()
	return &newT
}

type testUserRoleTable struct {
	*TableBase
	ID     Field[int64]
	UserID Field[int64]
	RoleID Field[int64]
}

var testUserRole = &testUserRoleTable{
	TableBase: NewTableBase(&TableMeta{
		TableName: "user_role",
		Fields: []FieldMeta{
			{ColumnName: "id", LocalType: LocalTypeInt64, Primary: true},
			{ColumnName: "user_id", LocalType: LocalTypeInt64},
			{ColumnName: "role_id", LocalType: LocalTypeInt64},
		},
	}),
	ID:     NewField[int64]("user_role", "id"),
	UserID: NewField[int64]("user_role", "user_id"),
	RoleID: NewField[int64]("user_role", "role_id"),
}

func (t *testUserRoleTable) As(alias string) *testUserRoleTable {
	newT := *t
	newT.TableBase = t.TableBase.As(alias)
	return &newT
}

func (t *testUserRoleTable) Clone() *testUserRoleTable {
	newT := *t
	newT.TableBase = t.TableBase.Clone()
	return &newT
}
