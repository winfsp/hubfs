/*
 * unionfs.go
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

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/billziss-gh/hubfs/fs/port"
	"github.com/billziss-gh/hubfs/fs/ptfs"
	"github.com/billziss-gh/hubfs/fs/unionfs"
)

func main() {
	port.Umask(0)

	args := os.Args
	bpos := 1
	epos := len(args)
	for i := 1; epos > i; i++ {
		if strings.HasPrefix(args[i], "-") {
			bpos = i + 1
		}
	}
	if epos > bpos {
		epos--
	}

	root := append([]string{}, args[bpos:epos]...)
	args = append(args[:bpos], args[epos:]...)
	if len(root) == 0 {
		root = []string{"."}
	}

	fslist := make([]fuse.FileSystemInterface, 0, len(root))
	for _, r := range root {
		r, err := filepath.Abs(r)
		if nil != err {
			panic(err)
		}
		fslist = append(fslist, ptfs.New(r))

	}

	caseins := false
	if "windows" == runtime.GOOS || "darwin" == runtime.GOOS {
		caseins = true
	}

	unfs := unionfs.New(unionfs.Config{Fslist: fslist, Caseins: caseins})
	host := fuse.NewFileSystemHost(unfs)
	host.SetCapReaddirPlus(true)
	host.SetCapCaseInsensitive(caseins)
	host.Mount("", args[1:])
}
