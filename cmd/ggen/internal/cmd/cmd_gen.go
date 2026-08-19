// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

package cmd

import (
	"github.com/gogf/gf/v2/util/gmeta"
	"github.com/gogf/gf/v2/util/gtag"
)

var (
	Gen = cGen{}
)

type cGen struct {
	gmeta.Meta `name:"gen" brief:"{cGenBrief}" dc:"{cGenDc}"`
	cGenDao
}

const (
	cGenBrief = `automatically generate gooq table objects for database tables`
	cGenDc    = `
The "gen" command is designed for generating gooq typed table objects.
Please use "ggen gen dao -h" for specified type help.
`
)

func init() {
	gtag.Sets(map[string]string{
		`cGenBrief`: cGenBrief,
		`cGenDc`:    cGenDc,
	})
}
