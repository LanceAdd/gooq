package gen

import (
	"fmt"
	"path/filepath"

	"github.com/gogf/gf/v2/os/gfile"
	"golang.org/x/tools/imports"

	"github.com/lanceadd/gooq/cmd/gooq-gen/internal/mlog"
)

func writeGenerated(path, content string) {
	path = filepath.FromSlash(path)
	if err := gfile.PutContents(path, content); err != nil {
		mlog.Fatalf("writing content to '%s' failed: %v", path, err)
	}
	if err := GoFmt(path); err != nil {
		mlog.Fatalf(`formatting "%s" failed: %v`, path, err)
	}
	mlog.Print("generated:", gfile.RealPath(path))
}

// GoFmt 格式化源文件，并按需增删 import。
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
