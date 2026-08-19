// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// Package cmd provides the management of CLI commands for `ggen` tool.
package cmd

import (
	"context"

	"github.com/gogf/gf/v2/os/gcmd"
	"github.com/gogf/gf/v2/util/gmeta"
	"github.com/gogf/gf/v2/util/gtag"
)

// GF is the management object for `ggen` command line tool.
var GF = cGF{}

type cGF struct {
	gmeta.Meta `name:"ggen" ad:"{cGFAd}"`
}

const (
	cGFAd = `
ADDITIONAL
    Use "ggen COMMAND -h" for details about a command.
`
)

func init() {
	gtag.Sets(map[string]string{
		`cGFAd`: cGFAd,
	})
}

type cGFInput struct {
	gmeta.Meta `name:"ggen"`
	Yes        bool `short:"y" name:"yes"     brief:"all yes for all command without prompt ask"   orphan:"true"`
	Debug      bool `short:"d" name:"debug"   brief:"show internal detailed debugging information" orphan:"true"`
}

type cGFOutput struct{}

func (c cGF) Index(ctx context.Context, in cGFInput) (out *cGFOutput, err error) {
	// Print help content.
	gcmd.CommandFromCtx(ctx).Print()
	return
}
