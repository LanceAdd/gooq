// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件实现 gen dao 命令：连接数据库 → 解析表名 → 生成 gooq 类型化表对象。
package gendao

import (
	"context"

	"github.com/gogf/gf/v2/container/garray"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/text/gregex"
	"github.com/gogf/gf/v2/text/gstr"
	"github.com/gogf/gf/v2/util/gmeta"

	"github.com/gogf/gf/cmd/ggen/internal/utility/mlog"
)

type (
	CGenDao      struct{}
	CGenDaoInput struct {
		gmeta.Meta        `name:"dao" usage:"ggen gen dao [OPTION]" brief:"generate gooq typed table objects from database"`
		Path              string `name:"path"              short:"p"  brief:"output directory" d:"internal"`
		Link              string `name:"link"              short:"l"  brief:"database link, like mysql:root:pass@tcp(127.0.0.1:3306)/db"`
		Tables            string `name:"tables"            short:"t"  brief:"table names or wildcard patterns, comma separated"`
		TablesEx          string `name:"tablesEx"          short:"x"  brief:"excluded tables, comma separated"`
		Prefix            string `name:"prefix"            short:"f"  brief:"prefix added to table names"`
		RemovePrefix      string `name:"removePrefix"      short:"r"  brief:"prefix removed from table names"`
		RemoveFieldPrefix string `name:"removeFieldPrefix" short:"rf" brief:"prefix removed from field names"`
		TablePath         string `name:"tablePath"         short:"tp" brief:"table directory" d:"table"`
		StdTime           bool   `name:"stdTime"           short:"s"  brief:"use time.Time instead of *gtime.Time" orphan:"true"`
		GJsonSupport      bool   `name:"gJsonSupport"      short:"n"  brief:"use *gjson.Json for json columns" orphan:"true"`
		OverwriteDao      bool   `name:"overwriteDao"      short:"v"  brief:"overwrite existing files" orphan:"true"`
		TplGooqTablePath  string `name:"tplGooqTablePath"  short:"t5" brief:"custom gooq table template file path" orphan:"true"`
	}
	CGenDaoOutput struct{}

	CGenDaoInternalInput struct {
		CGenDaoInput
		DB            gdb.DB
		TableNames    []string
		NewTableNames []string
	}
)

// Dao generates gooq typed table objects for given tables.
func (c CGenDao) Dao(ctx context.Context, in CGenDaoInput) (out *CGenDaoOutput, err error) {
	doGenDaoForArray(ctx, in)
	mlog.Print("done!")
	return
}

// doGenDaoForArray implements the "gen dao" command for a single database link.
func doGenDaoForArray(ctx context.Context, in CGenDaoInput) {
	if dirRealPath := gfile.RealPath(in.Path); dirRealPath == "" {
		mlog.Fatalf(`path "%s" does not exist`, in.Path)
	}
	if in.Link == "" {
		mlog.Fatalf(`database link is required, use -l/--link`)
	}
	var (
		err error
		db  gdb.DB
	)
	// It uses user passed database configuration.
	var tempGroup = gtime.TimestampNanoStr()
	err = gdb.AddConfigNode(tempGroup, gdb.ConfigNode{
		Link: in.Link,
	})
	if err != nil {
		mlog.Fatalf(`database configuration failed: %+v`, err)
	}
	if db, err = gdb.Instance(tempGroup); err != nil {
		mlog.Fatalf(`database initialization failed: %+v`, err)
	}

	// Table names resolving: exact names or wildcard patterns.
	var tableNames []string
	if in.Tables != "" {
		inputTables := gstr.SplitAndTrim(in.Tables, ",")
		// Check if any table pattern contains wildcard characters.
		// https://github.com/gogf/gf/issues/4629
		var hasPattern bool
		for _, t := range inputTables {
			if containsWildcard(t) {
				hasPattern = true
				break
			}
		}
		if hasPattern {
			allTables, err := db.Tables(context.TODO())
			if err != nil {
				mlog.Fatalf("fetching tables failed: %+v", err)
			}
			tableNames = filterTablesByPatterns(allTables, inputTables)
		} else {
			tableNames = inputTables
		}
	} else {
		tableNames, err = db.Tables(context.TODO())
		if err != nil {
			mlog.Fatalf("fetching tables failed: %+v", err)
		}
	}
	// Table excluding.
	if in.TablesEx != "" {
		array := garray.NewStrArrayFrom(tableNames)
		for _, p := range gstr.SplitAndTrim(in.TablesEx, ",") {
			if containsWildcard(p) {
				regPattern := "^" + patternToRegex(p) + "$"
				for _, v := range array.Clone().Slice() {
					if gregex.IsMatchString(regPattern, v) {
						array.RemoveValue(v)
					}
				}
			} else {
				array.RemoveValue(p)
			}
		}
		tableNames = array.Slice()
	}

	// New table names: remove prefix then add prefix.
	removePrefixArray := gstr.SplitAndTrim(in.RemovePrefix, ",")
	newTableNames := make([]string, len(tableNames))
	for i, tableName := range tableNames {
		newTableName := tableName
		for _, v := range removePrefixArray {
			newTableName = gstr.TrimLeftStr(newTableName, v, 1)
		}
		newTableNames[i] = in.Prefix + newTableName
	}

	// Generate gooq typed table objects.
	generateGooqTable(ctx, CGenDaoInternalInput{
		CGenDaoInput:  in,
		DB:            db,
		TableNames:    tableNames,
		NewTableNames: newTableNames,
	})
}

// containsWildcard checks if the pattern contains wildcard characters (* or ?).
func containsWildcard(pattern string) bool {
	return gstr.Contains(pattern, "*") || gstr.Contains(pattern, "?")
}

// patternToRegex converts a wildcard pattern to a regex pattern.
// Wildcard characters: * matches any characters, ? matches single character.
func patternToRegex(pattern string) string {
	pattern = gstr.ReplaceByMap(pattern, map[string]string{
		"\r": "",
		"\n": "",
	})
	pattern = gstr.ReplaceByMap(pattern, map[string]string{
		"*": "\r",
		"?": "\n",
	})
	pattern = gregex.Quote(pattern)
	pattern = gstr.ReplaceByMap(pattern, map[string]string{
		"\r": ".*",
		"\n": ".",
	})
	return pattern
}

// filterTablesByPatterns filters tables by given patterns.
// Patterns support wildcard characters: * matches any characters, ? matches single character.
// https://github.com/gogf/gf/issues/4629
func filterTablesByPatterns(allTables []string, patterns []string) []string {
	var result []string
	matched := make(map[string]bool)
	allTablesSet := make(map[string]bool)
	for _, t := range allTables {
		allTablesSet[t] = true
	}
	for _, p := range patterns {
		if containsWildcard(p) {
			regPattern := "^" + patternToRegex(p) + "$"
			for _, table := range allTables {
				if !matched[table] && gregex.IsMatchString(regPattern, table) {
					result = append(result, table)
					matched[table] = true
				}
			}
		} else {
			// Exact table name, use direct string comparison.
			if !allTablesSet[p] {
				mlog.Printf(`table "%s" does not exist, skipped`, p)
				continue
			}
			if !matched[p] {
				result = append(result, p)
				matched[p] = true
			}
		}
	}
	return result
}
