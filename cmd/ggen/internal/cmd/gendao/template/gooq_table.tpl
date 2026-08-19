// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package {{.TplPackageName}}

import (
	"github.com/lanceadd/gooq"
{{- if .TplGooqImports}}
{{.TplGooqImports}}
{{- end}}
)

// {{.TplTableNameCamelCase}}Table is the typed table object for table "{{.TplTableName}}".
type {{.TplTableNameCamelCase}}Table struct {
	*gooq.TableBase
	ALL_FIELDS gooq.AllFields

{{.TplGooqFields}}
}

// {{.TplTableNameCamelCase}} is the globally accessible table object for table {{.TplTableName}}.
var {{.TplTableNameCamelCase}} = &{{.TplTableNameCamelCase}}Table{
	TableBase: gooq.NewTableBase(&gooq.TableMeta{
		TableName: "{{.TplTableName}}",
		Fields: []gooq.FieldMeta{
{{.TplGooqFieldMetas}}
		},
	}),
	ALL_FIELDS: gooq.NewAllFields([]string{ {{.TplGooqAllColumns}} }),
{{.TplGooqFieldAssigns}}
}

// As returns a copy of the table object with the given alias.
func (t *{{.TplTableNameCamelCase}}Table) As(alias string) *{{.TplTableNameCamelCase}}Table {
	newT := *t
	newT.TableBase = t.TableBase.As(alias)
	return &newT
}

// Clone returns a copy of the table object without alias.
func (t *{{.TplTableNameCamelCase}}Table) Clone() *{{.TplTableNameCamelCase}}Table {
	newT := *t
	newT.TableBase = t.TableBase.Clone()
	return &newT
}
