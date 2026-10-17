// 本文件实现 gooq 类型化表对象的生成：连库取表结构 → 模板渲染 → 写盘。
package gen

import (
	"context"
	"embed"
	"fmt"
	"path/filepath"
	"sort"
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
	"Physical":   true,
	"ShardBy":    true,
	"TableBase":  true,
	"BoundBase":  true,
}

func generateTable(ctx context.Context, database *db.DB, specs []tableSpec, dirPathTable string) {
	tenantCfg, err := loadTenantSettings()
	if err != nil {
		mlog.Fatalf("%+v", err)
	}
	for _, spec := range specs {
		generateTableSingle(ctx, database, spec, dirPathTable, tenantCfg)
	}
	generateShardingRegister(specs, dirPathTable)
}

func generateTableSingle(ctx context.Context, database *db.DB, spec tableSpec, dirPathTable string, tenantCfg tenantSettings) {
	columns, err := database.TableFields(ctx, spec.Physical)
	if err != nil {
		mlog.Fatalf(`fetching tables fields failed for table "%s": %+v`, spec.Physical, err)
	}
	fileName := formatFileName(spec.Logical, "")
	tableContent, err := generateTableContent(ctx, database, spec, dirPathTable, columns, tenantCfg)
	if err != nil {
		mlog.Fatalf(`generating gooq table content failed for table "%s": %+v`, spec.Logical, err)
	}
	writeGenerated(gfile.Join(dirPathTable, fileName+".go"), tableContent)
}

func generateShardingRegister(specs []tableSpec, dirPathTable string) {
	path := gfile.Join(dirPathTable, "sharding.go")
	var names []string
	for _, spec := range specs {
		if spec.Sharding {
			names = append(names, formatFieldName(spec.Logical, FieldNameCaseCamel))
		}
	}
	if len(names) == 0 {
		if gfile.Exists(path) {
			_ = gfile.Remove(path)
		}
		return
	}
	sort.Strings(names)
	var assigns []string
	for _, name := range names {
		assigns = append(assigns, fmt.Sprintf("\t%sMeta.Sharding = rule", name))
	}
	gooqRef := "gooq."
	tplGooqImport := `	"github.com/lanceadd/gooq"`
	if filepath.Base(dirPathTable) == "gooq" {
		gooqRef = ""
		tplGooqImport = ""
	}
	tplView.ClearAssigns()
	tplView.Assigns(gview.Params{
		"TplPackageName": filepath.Base(dirPathTable),
		"TplGooqRef":     gooqRef,
		"TplGooqImport":  tplGooqImport,
		"TplAssignments": strings.Join(assigns, "\n"),
	})
	content, err := tplView.ParseContent(context.Background(), getTemplate(tplFileSharding))
	if err != nil {
		mlog.Fatalf("generating sharding register failed: %+v", err)
	}
	writeGenerated(path, content)
}

func generateTableContent(
	ctx context.Context, database *db.DB, spec tableSpec, dirPathTable string, columns []db.Column, tenantCfg tenantSettings,
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
	tenantCol, tenantErr := tenantColumn(spec.Logical, columns, tenantCfg)
	if tenantErr != nil {
		return "", tenantErr
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
		tenant := tenantCol != nil && tenantCol.Name == field.Name
		primary := field.Primary
		autoIncrement := field.AutoIncrement
		unique := field.Unique

		fieldsLines = append(fieldsLines, fmt.Sprintf(
			"\t%-20s %sField[%s]", camelName, gooqRef, goType,
		))
		metaLines = append(metaLines, fmt.Sprintf(
			"\t\t{ColumnName: %q, LocalType: %s%s%s%s%s%s},",
			fieldName, localTypeRef(gooqRef, field.LocalType),
			boolSuffix("Primary", primary),
			boolSuffix("AutoIncrement", autoIncrement),
			boolSuffix("SoftDelete", softDelete),
			boolSuffix("Tenant", tenant),
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

	var shardLines []string
	if spec.Sharding {
		for _, name := range spec.Shards {
			shardLines = append(shardLines, fmt.Sprintf("\t\t%q,", name))
		}
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
		"TplTableSharding":      spec.Sharding,
		"TplTableNameCamelCase": formatFieldName(spec.Logical, FieldNameCaseCamel),
		"TplTableName":          spec.Logical,
		"TplTableSchema":        currentSchema(ctx, database),
		"TplShardNames":         strings.Join(shardLines, "\n"),
		"TplGooqFields":         strings.Join(fieldsLines, "\n"),
		"TplGooqFieldMetas":     strings.Join(metaLines, "\n"),
		"TplGooqFieldAssigns":   strings.Join(assignLines, "\n"),
		"TplGooqImports":        strings.Join(imports, "\n"),
	})
	return tplView.ParseContent(ctx, tplContent)
}

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

// currentSchema 返回带 schema 渲染用的默认 schema（仅 PG）。
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

func boolSuffix(fieldName string, flag bool) string {
	if flag {
		return ", " + fieldName + ": true"
	}
	return ""
}
