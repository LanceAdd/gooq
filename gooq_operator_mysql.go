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
		return fmt.Sprintf(`DATE_FORMAT(%s, %s)`, args[0], args[1]), nil, nil
	})
	OperatorFunc("COUNT", func(ctx context.Context, args ...any) (string, []any, error) {
		return fmt.Sprintf(`COUNT(%s)`, args[0]), nil, nil
	})
	OperatorFunc("SUM", func(ctx context.Context, args ...any) (string, []any, error) {
		return fmt.Sprintf(`SUM(%s)`, args[0]), nil, nil
	})
	OperatorFunc("COALESCE", func(ctx context.Context, args ...any) (string, []any, error) {
		return "COALESCE(" + strings.Join(toStrings(args), ", ") + ")", nil, nil
	})
	OperatorFunc("IFNULL", func(ctx context.Context, args ...any) (string, []any, error) {
		return fmt.Sprintf(`IFNULL(%s, %s)`, args[0], args[1]), nil, nil
	})
	OperatorFunc("RANK", func(ctx context.Context, args ...any) (string, []any, error) {
		return "RANK()", nil, nil
	})
	OperatorFunc("NOW", func(ctx context.Context, args ...any) (string, []any, error) {
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
