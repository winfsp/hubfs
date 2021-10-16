/*
 * overlayfs.go
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

package overlayfs

import (
	"strings"
	"sync"

	"github.com/billziss-gh/cgofuse/fuse"
)

type Overlayfs struct {
	topfs   fuse.FileSystemInterface
	split   func(path string) (string, string)
	fsnew   func(prefix string) fuse.FileSystemInterface
	caseins bool
	fsmux   sync.Mutex
	fsmap   map[string]fuse.FileSystemInterface
}

func NewOverlayfs(
	topfs fuse.FileSystemInterface,
	split func(path string) (string, string),
	fsnew func(prefix string) fuse.FileSystemInterface,
	caseins bool) *Overlayfs {

	return &Overlayfs{
		topfs:   topfs,
		split:   split,
		fsnew:   fsnew,
		caseins: caseins,
		fsmap:   make(map[string]fuse.FileSystemInterface),
	}
}

func (fs *Overlayfs) destination(path string) (dstfs fuse.FileSystemInterface, remain string) {
	prefix, remain := fs.split(path)
	if "" == prefix {
		dstfs = fs.topfs
		return
	}

	if fs.caseins {
		prefix = strings.ToUpper(prefix)
	}

	fs.fsmux.Lock()
	dstfs = fs.fsmap[prefix]
	if nil == dstfs {
		dstfs = fs.fsnew(prefix)
		fs.fsmap[prefix] = dstfs
		dstfs.Init()
	}
	fs.fsmux.Unlock()
	return
}

func (fs *Overlayfs) Init() {
	fs.topfs.Init()
}

func (fs *Overlayfs) Destroy() {
	fs.fsmux.Lock()
	for _, fs := range fs.fsmap {
		fs.Destroy()
	}
	fs.fsmap = make(map[string]fuse.FileSystemInterface)
	fs.fsmux.Unlock()

	fs.topfs.Destroy()
}

func (fs *Overlayfs) Statfs(path string, stat *fuse.Statfs_t) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Statfs(path, stat)
}

func (fs *Overlayfs) Mknod(path string, mode uint32, dev uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Mknod(path, mode, dev)
}

func (fs *Overlayfs) Mkdir(path string, mode uint32) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Mkdir(path, mode)
}

func (fs *Overlayfs) Unlink(path string) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Unlink(path)
}

func (fs *Overlayfs) Rmdir(path string) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Rmdir(path)
}

func (fs *Overlayfs) Link(oldpath string, newpath string) (errc int) {
	return -fuse.ENOSYS
}

func (fs *Overlayfs) Symlink(target string, newpath string) (errc int) {
	dstfs, newpath := fs.destination(newpath)
	return dstfs.Symlink(target, newpath)
}

func (fs *Overlayfs) Readlink(path string) (errc int, target string) {
	dstfs, path := fs.destination(path)
	return dstfs.Readlink(path)
}

func (fs *Overlayfs) Rename(oldpath string, newpath string) (errc int) {
	return -fuse.ENOSYS
}

func (fs *Overlayfs) Chmod(path string, mode uint32) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Chmod(path, mode)
}

func (fs *Overlayfs) Chown(path string, uid uint32, gid uint32) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Chown(path, uid, gid)
}

func (fs *Overlayfs) Utimens(path string, tmsp []fuse.Timespec) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Utimens(path, tmsp)
}

func (fs *Overlayfs) Access(path string, mask uint32) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Access(path, mask)
}

func (fs *Overlayfs) Create(path string, flags int, mode uint32) (errc int, fh uint64) {
	dstfs, path := fs.destination(path)
	return dstfs.Create(path, flags, mode)
}

func (fs *Overlayfs) Open(path string, flags int) (errc int, fh uint64) {
	dstfs, path := fs.destination(path)
	return dstfs.Open(path, flags)
}

func (fs *Overlayfs) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Getattr(path, stat, fh)
}

func (fs *Overlayfs) Truncate(path string, size int64, fh uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Truncate(path, size, fh)
}

func (fs *Overlayfs) Read(path string, buff []byte, ofst int64, fh uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Read(path, buff, ofst, fh)
}

func (fs *Overlayfs) Write(path string, buff []byte, ofst int64, fh uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Write(path, buff, ofst, fh)
}

func (fs *Overlayfs) Flush(path string, fh uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Flush(path, fh)
}

func (fs *Overlayfs) Release(path string, fh uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Release(path, fh)
}

func (fs *Overlayfs) Fsync(path string, datasync bool, fh uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Fsync(path, datasync, fh)
}

func (fs *Overlayfs) Opendir(path string) (errc int, fh uint64) {
	dstfs, path := fs.destination(path)
	return dstfs.Opendir(path)
}

func (fs *Overlayfs) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Readdir(path, fill, ofst, fh)
}

func (fs *Overlayfs) Releasedir(path string, fh uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Releasedir(path, fh)
}

func (fs *Overlayfs) Fsyncdir(path string, datasync bool, fh uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Fsyncdir(path, datasync, fh)
}

func (fs *Overlayfs) Setxattr(path string, name string, value []byte, flags int) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Setxattr(path, name, value, flags)
}

func (fs *Overlayfs) Getxattr(path string, name string) (errc int, value []byte) {
	dstfs, path := fs.destination(path)
	return dstfs.Getxattr(path, name)
}

func (fs *Overlayfs) Removexattr(path string, name string) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Removexattr(path, name)
}

func (fs *Overlayfs) Listxattr(path string, fill func(name string) bool) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Listxattr(path, fill)
}

func (fs *Overlayfs) Chflags(path string, flags uint32) (errc int) {
	dstfs, path := fs.destination(path)
	intf, ok := dstfs.(fuse.FileSystemChflags)
	if !ok {
		return -fuse.ENOSYS
	}
	return intf.Chflags(path, flags)
}

func (fs *Overlayfs) Setcrtime(path string, tmsp fuse.Timespec) (errc int) {
	dstfs, path := fs.destination(path)
	intf, ok := dstfs.(fuse.FileSystemSetcrtime)
	if !ok {
		return -fuse.ENOSYS
	}
	return intf.Setcrtime(path, tmsp)
}

func (fs *Overlayfs) Setchgtime(path string, tmsp fuse.Timespec) (errc int) {
	dstfs, path := fs.destination(path)
	intf, ok := dstfs.(fuse.FileSystemSetchgtime)
	if !ok {
		return -fuse.ENOSYS
	}
	return intf.Setchgtime(path, tmsp)
}

var _ fuse.FileSystemInterface = (*Overlayfs)(nil)
var _ fuse.FileSystemChflags = (*Overlayfs)(nil)
var _ fuse.FileSystemSetcrtime = (*Overlayfs)(nil)
var _ fuse.FileSystemSetchgtime = (*Overlayfs)(nil)
