// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件提供命名工具：字段名与文件名的 Go 惯例转换。
package gendao

import (
	"strings"

	"github.com/gogf/gf/v2/text/gstr"
)

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
