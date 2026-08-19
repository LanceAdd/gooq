// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件实现 gooq 类型化表对象的生成：连库取表结构 → 模板渲染 → 写盘。
// 模板为外部文件（template/gooq_table.tpl），支持通过 TplGooqTablePath 参数覆盖。
package gendao

import (
	"context"
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gview"
	"github.com/gogf/gf/v2/text/gstr"

	"github.com/lanceadd/gooq/cmd/ggen/internal/utility/mlog"
	"github.com/lanceadd/gooq/cmd/ggen/internal/utility/utils"
)

//go:embed template/*.tpl
var gooqTemplateFS embed.FS

// generateGooqTable generates gooq typed table object files for given tables.
func generateGooqTable(ctx context.Context, in CGenDaoInternalInput) {
	dirPathTable := gfile.Join(in.Path, in.TablePath)
	in.genItems.AppendDirPath(dirPathTable)
	for i := 0; i < len(in.TableNames); i++ {
		generateGooqTableSingle(ctx, generateGooqTableSingleInput{
			CGenDaoInternalInput: in,
			TableName:            in.TableNames[i],
			NewTableName:         in.NewTableNames[i],
			DirPathTable:         dirPathTable,
		})
	}
}

// generateGooqTableSingleInput is the input parameter for generateGooqTableSingle.
type generateGooqTableSingleInput struct {
	CGenDaoInternalInput
	TableName    string
	NewTableName string
	DirPathTable string
}

// generateGooqTableSingle generates the gooq table object for a single table.
func generateGooqTableSingle(ctx context.Context, in generateGooqTableSingleInput) {
	fieldMap, err := in.DB.TableFields(ctx, in.TableName)
	if err != nil {
		mlog.Fatalf(`fetching tables fields failed for table "%s": %+v`, in.TableName, err)
	}
	fileName := formatFileName(in.NewTableName, in.FileNameCase)
	path := filepath.FromSlash(gfile.Join(in.DirPathTable, fileName+".go"))
	in.genItems.AppendGeneratedFilePath(path)
	if in.OverwriteDao || !gfile.Exists(path) {
		tableContent, err := generateGooqTableContent(ctx, in, fieldMap)
		if err != nil {
			mlog.Fatalf(`generating gooq table content failed for table "%s": %+v`, in.TableName, err)
		}
		if err = gfile.PutContents(path, tableContent); err != nil {
			mlog.Fatalf("writing content to '%s' failed: %v", path, err)
		} else {
			utils.GoFmt(path)
			mlog.Print("generated:", gfile.RealPath(path))
		}
	}
}

// generateGooqTableContent builds and renders the gooq table object content.
func generateGooqTableContent(
	ctx context.Context, in generateGooqTableSingleInput, fieldMap map[string]*gdb.TableField,
) (string, error) {
	var (
		fieldsLines []string
		metaLines   []string
		assignLines []string
		columnNames []string
		imports     = make([]string, 0, 4)
		hasGTime    bool
		hasGJson    bool
		hasStdTime  bool
		hasUUID     bool
	)
	fieldNames := make([]string, 0, len(fieldMap))
	for fieldName := range fieldMap {
		fieldNames = append(fieldNames, fieldName)
	}
	sort.Slice(fieldNames, func(i, j int) bool {
		return fieldMap[fieldNames[i]].Index < fieldMap[fieldNames[j]].Index
	})
	for _, fieldName := range fieldNames {
		field := fieldMap[fieldName]
		newFieldName := fieldName
		for _, v := range gstr.SplitAndTrim(in.RemoveFieldPrefix, ",") {
			newFieldName = gstr.TrimLeftStr(newFieldName, v, 1)
		}
		camelName := formatFieldName(newFieldName, FieldNameCaseCamel)
		// Go 惯例：主键 id 字段命名为 ID（与设计文档/示例一致）。
		if camelName == "Id" {
			camelName = "ID"
		}

		goType, localType := gooqFieldTypes(ctx, in.CGenDaoInternalInput, field, &hasGTime, &hasGJson, &hasStdTime, &hasUUID)

		primary := field.Key == "PRI"
		autoIncrement := gstr.Contains(field.Extra, "auto_increment")
		softDelete := fieldName == "deleted_at" || fieldName == "delete_at"
		unique := field.Key == "UNI"

		fieldsLines = append(fieldsLines, fmt.Sprintf(
			"\t%-20s gooq.Field[%s]", camelName, goType,
		))
		metaLines = append(metaLines, fmt.Sprintf(
			"\t\t\t{ColumnName: %q, LocalType: gooq.LocalType(%q)%s%s%s%s},",
			fieldName, localType,
			boolSuffix("Primary", primary),
			boolSuffix("AutoIncrement", autoIncrement),
			boolSuffix("SoftDelete", softDelete),
			boolSuffix("Unique", unique),
		))
		assignLines = append(assignLines, fmt.Sprintf(
			"\t%s: gooq.NewField[%s](%q, %q),", camelName, goType, in.NewTableName, fieldName,
		))
		columnNames = append(columnNames, strconv.Quote(fieldName))
	}
	if hasGTime {
		imports = append(imports, `"github.com/gogf/gf/v2/os/gtime"`)
	}
	if hasGJson {
		imports = append(imports, `"github.com/gogf/gf/v2/encoding/gjson"`)
	}
	if hasStdTime {
		imports = append(imports, `"time"`)
	}
	if hasUUID {
		imports = append(imports, `"github.com/google/uuid"`)
	}

	tplContent, err := getGooqTableTemplate(in.CGenDaoInternalInput)
	if err != nil {
		return "", err
	}
	tplView.ClearAssigns()
	tplView.Assigns(gview.Params{
		"TplPackageName":        filepath.Base(in.TablePath),
		"TplTableNameCamelCase": formatFieldName(in.NewTableName, FieldNameCaseCamel),
		"TplTableName":          in.TableName,
		"TplGooqFields":         strings.Join(fieldsLines, "\n"),
		"TplGooqFieldMetas":     strings.Join(metaLines, "\n"),
		"TplGooqFieldAssigns":   strings.Join(assignLines, "\n"),
		"TplGooqAllColumns":     strings.Join(columnNames, ", "),
		"TplGooqImports":        strings.Join(imports, "\n"),
	})
	return tplView.ParseContent(ctx, tplContent)
}

// gooqFieldTypes returns the Go type name and LocalType string for the given field.
func gooqFieldTypes(
	ctx context.Context, in CGenDaoInternalInput, field *gdb.TableField,
	hasGTime, hasGJson, hasStdTime, hasUUID *bool,
) (goType string, localTypeStr string) {
	localType, err := in.DB.CheckLocalTypeForField(ctx, field.Type, nil)
	if err != nil {
		mlog.Fatalf(`check local type for field "%s" failed: %+v`, field.Name, err)
	}
	localTypeStr = string(localType)
	goType = localTypeStr
	switch localType {
	case gdb.LocalTypeDate, gdb.LocalTypeTime, gdb.LocalTypeDatetime:
		if in.StdTime {
			goType = "time.Time"
			*hasStdTime = true
		} else {
			goType = "*gtime.Time"
			*hasGTime = true
		}
	case gdb.LocalTypeInt64Bytes:
		goType = "int64"
	case gdb.LocalTypeUint64Bytes:
		goType = "uint64"
	case gdb.LocalTypeJson, gdb.LocalTypeJsonb:
		if in.GJsonSupport {
			goType = "*gjson.Json"
			*hasGJson = true
		} else {
			goType = "string"
		}
	case gdb.LocalTypeUUID:
		goType = "uuid.UUID"
		*hasUUID = true
	}
	return goType, localTypeStr
}

// getGooqTableTemplate reads the gooq table template: external path first, embedded as fallback.
func getGooqTableTemplate(in CGenDaoInternalInput) (string, error) {
	if in.TplGooqTablePath != "" {
		if gfile.Exists(in.TplGooqTablePath) {
			return gfile.GetContents(in.TplGooqTablePath), nil
		}
		return "", fmt.Errorf("template file not found: %s", in.TplGooqTablePath)
	}
	content, err := gooqTemplateFS.ReadFile("template/gooq_table.tpl")
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// boolSuffix renders a struct field suffix for boolean meta flags.
func boolSuffix(fieldName string, flag bool) string {
	if flag {
		return ", " + fieldName + ": true"
	}
	return ""
}
