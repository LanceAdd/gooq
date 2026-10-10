// 本文件实现 gooq 行模型生成：可链式赋值，并通过 Assignments 交给 ValuesOf。

package gen

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gview"

	"github.com/lanceadd/gooq/cmd/gooq-gen/internal/mlog"
	"github.com/lanceadd/gooq/db"
)

const dirPathModel = "model"

var reservedModelNames = map[string]bool{
	"Assignments": true,
}

// generateModel generates row models for the given tables.
func generateModel(ctx context.Context, database *db.DB, tableNames []string, dirPath string) {
	for _, tableName := range tableNames {
		generateModelSingle(ctx, database, tableName, dirPath)
	}
}

func generateModelSingle(ctx context.Context, database *db.DB, tableName, dirPath string) {
	columns, err := database.TableFields(ctx, tableName)
	if err != nil {
		mlog.Fatalf(`fetching tables fields failed for table "%s": %+v`, tableName, err)
	}
	fileName := formatFileName(tableName, "")
	path := filepath.FromSlash(gfile.Join(dirPath, fileName+".go"))
	content, err := generateModelContent(ctx, tableName, dirPath, columns)
	if err != nil {
		mlog.Fatalf(`generating gooq model content failed for table "%s": %+v`, tableName, err)
	}
	if err = gfile.PutContents(path, content); err != nil {
		mlog.Fatalf("writing content to '%s' failed: %v", path, err)
	}
	GoFmt(path)
	mlog.Print("generated:", gfile.RealPath(path))
}

func generateModelContent(ctx context.Context, tableName, dirPath string, columns []db.Column) (string, error) {
	typeName := formatFieldName(tableName, FieldNameCaseCamel)
	var (
		fields      []string
		methods     []string
		imports     []string
		hasStdTime  bool
		hasUUID     bool
		assignments []string
	)
	for i, field := range columns {
		camel := modelFieldName(field.Name)
		nullable := field.Nullable && !field.Primary
		fieldType, setType := modelFieldTypes(field, nullable, &hasStdTime, &hasUUID)
		fields = append(fields, fmt.Sprintf("\t%s %s", camel, fieldType))
		methods = append(methods, modelSetter(typeName, camel, setType, fieldType, nullable, i))
		assignments = append(assignments, modelAssignment(camel, field.Name, nullable, i))
	}
	methods = append(methods, modelAssignmentsMethod(typeName, len(columns), strings.Join(assignments, "\n")))
	if hasStdTime {
		imports = append(imports, `"time"`)
	}
	if hasUUID {
		imports = append(imports, `"github.com/google/uuid"`)
	}
	imports = append(imports, `"github.com/lanceadd/gooq"`)

	tplView.ClearAssigns()
	tplView.Assigns(gview.Params{
		"TplPackageName": filepath.Base(dirPath),
		"TplImports":     strings.Join(imports, "\n"),
		"TplType":        typeName,
		"TplTableName":   tableName,
		"TplFields":      strings.Join(fields, "\n"),
		"TplStateLen":    len(columns),
		"TplMethods":     strings.Join(methods, "\n"),
	})
	return tplView.ParseContent(ctx, getTemplate(tplFileModel))
}

func modelFieldName(column string) string {
	name := formatFieldName(column, FieldNameCaseCamel)
	if reservedModelNames[name] {
		return name + "Field"
	}
	return name
}

func modelFieldTypes(field db.Column, nullable bool, hasStdTime, hasUUID *bool) (fieldType, setType string) {
	setType, _ = tableFieldTypes(field, hasStdTime, hasUUID)
	fieldType = setType
	if nullable {
		fieldType = "*" + setType
	}
	return fieldType, setType
}

func modelSetter(typeName, camel, setType, fieldType string, nullable bool, index int) string {
	var setter string
	if nullable {
		setter = fmt.Sprintf(`func (m *%s) Set%s(v %s) *%s {
	value := v
	m.%s = &value
	m.state[%d] = gooq.ValueSet
	return m
}`, typeName, camel, setType, typeName, camel, index)
		setter += fmt.Sprintf(`

func (m *%s) Set%sNull() *%s {
	m.%s = nil
	m.state[%d] = gooq.ValueNull
	return m
}`, typeName, camel, typeName, camel, index)
		return setter
	}
	return fmt.Sprintf(`func (m *%s) Set%s(v %s) *%s {
	m.%s = v
	m.state[%d] = gooq.ValueSet
	return m
}`, typeName, camel, fieldType, typeName, camel, index)
}

func modelAssignment(camel, column string, nullable bool, index int) string {
	if !nullable {
		return fmt.Sprintf(`	if m.state[%d] == gooq.ValueSet {
		out = append(out, gooq.Assignment{Column: %q, Value: m.%s})
	}`, index, column, camel)
	}
	return fmt.Sprintf(`	if m.state[%d] == gooq.ValueNull {
		out = append(out, gooq.Assignment{Column: %q, Null: true})
	} else if m.state[%d] == gooq.ValueSet {
		out = append(out, gooq.Assignment{Column: %q, Value: m.%s})
	}`, index, column, index, column, camel)
}

func modelAssignmentsMethod(typeName string, count int, body string) string {
	return fmt.Sprintf(`func (m *%s) Assignments() []gooq.Assignment {
	if m == nil {
		return nil
	}
	out := make([]gooq.Assignment, 0, %d)
%s
	return out
}`, typeName, count, body)
}
