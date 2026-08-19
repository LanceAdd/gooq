// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件提供示例表对象（user/order/product），对应 gen dao 生成的 table 包形态。
// 生产环境由 gf gen dao 生成同类代码。
package gooq

import "time"

// UserTable 是 user 表的类型化表对象。
type UserTable struct {
	*TableBase
	ALL_FIELDS AllFields

	ID        Field[int64]
	Name      Field[string]
	Age       Field[int]
	Status    Field[string]
	DeletedAt Field[*time.Time]
	CreatedAt Field[*time.Time]
}

// User 是 user 表的全局表对象。
var User = &UserTable{
	TableBase: NewTableBase(&TableMeta{
		TableName: "user",
		Fields: []FieldMeta{
			{ColumnName: "id", LocalType: LocalTypeInt64, Primary: true, AutoIncrement: true},
			{ColumnName: "name", LocalType: LocalTypeString},
			{ColumnName: "age", LocalType: LocalTypeInt},
			{ColumnName: "status", LocalType: LocalTypeString},
			{ColumnName: "deleted_at", LocalType: LocalTypeDatetime, SoftDelete: true},
			{ColumnName: "created_at", LocalType: LocalTypeDatetime},
		},
	}),
	ALL_FIELDS: NewAllFields([]string{"id", "name", "age", "status", "deleted_at", "created_at"}),

	ID:        NewField[int64]("user", "id"),
	Name:      NewField[string]("user", "name"),
	Age:       NewField[int]("user", "age"),
	Status:    NewField[string]("user", "status"),
	DeletedAt: NewField[*time.Time]("user", "deleted_at"),
	CreatedAt: NewField[*time.Time]("user", "created_at"),
}

// As 返回指定别名的表副本。
func (t *UserTable) As(alias string) *UserTable {
	newT := *t
	newT.TableBase = t.TableBase.As(alias)
	return &newT
}

// Clone 返回无别名表副本。
func (t *UserTable) Clone() *UserTable {
	newT := *t
	newT.TableBase = t.TableBase.Clone()
	return &newT
}

// OrderTable 是 order 表的类型化表对象（示例借用表）。
type OrderTable struct {
	*TableBase
	ALL_FIELDS AllFields

	ID        Field[int64]
	UserID    Field[int64]
	ProductID Field[int64]
	Amount    Field[int]
	DeletedAt Field[*time.Time]
}

// Order 是 order 表的全局表对象。
var Order = &OrderTable{
	TableBase: NewTableBase(&TableMeta{
		TableName: "order",
		Fields: []FieldMeta{
			{ColumnName: "id", LocalType: LocalTypeInt64, Primary: true, AutoIncrement: true},
			{ColumnName: "user_id", LocalType: LocalTypeInt64},
			{ColumnName: "product_id", LocalType: LocalTypeInt64},
			{ColumnName: "amount", LocalType: LocalTypeInt},
			{ColumnName: "deleted_at", LocalType: LocalTypeDatetime, SoftDelete: true},
		},
	}),
	ALL_FIELDS: NewAllFields([]string{"id", "user_id", "product_id", "amount", "deleted_at"}),

	ID:        NewField[int64]("order", "id"),
	UserID:    NewField[int64]("order", "user_id"),
	ProductID: NewField[int64]("order", "product_id"),
	Amount:    NewField[int]("order", "amount"),
	DeletedAt: NewField[*time.Time]("order", "deleted_at"),
}

// As 返回指定别名的表副本。
func (t *OrderTable) As(alias string) *OrderTable {
	newT := *t
	newT.TableBase = t.TableBase.As(alias)
	return &newT
}

// Clone 返回无别名表副本。
func (t *OrderTable) Clone() *OrderTable {
	newT := *t
	newT.TableBase = t.TableBase.Clone()
	return &newT
}

// ProductTable 是 product 表的类型化表对象（示例借用表）。
type ProductTable struct {
	*TableBase
	ALL_FIELDS AllFields

	ID       Field[int64]
	ParentID Field[int64]
	Name     Field[string]
}

// Product 是 product 表的全局表对象。
var Product = &ProductTable{
	TableBase: NewTableBase(&TableMeta{
		TableName: "product",
		Fields: []FieldMeta{
			{ColumnName: "id", LocalType: LocalTypeInt64, Primary: true, AutoIncrement: true},
			{ColumnName: "parent_id", LocalType: LocalTypeInt64},
			{ColumnName: "name", LocalType: LocalTypeString},
		},
	}),
	ALL_FIELDS: NewAllFields([]string{"id", "parent_id", "name"}),

	ID:       NewField[int64]("product", "id"),
	ParentID: NewField[int64]("product", "parent_id"),
	Name:     NewField[string]("product", "name"),
}

// As 返回指定别名的表副本。
func (t *ProductTable) As(alias string) *ProductTable {
	newT := *t
	newT.TableBase = t.TableBase.As(alias)
	return &newT
}

// Clone 返回无别名表副本。
func (t *ProductTable) Clone() *ProductTable {
	newT := *t
	newT.TableBase = t.TableBase.Clone()
	return &newT
}
