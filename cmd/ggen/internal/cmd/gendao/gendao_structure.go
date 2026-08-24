// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件提供生成工具：结构体渲染、类型推导与字段名/文件名的 Go 惯例转换。
package gendao

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/container/garray"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gview"
	"github.com/gogf/gf/v2/text/gstr"

	"github.com/gogf/gf/cmd/ggen/internal/mlog"
)

// generateStructDefinition generates and returns the struct definition content for given table.
// isDo 为 true 时输出 do 结构体（g.Meta 标记、字段类型统一为 any），否则输出 entity 结构体（json/orm tag）。
func generateStructDefinition(
	ctx context.Context, db gdb.DB, fieldMap map[string]*gdb.TableField, structName, tableName string, isDo bool,
) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "type %s struct {\n", structName)
	if isDo {
		fmt.Fprintf(&builder, "\tg.Meta `orm:\"table:%s, do:true\"`\n", tableName)
	}
	for _, name := range sortFieldKeyForDao(fieldMap) {
		field := fieldMap[name]
		fieldName := formatFieldName(field.Name, FieldNameCaseCamel)
		if isDo {
			fmt.Fprintf(&builder, "\t%s any // %s\n", fieldName, formatComment(field.Comment))
		} else {
			jsonTag := gstr.CaseConvert(field.Name, gstr.CaseTypeMatch("CamelLower"))
			fmt.Fprintf(
				&builder,
				"\t%s %s `json:\"%s\" orm:\"%s\"` // %s\n",
				fieldName, structFieldType(ctx, db, field), jsonTag, field.Name, formatComment(field.Comment),
			)
		}
	}
	builder.WriteString("}")
	return builder.String()
}

// structFieldType converts the database field type to the local Go type.
func structFieldType(ctx context.Context, db gdb.DB, field *gdb.TableField) string {
	localTypeName, err := db.CheckLocalTypeForField(ctx, field.Type, nil)
	if err != nil {
		mlog.Fatalf(`check local type for field "%s" failed: %+v`, field.Name, err)
	}
	switch localTypeName {
	case gdb.LocalTypeDate, gdb.LocalTypeTime, gdb.LocalTypeDatetime:
		// 统一使用 stdlib time.Time。
		return "time.Time"
	case gdb.LocalTypeInt64Bytes:
		return "int64"
	case gdb.LocalTypeUint64Bytes:
		return "uint64"
	case gdb.LocalTypeJson, gdb.LocalTypeJsonb:
		return "string"
	case gdb.LocalTypeUUID:
		return "uuid.UUID"
	default:
		return string(localTypeName)
	}
}

// renderStructContent renders the struct content with the given template.
func renderStructContent(
	tplContent, packageName, tableName, tableNameCamelCase, structDefine string, isDo bool,
) string {
	tplView.ClearAssigns()
	tplView.Assigns(gview.Params{
		tplVarTableName:          tableName,
		tplVarPackageImports:     getImportPartContent(structDefine, isDo),
		tplVarTableNameCamelCase: tableNameCamelCase,
		tplVarStructDefine:       structDefine,
		tplVarPackageName:        packageName,
	})
	content, err := tplView.ParseContent(context.Background(), tplContent)
	if err != nil {
		mlog.Fatalf("parsing template content failed: %v", err)
	}
	return content
}

// getImportPartContent generates and returns the import content for generated struct files.
func getImportPartContent(source string, isDo bool) string {
	var packageImportsArray = garray.NewStrArray()
	if isDo {
		packageImportsArray.Append(`"github.com/gogf/gf/v2/frame/g"`)
	}
	// Time package recognition.
	if strings.Contains(source, "time.Time") {
		packageImportsArray.Append(`"time"`)
	}
	// UUID type.
	if strings.Contains(source, "uuid.UUID") {
		packageImportsArray.Append(`"github.com/google/uuid"`)
	}
	// Generate and write content to golang file.
	packageImportsStr := ""
	if packageImportsArray.Len() > 0 {
		packageImportsStr = fmt.Sprintf("import(\n%s\n)", packageImportsArray.Join("\n"))
	}
	return packageImportsStr
}

// sortFieldKeyForDao sorts the field map by field index.
func sortFieldKeyForDao(fieldMap map[string]*gdb.TableField) []string {
	names := make(map[int]string)
	for _, field := range fieldMap {
		names[field.Index] = field.Name
	}
	var (
		i      = 0
		j      = 0
		result = make([]string, len(names))
	)
	for {
		if len(names) == 0 {
			break
		}
		if value, ok := names[i]; ok {
			result[j] = value
			j++
			delete(names, i)
		}
		i++
	}
	return result
}

// formatComment formats the comment string to fit the golang code without any lines.
func formatComment(comment string) string {
	comment = gstr.ReplaceByArray(comment, g.SliceStr{
		"\n", " ",
		"\r", " ",
	})
	comment = gstr.Replace(comment, `\n`, " ")
	comment = gstr.Trim(comment)
	return comment
}

// FieldNameCase 是字段命名风格。
type FieldNameCase string

const (
	// FieldNameCaseCamel 是首字母大写驼峰（Id → ID 由调用方按 Go 惯例调整）。
	FieldNameCaseCamel FieldNameCase = "CaseCamel"
	// FieldNameCaseCamelLower 是首字母小写驼峰。
	FieldNameCaseCamelLower FieldNameCase = "CaseCamelLower"
)

// formatFieldName formats and returns a new field name that is used for golang codes generating.
func formatFieldName(fieldName string, nameCase FieldNameCase) string {
	// For normal databases like mysql, pgsql, sqlite,
	// field/table names of that are in normal case.
	var newFieldName = fieldName
	if isAllUpper(fieldName) {
		// For special databases like dm, oracle,
		// field/table names of that are in upper case.
		newFieldName = strings.ToLower(fieldName)
	}
	switch nameCase {
	case FieldNameCaseCamel:
		return gstr.CaseCamel(newFieldName)
	case FieldNameCaseCamelLower:
		return gstr.CaseCamelLower(newFieldName)
	default:
		return ""
	}
}

// formatFileName formats and returns a new file name for generated source files.
func formatFileName(fileName, nameCase string) string {
	if nameCase == "" {
		nameCase = string(gstr.Snake)
	}
	fileName = normalizeNameForCaseConvert(fileName)
	fileName = gstr.Trim(gstr.CaseConvert(fileName, gstr.CaseTypeMatch(nameCase)), "-_.")
	if len(fileName) > 5 && fileName[len(fileName)-5:] == "_test" {
		// Add suffix to avoid the table name which contains "_test",
		// which would make the go file a testing file.
		fileName += "_table"
	}
	return fileName
}

func normalizeNameForCaseConvert(name string) string {
	if isAllUpper(name) {
		return strings.ToLower(name)
	}
	return name
}

// isAllUpper checks and returns whether given `fieldName` all letters are upper case.
func isAllUpper(fieldName string) bool {
	for _, b := range fieldName {
		if b >= 'a' && b <= 'z' {
			return false
		}
	}
	return true
}
