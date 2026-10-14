// 本文件实现 gooq 类型化表对象的生成：连库取表结构 → 模板渲染 → 写盘。
// 模板为外部文件（template/table.tmpl）。
package gen

import (
	"context"
	"embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gview"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/cmd/gooq-gen/internal/mlog"
	"github.com/lanceadd/gooq/db"
)

//go:embed template/*
var gooqTemplateFS embed.FS

var reservedFieldNames = map[string]bool{
	"TableName":  true,
	"AliasName":  true,
	"Meta":       true,
	"AllColumns": true,
	"AllFields":  true,
	"Field":      true,
	"As":         true,
	"Clone":      true,
	"TableBase":  true,
	"BoundBase":  true,
}

// generateTable generates gooq typed table object files for given tables.
func generateTable(ctx context.Context, database *db.DB, tableNames []string, dirPathTable string) {
	for _, tableName := range tableNames {
		generateTableSingle(ctx, database, tableName, dirPathTable)
	}
}

// generateTableSingle generates the gooq table object for a single table.
func generateTableSingle(ctx context.Context, database *db.DB, tableName, dirPathTable string) {
	columns, err := database.TableFields(ctx, tableName)
	if err != nil {
		mlog.Fatalf(`fetching tables fields failed for table "%s": %+v`, tableName, err)
	}
	fileName := formatFileName(tableName, "")
	path := filepath.FromSlash(gfile.Join(dirPathTable, fileName+".go"))
	tableContent, err := generateTableContent(ctx, database, tableName, dirPathTable, columns)
	if err != nil {
		mlog.Fatalf(`generating gooq table content failed for table "%s": %+v`, tableName, err)
	}
	if err = gfile.PutContents(path, tableContent); err != nil {
		mlog.Fatalf("writing content to '%s' failed: %v", path, err)
	}
	if err = GoFmt(path); err != nil {
		mlog.Fatalf(`formatting "%s" failed: %v`, path, err)
	}
	mlog.Print("generated:", gfile.RealPath(path))
}

// generateTableContent builds and renders the gooq table object content.
func generateTableContent(
	ctx context.Context, database *db.DB, tableName, dirPathTable string, columns []db.Column,
) (string, error) {
	var (
		fieldsLines []string
		metaLines   []string
		assignLines []string
		imports     = make([]string, 0, 4)
		hasStdTime  bool
		hasUUID     bool
	)
	// 包内生成（TplPackageName == "gooq"）时无需包前缀与自引用 import。
	gooqRef := "gooq."
	if filepath.Base(dirPathTable) == "gooq" {
		gooqRef = ""
	}
	usedNames := map[string]string{}
	for _, field := range columns {
		fieldName := field.Name
		camelName, nameErr := goFieldName(fieldName, reservedFieldNames, usedNames)
		if nameErr != nil {
			return "", nameErr
		}

		goType := tableFieldTypes(field, &hasStdTime, &hasUUID)
		softDelete := isSoftDeleteColumn(fieldName)
		primary := field.Primary
		autoIncrement := field.AutoIncrement
		unique := field.Unique

		fieldsLines = append(fieldsLines, fmt.Sprintf(
			"\t%-20s %sField[%s]", camelName, gooqRef, goType,
		))
		metaLines = append(metaLines, fmt.Sprintf(
			"\t\t{ColumnName: %q, LocalType: %s%s%s%s%s},",
			fieldName, localTypeRef(gooqRef, field.LocalType),
			boolSuffix("Primary", primary),
			boolSuffix("AutoIncrement", autoIncrement),
			boolSuffix("SoftDelete", softDelete),
			boolSuffix("Unique", unique),
		))
		assignLines = append(assignLines, fmt.Sprintf(
			"\tt.%s = %sNewFieldAt[%s](t.TableBase, %q)", camelName, gooqRef, goType, fieldName,
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
		"TplPackageName":        tplPackageName,
		"TplGooqRef":            gooqRef,
		"TplGooqImport":         tplGooqImport,
		"TplTableNameCamelCase": formatFieldName(tableName, FieldNameCaseCamel),
		"TplTableName":          tableName,
		"TplTableSchema":        currentSchema(ctx, database),
		"TplGooqFields":         strings.Join(fieldsLines, "\n"),
		"TplGooqFieldMetas":     strings.Join(metaLines, "\n"),
		"TplGooqFieldAssigns":   strings.Join(assignLines, "\n"),
		"TplGooqImports":        strings.Join(imports, "\n"),
	})
	return tplView.ParseContent(ctx, tplContent)
}

// tableFieldTypes returns the Go type name for the given field.
func tableFieldTypes(field db.Column, hasStdTime, hasUUID *bool) string {
	goType := string(field.LocalType)
	switch field.LocalType {
	case gooq.LocalTypeDate, gooq.LocalTypeTime, gooq.LocalTypeDatetime:
		goType = "time.Time"
		*hasStdTime = true
	case gooq.LocalTypeInt64Bytes:
		goType = "int64"
	case gooq.LocalTypeUint64Bytes:
		goType = "uint64"
	case gooq.LocalTypeJson, gooq.LocalTypeJsonb, gooq.LocalTypeDecimal:
		goType = "string"
	case gooq.LocalTypeUUID:
		goType = "uuid.UUID"
		*hasUUID = true
	case gooq.LocalTypeUUIDSlice:
		*hasUUID = true
	}
	return goType
}

var localTypeConstantNames = map[gooq.LocalType]string{
	gooq.LocalTypeUndefined:    "LocalTypeUndefined",
	gooq.LocalTypeString:       "LocalTypeString",
	gooq.LocalTypeTime:         "LocalTypeTime",
	gooq.LocalTypeDate:         "LocalTypeDate",
	gooq.LocalTypeDatetime:     "LocalTypeDatetime",
	gooq.LocalTypeInt:          "LocalTypeInt",
	gooq.LocalTypeUint:         "LocalTypeUint",
	gooq.LocalTypeInt32:        "LocalTypeInt32",
	gooq.LocalTypeUint32:       "LocalTypeUint32",
	gooq.LocalTypeInt64:        "LocalTypeInt64",
	gooq.LocalTypeUint64:       "LocalTypeUint64",
	gooq.LocalTypeBigInt:       "LocalTypeBigInt",
	gooq.LocalTypeIntSlice:     "LocalTypeIntSlice",
	gooq.LocalTypeUintSlice:    "LocalTypeUintSlice",
	gooq.LocalTypeInt32Slice:   "LocalTypeInt32Slice",
	gooq.LocalTypeUint32Slice:  "LocalTypeUint32Slice",
	gooq.LocalTypeInt64Slice:   "LocalTypeInt64Slice",
	gooq.LocalTypeUint64Slice:  "LocalTypeUint64Slice",
	gooq.LocalTypeStringSlice:  "LocalTypeStringSlice",
	gooq.LocalTypeInt64Bytes:   "LocalTypeInt64Bytes",
	gooq.LocalTypeUint64Bytes:  "LocalTypeUint64Bytes",
	gooq.LocalTypeFloat32:      "LocalTypeFloat32",
	gooq.LocalTypeFloat64:      "LocalTypeFloat64",
	gooq.LocalTypeDecimal:      "LocalTypeDecimal",
	gooq.LocalTypeFloat32Slice: "LocalTypeFloat32Slice",
	gooq.LocalTypeFloat64Slice: "LocalTypeFloat64Slice",
	gooq.LocalTypeBytes:        "LocalTypeBytes",
	gooq.LocalTypeBytesSlice:   "LocalTypeBytesSlice",
	gooq.LocalTypeBool:         "LocalTypeBool",
	gooq.LocalTypeBoolSlice:    "LocalTypeBoolSlice",
	gooq.LocalTypeJson:         "LocalTypeJson",
	gooq.LocalTypeJsonb:        "LocalTypeJsonb",
	gooq.LocalTypeUUID:         "LocalTypeUUID",
	gooq.LocalTypeUUIDSlice:    "LocalTypeUUIDSlice",
}

func localTypeRef(gooqRef string, localType gooq.LocalType) string {
	if name, ok := localTypeConstantNames[localType]; ok {
		return gooqRef + name
	}
	return fmt.Sprintf("%sLocalType(%q)", gooqRef, string(localType))
}

// currentSchema returns the default schema for schema-qualified rendering (PG only).
func currentSchema(ctx context.Context, database *db.DB) string {
	if database.Dialect() != gooq.DialectPgsql {
		return ""
	}
	value, err := database.GetValue(ctx, "SELECT current_schema()")
	if err != nil || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

// boolSuffix renders a struct field suffix for boolean meta flags.
func boolSuffix(fieldName string, flag bool) string {
	if flag {
		return ", " + fieldName + ": true"
	}
	return ""
}
