// SPDX-License-Identifier: EUPL-1.2

package main

import core "github.com/dappcore/go"

// mustSub is dir inside an embedded FS (always there: it's compiled in).
func mustSub(f core.FS, dir string) core.FS {
	r := core.Sub(f, dir)
	if !r.OK {
		panic(r.Error())
	}
	return r.Value.(core.FS)
}
