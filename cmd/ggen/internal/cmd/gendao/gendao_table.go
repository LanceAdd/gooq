// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件实现 gooq 类型化表对象的生成：连库取表结构 → 模板渲染 → 写盘。
// 模板为外部文件（template/table.tmpl）。
package gendao

import (
	"context"
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gview"
	"github.com/gogf/gf/v2/text/gstr"

	"github.com/gogf/gf/cmd/ggen/internal/mlog"
)

//go:embed template/*
var gooqTemplateFS embed.FS

// generateTable generates gooq typed table object files for given tables.
func generateTable(ctx context.Context, db gdb.DB, tableNames []string, dirPathTable string) {
	for _, tableName := range tableNames {
		generateTableSingle(ctx, db, tableName, dirPathTable)
	}
}

// generateTableSingle generates the gooq table object for a single table.
func generateTableSingle(ctx context.Context, db gdb.DB, tableName, dirPathTable string) {
	fieldMap, err := db.TableFields(ctx, tableName)
	if err != nil {
		mlog.Fatalf(`fetching tables fields failed for table "%s": %+v`, tableName, err)
	}
	fileName := formatFileName(tableName, "")
	path := filepath.FromSlash(gfile.Join(dirPathTable, fileName+".go"))
	tableContent, err := generateTableContent(ctx, db, tableName, dirPathTable, fieldMap)
	if err != nil {
		mlog.Fatalf(`generating gooq table content failed for table "%s": %+v`, tableName, err)
	}
	if err = gfile.PutContents(path, tableContent); err != nil {
		mlog.Fatalf("writing content to '%s' failed: %v", path, err)
	} else {
		GoFmt(path)
		mlog.Print("generated:", gfile.RealPath(path))
	}
}

// generateTableContent builds and renders the gooq table object content.
func generateTableContent(
	ctx context.Context, db gdb.DB, tableName, dirPathTable string, fieldMap map[string]*gdb.TableField,
) (string, error) {
	var (
		fieldsLines      []string
		metaLines        []string
		assignLines      []string
		aliasAssignLines []string
		imports          = make([]string, 0, 4)
		hasStdTime       bool
		hasUUID          bool
	)
	// 包内生成（TplPackageName == "gooq"）时无需包前缀与自引用 import。
	gooqRef := "gooq."
	if filepath.Base(dirPathTable) == "gooq" {
		gooqRef = ""
	}
	fieldNames := make([]string, 0, len(fieldMap))
	for fieldName := range fieldMap {
		fieldNames = append(fieldNames, fieldName)
	}
	sort.Slice(fieldNames, func(i, j int) bool {
		return fieldMap[fieldNames[i]].Index < fieldMap[fieldNames[j]].Index
	})
	for _, fieldName := range fieldNames {
		field := fieldMap[fieldName]
		camelName := formatFieldName(fieldName, FieldNameCaseCamel)
		// Go 惯例：Id 词统一为 ID 缩写（UserId → UserID、ProductId → ProductID、主键 id → ID）。
		camelName = gstr.Replace(camelName, "Id", "ID")

		goType, localType := tableFieldTypes(ctx, db, field, &hasStdTime, &hasUUID)

		// Key 大小写不敏感：sqlite 驱动返回 "pri"/"uni"（小写），MySQL 返回 "PRI"/"UNI"。
		primary := strings.EqualFold(field.Key, "PRI")
		autoIncrement := gstr.Contains(field.Extra, "auto_increment")
		// sqlite：INTEGER PRIMARY KEY 即 rowid 别名（自增语义），驱动 Extra 不返回 auto_increment。
		if !autoIncrement && db.GetConfig().Type == "sqlite" && primary && localType == string(gdb.LocalTypeInt) {
			autoIncrement = true
		}
		softDelete := fieldName == "deleted_at" || fieldName == "delete_at"
		unique := strings.EqualFold(field.Key, "UNI")

		fieldsLines = append(fieldsLines, fmt.Sprintf(
			"\t%-20s %sField[%s]", camelName, gooqRef, goType,
		))
		metaLines = append(metaLines, fmt.Sprintf(
			"\t\t\t{ColumnName: %q, LocalType: %sLocalType(%q)%s%s%s%s},",
			fieldName, gooqRef, localType,
			boolSuffix("Primary", primary),
			boolSuffix("AutoIncrement", autoIncrement),
			boolSuffix("SoftDelete", softDelete),
			boolSuffix("Unique", unique),
		))
		assignLines = append(assignLines, fmt.Sprintf(
			"\tt.%s = %sNewFieldAt[%s](t.TableBase, %q)", camelName, gooqRef, goType, fieldName,
		))
		aliasAssignLines = append(aliasAssignLines, fmt.Sprintf(
			"\tnewT.%s.BindTable(newT.TableBase)", camelName,
		))
	}
	if hasStdTime {
		imports = append(imports, `"time"`)
	}
	if hasUUID {
		imports = append(imports, `"github.com/google/uuid"`)
	}

	tplContent := getTemplate(tplFileTable)
	tplPackageName := filepath.Base(dirPathTable)
	tplGooqImport := `	"github.com/lanceadd/gooq"`
	if gooqRef == "" {
		tplGooqImport = ""
	}
	tplView.ClearAssigns()
	tplView.Assigns(gview.Params{
		"TplPackageName":           tplPackageName,
		"TplGooqRef":               gooqRef,
		"TplGooqImport":            tplGooqImport,
		"TplTableNameCamelCase":    formatFieldName(tableName, FieldNameCaseCamel),
		"TplTableName":             tableName,
		"TplTableSchema":           currentSchema(ctx, db),
		"TplGooqFields":            strings.Join(fieldsLines, "\n"),
		"TplGooqFieldMetas":        strings.Join(metaLines, "\n"),
		"TplGooqFieldAssigns":      strings.Join(assignLines, "\n"),
		"TplGooqFieldAliasAssigns": strings.Join(aliasAssignLines, "\n"),
		"TplGooqImports":           strings.Join(imports, "\n"),
	})
	return tplView.ParseContent(ctx, tplContent)
}

// tableFieldTypes returns the Go type name and LocalType string for the given field.
func tableFieldTypes(
	ctx context.Context, db gdb.DB, field *gdb.TableField,
	hasStdTime, hasUUID *bool,
) (goType string, localTypeStr string) {
	localType, err := db.CheckLocalTypeForField(ctx, field.Type, nil)
	if err != nil {
		mlog.Fatalf(`check local type for field "%s" failed: %+v`, field.Name, err)
	}
	localTypeStr = string(localType)
	goType = localTypeStr
	switch localType {
	case gdb.LocalTypeDate, gdb.LocalTypeTime, gdb.LocalTypeDatetime:
		// 统一使用 stdlib time.Time。
		goType = "time.Time"
		*hasStdTime = true
	case gdb.LocalTypeInt64Bytes:
		goType = "int64"
	case gdb.LocalTypeUint64Bytes:
		goType = "uint64"
	case gdb.LocalTypeJson, gdb.LocalTypeJsonb:
		goType = "string"
	case gdb.LocalTypeUUID:
		goType = "uuid.UUID"
		*hasUUID = true
	}
	return goType, localTypeStr
}

// currentSchema returns the default schema for schema-qualified rendering (PG only).
func currentSchema(ctx context.Context, db gdb.DB) string {
	if db.GetConfig().Type != "pgsql" {
		return ""
	}
	if v, err := db.GetValue(ctx, "SELECT current_schema()"); err == nil {
		return v.String()
	}
	return ""
}

// boolSuffix renders a struct field suffix for boolean meta flags.
func boolSuffix(fieldName string, flag bool) string {
	if flag {
		return ", " + fieldName + ": true"
	}
	return ""
}
