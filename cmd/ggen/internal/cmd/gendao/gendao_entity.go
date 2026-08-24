// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件实现 entity 结构体生成（model/entity 目录，带 json/orm tag）。
package gendao

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gfile"

	"github.com/gogf/gf/cmd/ggen/internal/mlog"
)

// generateEntity generates entity struct files for given tables.
func generateEntity(ctx context.Context, db gdb.DB, tableNames []string, dirPathEntity string) {
	for _, tableName := range tableNames {
		fieldMap, err := db.TableFields(ctx, tableName)
		if err != nil {
			mlog.Fatalf("fetching tables fields failed for table '%s':\n%v", tableName, err)
		}
		tableNameCamelCase := formatFieldName(tableName, FieldNameCaseCamel)
		entityFilePath := gfile.Join(dirPathEntity, formatFileName(tableName, "")+".go")
		structDefinition := generateStructDefinition(ctx, db, fieldMap, tableNameCamelCase, tableName, false)
		entityContent := renderStructContent(
			getTemplate(tplFileEntity), "entity", tableName, tableNameCamelCase, structDefinition, false,
		)
		err = gfile.PutContents(entityFilePath, strings.TrimSpace(entityContent))
		if err != nil {
			mlog.Fatalf("writing content to '%s' failed: %v", entityFilePath, err)
		} else {
			GoFmt(entityFilePath)
			mlog.Print("generated:", gfile.RealPath(entityFilePath))
		}
	}
}
