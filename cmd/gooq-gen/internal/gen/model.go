// 本文件实现 gooq 行模型生成：普通结构体带 json/orm 标签，写入记录使用 Property[T]。

package gen

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gogf/gf/v2/os/gview"
	"github.com/gogf/gf/v2/text/gstr"

	"github.com/lanceadd/gooq/cmd/gooq-gen/internal/mlog"
	"github.com/lanceadd/gooq/db"
)

const dirPathModel = "model"

func generateModel(ctx context.Context, database *db.DB, specs []tableSpec, dirPath string) {
	for _, spec := range specs {
		generateModelSingle(ctx, database, spec, dirPath)
	}
}

func generateModelSingle(ctx context.Context, database *db.DB, spec tableSpec, dirPath string) {
	columns, err := database.TableFields(ctx, spec.Physical)
	if err != nil {
		mlog.Fatalf(`fetching tables fields failed for table "%s": %+v`, spec.Physical, err)
	}
	fileName := formatFileName(spec.Logical, "")
	modelContent, recordContent, err := generateModelContent(ctx, spec.Logical, dirPath, columns)
	if err != nil {
		mlog.Fatalf(`generating gooq model content failed for table "%s": %+v`, spec.Logical, err)
	}
	writeGenerated(filepath.Join(dirPath, fileName+"_model.go"), modelContent)
	writeGenerated(filepath.Join(dirPath, fileName+"_record.go"), recordContent)
}

func generateModelContent(ctx context.Context, tableName, dirPath string, columns []db.Column) (string, string, error) {
	baseName := formatFieldName(tableName, FieldNameCaseCamel)
	typeName := baseName + "Model"
	recordName := baseName + "Record"
	var (
		fields       []string
		recordFields []string
		methods      []string
		modelImports []string
		imports      []string
		hasStdTime   bool
		hasUUID      bool
		assignments  []string
		toModel      []string
		fromModel    []string
	)
	var named []namedField
	usedNames := map[string]string{}
	for _, field := range columns {
		camel, nameErr := goFieldName(field.Name, nil, usedNames)
		if nameErr != nil {
			return "", "", nameErr
		}
		nullable := field.Nullable && !field.Primary
		named = append(named, namedField{Column: field.Name, Name: camel, Nullable: nullable})
	}
	conflicts := recordConflicts(named)
	for _, field := range named {
		if decl, ok := conflicts[field.Name]; ok {
			return "", "", fmt.Errorf("gooq: column %q conflicts with %s", field.Column, decl)
		}
	}
	for _, field := range columns {
		camel := formatFieldName(field.Name, FieldNameCaseCamel)
		nullable := field.Nullable && !field.Primary
		fieldType, setType := modelFieldTypes(field, nullable, &hasStdTime, &hasUUID)
		fields = append(fields, fmt.Sprintf("\t%s %s `json:%q orm:%q`", camel, fieldType, modelJSONName(field.Name), field.Name))
		recordFields = append(recordFields, fmt.Sprintf("\t%s gooq.Property[%s]", camel, setType))
		methods = append(methods, modelSetter(recordName, camel, setType, nullable))
		assignments = append(assignments, modelAssignment(camel, field.Name))
		toModel = append(toModel, modelToModelLine(camel, nullable))
		fromModel = append(fromModel, modelFromModelLine(camel, setType, nullable))
	}
	methods = append(methods,
		modelAssignmentsMethod(recordName, len(columns), strings.Join(assignments, "\n")),
		modelToModelMethod(recordName, typeName, strings.Join(toModel, "\n")),
		modelFromModelMethod(recordName, typeName, strings.Join(fromModel, "\n")),
	)
	if hasStdTime {
		modelImports = append(modelImports, `"time"`)
		imports = append(imports, `"time"`)
	}
	if hasUUID {
		modelImports = append(modelImports, `"github.com/google/uuid"`)
		imports = append(imports, `"github.com/google/uuid"`)
	}
	imports = append(imports, `"github.com/lanceadd/gooq"`)

	pkg := filepath.Base(dirPath)
	modelView := gview.Params{
		"TplPackageName": pkg,
		"TplImports":     strings.Join(modelImports, "\n"),
		"TplType":        typeName,
		"TplTableName":   tableName,
		"TplFields":      strings.Join(fields, "\n"),
	}
	recordView := gview.Params{
		"TplPackageName":  pkg,
		"TplImports":      strings.Join(imports, "\n"),
		"TplType":         typeName,
		"TplRecordType":   recordName,
		"TplTableName":    tableName,
		"TplRecordFields": strings.Join(recordFields, "\n"),
		"TplMethods":      strings.Join(methods, "\n\n"),
	}
	modelContent, err := renderTemplate(ctx, tplFileModel, modelView)
	if err != nil {
		return "", "", err
	}
	recordContent, err := renderTemplate(ctx, tplFileRecord, recordView)
	if err != nil {
		return "", "", err
	}
	return modelContent, recordContent, nil
}

func renderTemplate(ctx context.Context, name string, params gview.Params) (string, error) {
	tplView.ClearAssigns()
	tplView.Assigns(params)
	return tplView.ParseContent(ctx, getTemplate(name))
}

func modelJSONName(column string) string {
	if isAllUpper(column) {
		column = strings.ToLower(column)
	}
	return gstr.CaseCamelLower(column)
}

func modelFieldTypes(field db.Column, nullable bool, hasStdTime, hasUUID *bool) (fieldType, setType string) {
	setType = tableFieldTypes(field, hasStdTime, hasUUID)
	fieldType = setType
	if nullable {
		fieldType = "*" + setType
	}
	return fieldType, setType
}

func modelSetter(recordName, camel, setType string, nullable bool) string {
	setter := fmt.Sprintf(`func (r *%s) Set%s(v %s) *%s {
	r.%s = gooq.Set(v)
	return r
}`, recordName, camel, setType, recordName, camel)
	if !nullable {
		return setter
	}
	setter += fmt.Sprintf(`

func (r *%s) Set%sNull() *%s {
	r.%s = gooq.Null[%s]()
	return r
}`, recordName, camel, recordName, camel, setType)
	return setter
}

func modelAssignment(camel, column string) string {
	return fmt.Sprintf(`	if item, ok := r.%s.Assignment(%q); ok {
		out = append(out, item)
	}`, camel, column)
}

func modelToModelLine(camel string, nullable bool) string {
	if !nullable {
		return fmt.Sprintf(`	if value, ok := r.%s.Value(); ok {
		row.%s = value
	}`, camel, camel)
	}
	return fmt.Sprintf(`	if value, ok := r.%s.Value(); ok {
		row.%s = &value
	}`, camel, camel)
}

func modelFromModelLine(camel, setType string, nullable bool) string {
	if !nullable {
		return fmt.Sprintf(`	r.%s = gooq.Set(row.%s)`, camel, camel)
	}
	return fmt.Sprintf(`	if row.%s == nil {
		r.%s = gooq.Null[%s]()
	} else {
		r.%s = gooq.Set(*row.%s)
	}`, camel, camel, setType, camel, camel)
}

func modelAssignmentsMethod(recordName string, count int, body string) string {
	return fmt.Sprintf(`func (r *%s) Assignments() []gooq.Assignment {
	if r == nil {
		return nil
	}
	out := make([]gooq.Assignment, 0, %d)
%s
	return out
}`, recordName, count, body)
}

func modelToModelMethod(recordName, typeName, body string) string {
	return fmt.Sprintf(`func (r *%s) ToModel() %s {
	if r == nil {
		return %s{}
	}
	var row %s
%s
	return row
}`, recordName, typeName, typeName, typeName, body)
}

func modelFromModelMethod(recordName, typeName, body string) string {
	return fmt.Sprintf(`func New%sFrom(row %s) *%s {
	r := New%s()
%s
	return r
}`, recordName, typeName, recordName, recordName, body)
}
