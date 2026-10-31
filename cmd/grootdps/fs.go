// SPDX-License-Identifier: EUPL-1.2

package main

import "io/fs"

func mustSub(f fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
