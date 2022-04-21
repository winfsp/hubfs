/*
 * memfs_test.go
 *
 * Copyright 2021-2022 Bill Zissimopoulos
 */
/*
 * This file is part of Hubfs.
 *
 * You can redistribute it and/or modify it under the terms of the GNU
 * Affero General Public License version 3 as published by the Free
 * Software Foundation.
 */

package unionfs

import (
	"github.com/winfsp/cgofuse/fuse"
	"github.com/winfsp/hubfs/fs/memfs"
)

func newTestfs() fuse.FileSystemInterface {
	fuse.OptParse([]string{}, "")

	return memfs.New()
}
