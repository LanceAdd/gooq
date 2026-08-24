// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件实现 do 结构体生成（model/do 目录，字段类型统一为 any）。
package gendao

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gfile"

	"github.com/gogf/gf/cmd/ggen/internal/mlog"
)

// generateDo generates do struct files for given tables.
func generateDo(ctx context.Context, db gdb.DB, tableNames []string, dirPathDo string) {
	for _, tableName := range tableNames {
		fieldMap, err := db.TableFields(ctx, tableName)
		if err != nil {
			mlog.Fatalf("fetching tables fields failed for table '%s':\n%v", tableName, err)
		}
		tableNameCamelCase := formatFieldName(tableName, FieldNameCaseCamel)
		doFilePath := gfile.Join(dirPathDo, formatFileName(tableName, "")+".go")
		structDefinition := generateStructDefinition(ctx, db, fieldMap, tableNameCamelCase, tableName, true)
		doContent := renderStructContent(
			getTemplate(tplFileDo), "do", tableName, tableNameCamelCase, structDefinition, true,
		)
		err = gfile.PutContents(doFilePath, strings.TrimSpace(doContent))
		if err != nil {
			mlog.Fatalf(`writing content to "%s" failed: %v`, doFilePath, err)
		} else {
			GoFmt(doFilePath)
			mlog.Print("generated:", gfile.RealPath(doFilePath))
		}
	}
}
