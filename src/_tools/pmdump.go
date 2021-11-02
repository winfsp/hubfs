/*
 * pmdump.go
 *
 * Copyright 2021 Bill Zissimopoulos
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
	"fmt"
	"io"
	"io/fs"
	"os"
	pathutil "path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/billziss-gh/hubfs/fs/unionfs"
)

type onefs struct {
	fuse.FileSystemBase
	file *os.File
}

func (fs *onefs) Open(path string, flags int) (int, uint64) {
	return 0, 0
}

func (fs *onefs) Read(path string, buff []byte, ofst int64, fh uint64) (n int) {
	n, err := fs.file.ReadAt(buff, ofst)
	if nil != err && io.EOF != err {
		n = -fuse.EIO
	}
	return
}

func main() {
	if 2 > len(os.Args) {
		fmt.Println("usage: go run pmdump.go pathmap [dir ...]")
		os.Exit(2)
	}

	file, err := os.Open(os.Args[1])
	if nil != err {
		fmt.Fprintf(os.Stderr, "cannot open pathmap: %s\n", err)
		os.Exit(1)
	}
	defer file.Close()

	caseins := false
	if "windows" == runtime.GOOS || "darwin" == runtime.GOOS {
		caseins = true
	}

	_, pm := unionfs.OpenPathmap(&onefs{file: file}, "/.unionfs", caseins)

	for i, arg := range os.Args[1:] {
		if 0 == i {
			arg = filepath.Dir(arg)
		}
		filepath.Walk(arg, func(path string, info fs.FileInfo, err error) error {
			path, err = filepath.Rel(arg, path)
			if nil != err {
				/* do not report error */
				return nil
			}

			if "windows" == runtime.GOOS {
				path = strings.ReplaceAll(path, `\`, `/`)
			}
			path = pathutil.Join("/", path)

			pm.AddDumpPath(path)

			return nil
		})
	}

	pm.Dump(os.Stdout)
}
