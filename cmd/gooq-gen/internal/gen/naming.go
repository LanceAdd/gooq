// 本文件提供字段名/文件名的 Go 惯例转换。
package gen

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/gogf/gf/v2/text/gstr"
)

// FieldNameCase 是字段命名风格。
type FieldNameCase string

const (
	// FieldNameCaseCamel 是首字母大写驼峰（Id → ID 由调用方按 Go 惯例调整）。
	FieldNameCaseCamel FieldNameCase = "CaseCamel"
)

// formatFieldName 把字段名格式化成生成 Go 代码使用的新名字。
func formatFieldName(fieldName string, nameCase FieldNameCase) string {
	// mysql、pgsql、sqlite 这类普通数据库，字段/表名是正常大小写。
	var newFieldName = fieldName
	if isAllUpper(fieldName) {
		// dm、oracle 这类特殊数据库，字段/表名是大写。
		newFieldName = strings.ToLower(fieldName)
	}
	switch nameCase {
	case FieldNameCaseCamel:
		return gstr.CaseCamel(newFieldName)
	default:
		return ""
	}
}

// formatFileName 把文件名格式化成生成源文件使用的新名字。
func formatFileName(fileName, nameCase string) string {
	if nameCase == "" {
		nameCase = string(gstr.Snake)
	}
	fileName = normalizeNameForCaseConvert(fileName)
	fileName = gstr.Trim(gstr.CaseConvert(fileName, gstr.CaseTypeMatch(nameCase)), "-_.")
	if len(fileName) > 5 && fileName[len(fileName)-5:] == "_test" {
		// 表名含 "_test" 时加后缀，否则生成文件会被当成测试文件。
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

// goFieldName 生成列的 Go 字段名：保留字加 Field 后缀，非法或撞名返回错误。
func goFieldName(column string, reserved map[string]bool, used map[string]string) (string, error) {
	name := formatFieldName(column, FieldNameCaseCamel)
	if reserved[name] {
		name += "Field"
	}
	if !validGoIdent(name) {
		return "", fmt.Errorf("gooq: column %q produces invalid Go name %q", column, name)
	}
	if prev, ok := used[name]; ok {
		return "", fmt.Errorf("gooq: columns %q and %q both generate field %s", prev, column, name)
	}
	used[name] = column
	return name, nil
}

func isSoftDeleteColumn(name string) bool {
	folded := strings.ReplaceAll(strings.ToLower(name), "_", "")
	return folded == "deletedat" || folded == "deleteat"
}

func validGoIdent(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 && !unicode.IsLetter(r) && r != '_' {
			return false
		}
		if i > 0 && !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}

func isAllUpper(fieldName string) bool {
	for _, b := range fieldName {
		if b >= 'a' && b <= 'z' {
			return false
		}
	}
	return true
}
