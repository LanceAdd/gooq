// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

package gooq

import (
	"fmt"
	"strings"
)

func init() {
	OperatorFunc(FuncDateFormat, func(args ...any) (string, []any, error) {
		if len(args) != 2 {
			return "", nil, fmt.Errorf("DATE_FORMAT expects 2 arguments")
		}
		return fmt.Sprintf(`DATE_FORMAT(%s, %s)`, args[0], args[1]), nil, nil
	})
	OperatorFunc(FuncCount, func(args ...any) (string, []any, error) {
		return fmt.Sprintf(`COUNT(%s)`, args[0]), nil, nil
	})
	OperatorFunc(FuncSum, func(args ...any) (string, []any, error) {
		return fmt.Sprintf(`SUM(%s)`, args[0]), nil, nil
	})
	OperatorFunc(FuncCoalesce, func(args ...any) (string, []any, error) {
		return "COALESCE(" + strings.Join(toStrings(args), ", ") + ")", nil, nil
	})
	OperatorFunc(FuncIfNull, func(args ...any) (string, []any, error) {
		return fmt.Sprintf(`IFNULL(%s, %s)`, args[0], args[1]), nil, nil
	})
	OperatorFunc(FuncRank, func(args ...any) (string, []any, error) {
		return "RANK()", nil, nil
	})
	OperatorFunc(FuncNow, func(args ...any) (string, []any, error) {
		return "NOW()", nil, nil
	})
}

func toStrings(args []any) []string {
	result := make([]string, len(args))
	for i, a := range args {
		result[i] = fmt.Sprintf(`%v`, a)
	}
	return result
}
