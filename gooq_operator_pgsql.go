// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

package gooq

import (
	"context"
	"fmt"
	"strings"
)

func init() {
	OperatorFunc("DATE_FORMAT", func(ctx context.Context, args ...any) (string, []any, error) {
		if len(args) != 2 {
			return "", nil, fmt.Errorf("DATE_FORMAT expects 2 arguments")
		}
		pgFormat := mysqlToPgFormat(trimQuotes(args[1].(string)))
		return "TO_CHAR(" + args[0].(string) + ", '" + pgFormat + "')", nil, nil
	}, "pgsql")
}

func trimQuotes(s string) string {
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return s
}

func mysqlToPgFormat(format string) string {
	var replacer = strings.NewReplacer(
		"%Y", "YYYY", "%y", "YY",
		"%m", "MM", "%d", "DD",
		"%H", "HH24", "%i", "MI", "%s", "SS",
		"%e", "FMDD", "%j", "DDD",
	)
	return replacer.Replace(format)
}
