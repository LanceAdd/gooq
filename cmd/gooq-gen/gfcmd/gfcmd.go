// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// Package gfcmd provides the management of CLI commands for `gooq-gen` tool.
package gfcmd

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/gcmd"
	"github.com/gogf/gf/v2/os/gfile"

	"github.com/lanceadd/gooq/cmd/gooq-gen/internal/cmd"
	"github.com/lanceadd/gooq/cmd/gooq-gen/internal/mlog"
)

const (
	cliFolderName = `hack`
	cliConfigName = `gooq.yaml`
)

// Command manages the CLI command of `gooq-gen`.
// This struct can be globally accessible and extended with custom struct.
type Command struct {
	*gcmd.Command
}

// Run starts running the command according the command line arguments and options.
func (c *Command) Run(ctx context.Context) {
	defer func() {
		if err := recover(); err != nil {
			mlog.Print(err)
		}
	}()

	// Configuration, using the `hack/gooq.yaml` in priority.
	if path, _ := gfile.Search(cliFolderName); path != "" {
		if adapter, ok := g.Cfg().GetAdapter().(*gcfg.AdapterFile); ok {
			if err := adapter.SetPath(path); err != nil {
				mlog.Fatal(err)
			}
			adapter.SetFileName(cliConfigName)
		}
	}

	if err := c.RunWithError(ctx); err != nil {
		mlog.Fatalf(`%+v`, err)
	}
}

// GetCommand retrieves and returns the root command of CLI `gooq-gen`.
func GetCommand(ctx context.Context) (*Command, error) {
	root, err := gcmd.NewFromObject(cmd.GF)
	if err != nil {
		return nil, err
	}
	return &Command{root}, nil
}
