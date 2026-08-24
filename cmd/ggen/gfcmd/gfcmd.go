// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// Package gfcmd provides the management of CLI commands for `ggen` tool.
package gfcmd

import (
	"context"

	"github.com/gogf/gf/v2/os/gcmd"

	"github.com/gogf/gf/cmd/ggen/internal/cmd"
	"github.com/gogf/gf/cmd/ggen/internal/mlog"
)

// Command manages the CLI command of `ggen`.
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

	if err := c.RunWithError(ctx); err != nil {
		mlog.Fatalf(`%+v`, err)
	}
}

// GetCommand retrieves and returns the root command of CLI `ggen`.
func GetCommand(ctx context.Context) (*Command, error) {
	root, err := gcmd.NewFromObject(cmd.GF)
	if err != nil {
		return nil, err
	}
	return &Command{root}, nil
}
