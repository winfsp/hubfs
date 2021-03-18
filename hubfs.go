/*
 * hubfs.go
 *
 * Copyright 2021 Bill Zissimopoulos
 */
/*
 * This file is part of Hubfs.
 *
 * It is licensed under the MIT license. The full license text can be found
 * in the License.txt file at the root of this project.
 */

package main

import (
	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/billziss-gh/hubfs/providers"
)

type Hubfs struct {
	fuse.FileSystemBase
}

func (fs *Hubfs) Getattr(path string, stat *fuse.Stat_t, fh uint64) int {
	return -fuse.ENOSYS
}

func (fs *Hubfs) Opendir(path string) (int, uint64) {
	return -fuse.ENOSYS, ^uint64(0)
}

func (fs *Hubfs) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) int {
	return -fuse.ENOSYS
}

func (fs *Hubfs) Releasedir(path string, fh uint64) int {
	return -fuse.ENOSYS
}

func (fs *Hubfs) Open(path string, flags int) (int, uint64) {
	return -fuse.ENOSYS, ^uint64(0)
}

func (fs *Hubfs) Read(path string, buff []byte, ofst int64, fh uint64) int {
	return -fuse.ENOSYS
}

func (fs *Hubfs) Release(path string, fh uint64) int {
	return -fuse.ENOSYS
}

func Mount(client providers.Client, mntpnt string, mntopt0 []string) bool {
	mntopt := make([]string, 2*len(mntopt0))
	for i, m := range mntopt0 {
		mntopt[2*i+0] = "-o"
		mntopt[2*i+1] = m
	}

	fs := &Hubfs{}

	host := fuse.NewFileSystemHost(fs)
	host.SetCapReaddirPlus(true)
	return host.Mount(mntpnt, mntopt)
}
