/*
 * ptfs.go
 *
 * Copyright 2017-2021 Bill Zissimopoulos
 */
/*
 * This file is part of Hubfs.
 *
 * You can redistribute it and/or modify it under the terms of the GNU
 * Affero General Public License version 3 as published by the Free
 * Software Foundation.
 */

package main

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/billziss-gh/hubfs/fs/port"
	"github.com/billziss-gh/hubfs/fs/ptfs"
)

func main() {
	port.Umask(0)

	args := os.Args
	root := "."
	if 3 <= len(args) && '-' != args[len(args)-2][0] && '-' != args[len(args)-1][0] {
		root = args[len(args)-2]
		args = append(args[:len(args)-2], args[len(args)-1])
	}
	root, err := filepath.Abs(root)
	if nil != err {
		root = "."
	}

	ptfs := ptfs.NewPtfs(root)
	host := fuse.NewFileSystemHost(ptfs)
	if "windows" == runtime.GOOS {
		host.SetCapCaseInsensitive(true)
		host.SetCapReaddirPlus(true)
	}
	host.Mount("", args[1:])
}
