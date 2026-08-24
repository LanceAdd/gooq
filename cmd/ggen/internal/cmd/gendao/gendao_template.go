// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件定义 do 与 entity 结构体的默认生成模板（gview 语法）。
package gendao

// 模板变量常量。
const (
	tplVarTableName          = `TplTableName`
	tplVarTableNameCamelCase = `TplTableNameCamelCase`
	tplVarPackageImports     = `TplPackageImports`
	tplVarStructDefine       = `TplStructDefine`
	tplVarPackageName        = `TplPackageName`
)

const TemplateGenDaoDoContent = `
// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT. {{.TplCreatedAtDatetimeStr}}
// =================================================================================

package {{.TplPackageName}}

{{.TplPackageImports}}

// {{.TplTableNameCamelCase}} is the golang structure of table {{.TplTableName}} for DAO operations like Where/Data.
{{.TplStructDefine}}
`

const TemplateGenDaoEntityContent = `
// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT. {{.TplCreatedAtDatetimeStr}}
// =================================================================================

package {{.TplPackageName}}

{{.TplPackageImports}}

// {{.TplTableNameCamelCase}} is the golang structure for table {{.TplTableName}}.
{{.TplStructDefine}}
`
