// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件提供生成工具函数：GoFmt 格式化生成的 go 源文件并整理 import。
package gen

import (
	"fmt"

	"golang.org/x/tools/imports"

	"github.com/gogf/gf/v2/os/gfile"
)

// GoFmt formats the source file and adds or removes import statements as necessary.
func GoFmt(path string) error {
	var formatErr error
	replaceFunc := func(path, content string) string {
		res, err := imports.Process(path, []byte(content), nil)
		if err != nil {
			if formatErr == nil {
				formatErr = fmt.Errorf("gooq: format %s: %w", path, err)
			}
			return content
		}
		return string(res)
	}

	var err error
	if gfile.IsFile(path) {
		if gfile.ExtName(path) != "go" {
			return nil
		}
		err = gfile.ReplaceFileFunc(replaceFunc, path)
	} else {
		err = gfile.ReplaceDirFunc(replaceFunc, path, "*.go", true)
	}
	if err != nil {
		return err
	}
	return formatErr
}
