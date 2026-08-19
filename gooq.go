// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// Package gooq provides a typed SQL query DSL:
// typed fields, composable conditions, offline SQL rendering and dialect-aware operators.
//
// gooq 定位为纯 SQL 构建器：DSL 类型安全地构建 SQL（Select/From/条件/子查询/函数/离线渲染），
// 不依赖任何数据库实例；生成的 SQL 可由调用方自选执行方式（标准库 database/sql 等）。
package gooq

import (
	"github.com/gogf/gf/v2/container/gvar"
)

type (
	// Value is the field value type.
	Value = *gvar.Var

	// Array is the field value array type.
	Array = gvar.Vars

	// Record is the row record of the table.
	Record map[string]Value

	// Result is the row record array.
	Result []Record

	// Map is alias of map[string]any, which is the most common usage map type.
	Map = map[string]any

	// List is type of map array.
	List = []Map
)

// LocalType is a type that defines the local storage type of a field value.
// It is used to specify how the field value should be processed locally.
type LocalType string

const (
	LocalTypeUndefined    LocalType = ""
	LocalTypeString       LocalType = "string"
	LocalTypeTime         LocalType = "time"
	LocalTypeDate         LocalType = "date"
	LocalTypeDatetime     LocalType = "datetime"
	LocalTypeInt          LocalType = "int"
	LocalTypeUint         LocalType = "uint"
	LocalTypeInt32        LocalType = "int32"
	LocalTypeUint32       LocalType = "uint32"
	LocalTypeInt64        LocalType = "int64"
	LocalTypeUint64       LocalType = "uint64"
	LocalTypeBigInt       LocalType = "bigint"
	LocalTypeIntSlice     LocalType = "[]int"
	LocalTypeUintSlice    LocalType = "[]uint"
	LocalTypeInt32Slice   LocalType = "[]int32"
	LocalTypeUint32Slice  LocalType = "[]uint32"
	LocalTypeInt64Slice   LocalType = "[]int64"
	LocalTypeUint64Slice  LocalType = "[]uint64"
	LocalTypeStringSlice  LocalType = "[]string"
	LocalTypeInt64Bytes   LocalType = "int64-bytes"
	LocalTypeUint64Bytes  LocalType = "uint64-bytes"
	LocalTypeFloat32      LocalType = "float32"
	LocalTypeFloat64      LocalType = "float64"
	LocalTypeFloat32Slice LocalType = "[]float32"
	LocalTypeFloat64Slice LocalType = "[]float64"
	LocalTypeBytes        LocalType = "[]byte"
	LocalTypeBytesSlice   LocalType = "[][]byte"
	LocalTypeBool         LocalType = "bool"
	LocalTypeBoolSlice    LocalType = "[]bool"
	LocalTypeJson         LocalType = "json"
	LocalTypeJsonb        LocalType = "jsonb"
	LocalTypeUUID         LocalType = "uuid.UUID"
	LocalTypeUUIDSlice    LocalType = "[]uuid.UUID"
)
