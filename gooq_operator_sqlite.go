// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

package gooq

import (
	"context"
	"fmt"
)

func init() {
	OperatorFunc("DATE_FORMAT", func(ctx context.Context, args ...any) (string, []any, error) {
		if len(args) != 2 {
			return "", nil, fmt.Errorf("DATE_FORMAT expects 2 arguments")
		}
		return "strftime(" + args[1].(string) + ", " + args[0].(string) + ")", nil, nil
	}, "sqlite")
}
