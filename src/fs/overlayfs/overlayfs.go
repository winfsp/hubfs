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

type filesystem struct {
	topfs   fuse.FileSystemInterface
	split   func(path string) (string, string)
	fsnew   func(prefix string) fuse.FileSystemInterface
	caseins bool
	fsmux   sync.Mutex
	fsmap   map[string]fuse.FileSystemInterface
}

type Config struct {
	Topfs   fuse.FileSystemInterface
	Split   func(path string) (string, string)
	Fsnew   func(prefix string) fuse.FileSystemInterface
	Caseins bool
}

func New(c Config) fuse.FileSystemInterface {
	return &filesystem{
		topfs:   c.Topfs,
		split:   c.Split,
		fsnew:   c.Fsnew,
		caseins: c.Caseins,
		fsmap:   make(map[string]fuse.FileSystemInterface),
	}
}

func (fs *filesystem) destination(path string) (dstfs fuse.FileSystemInterface, remain string) {
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

func (fs *filesystem) Init() {
	fs.topfs.Init()
}

func (fs *filesystem) Destroy() {
	fs.fsmux.Lock()
	for _, fs := range fs.fsmap {
		fs.Destroy()
	}
	fs.fsmap = make(map[string]fuse.FileSystemInterface)
	fs.fsmux.Unlock()

	fs.topfs.Destroy()
}

func (fs *filesystem) Statfs(path string, stat *fuse.Statfs_t) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Statfs(path, stat)
}

func (fs *filesystem) Mknod(path string, mode uint32, dev uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Mknod(path, mode, dev)
}

func (fs *filesystem) Mkdir(path string, mode uint32) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Mkdir(path, mode)
}

func (fs *filesystem) Unlink(path string) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Unlink(path)
}

func (fs *filesystem) Rmdir(path string) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Rmdir(path)
}

func (fs *filesystem) Link(oldpath string, newpath string) (errc int) {
	oldprefix, _ := fs.split(oldpath)
	newprefix, newpath := fs.split(oldpath)
	if oldprefix != newprefix {
		return -fuse.EXDEV
	}
	dstfs, oldpath := fs.destination(oldpath)
	return dstfs.Link(oldpath, newpath)
}

func (fs *filesystem) Symlink(target string, newpath string) (errc int) {
	dstfs, newpath := fs.destination(newpath)
	return dstfs.Symlink(target, newpath)
}

func (fs *filesystem) Readlink(path string) (errc int, target string) {
	dstfs, path := fs.destination(path)
	return dstfs.Readlink(path)
}

func (fs *filesystem) Rename(oldpath string, newpath string) (errc int) {
	oldprefix, _ := fs.split(oldpath)
	newprefix, newpath := fs.split(oldpath)
	if oldprefix != newprefix {
		return -fuse.EXDEV
	}
	dstfs, oldpath := fs.destination(oldpath)
	return dstfs.Rename(oldpath, newpath)
}

func (fs *filesystem) Chmod(path string, mode uint32) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Chmod(path, mode)
}

func (fs *filesystem) Chown(path string, uid uint32, gid uint32) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Chown(path, uid, gid)
}

func (fs *filesystem) Utimens(path string, tmsp []fuse.Timespec) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Utimens(path, tmsp)
}

func (fs *filesystem) Access(path string, mask uint32) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Access(path, mask)
}

func (fs *filesystem) Create(path string, flags int, mode uint32) (errc int, fh uint64) {
	dstfs, path := fs.destination(path)
	return dstfs.Create(path, flags, mode)
}

func (fs *filesystem) Open(path string, flags int) (errc int, fh uint64) {
	dstfs, path := fs.destination(path)
	return dstfs.Open(path, flags)
}

func (fs *filesystem) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Getattr(path, stat, fh)
}

func (fs *filesystem) Truncate(path string, size int64, fh uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Truncate(path, size, fh)
}

func (fs *filesystem) Read(path string, buff []byte, ofst int64, fh uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Read(path, buff, ofst, fh)
}

func (fs *filesystem) Write(path string, buff []byte, ofst int64, fh uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Write(path, buff, ofst, fh)
}

func (fs *filesystem) Flush(path string, fh uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Flush(path, fh)
}

func (fs *filesystem) Release(path string, fh uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Release(path, fh)
}

func (fs *filesystem) Fsync(path string, datasync bool, fh uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Fsync(path, datasync, fh)
}

func (fs *filesystem) Opendir(path string) (errc int, fh uint64) {
	dstfs, path := fs.destination(path)
	return dstfs.Opendir(path)
}

func (fs *filesystem) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Readdir(path, fill, ofst, fh)
}

func (fs *filesystem) Releasedir(path string, fh uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Releasedir(path, fh)
}

func (fs *filesystem) Fsyncdir(path string, datasync bool, fh uint64) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Fsyncdir(path, datasync, fh)
}

func (fs *filesystem) Setxattr(path string, name string, value []byte, flags int) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Setxattr(path, name, value, flags)
}

func (fs *filesystem) Getxattr(path string, name string) (errc int, value []byte) {
	dstfs, path := fs.destination(path)
	return dstfs.Getxattr(path, name)
}

func (fs *filesystem) Removexattr(path string, name string) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Removexattr(path, name)
}

func (fs *filesystem) Listxattr(path string, fill func(name string) bool) (errc int) {
	dstfs, path := fs.destination(path)
	return dstfs.Listxattr(path, fill)
}

func (fs *filesystem) Chflags(path string, flags uint32) (errc int) {
	dstfs, path := fs.destination(path)
	intf, ok := dstfs.(fuse.FileSystemChflags)
	if !ok {
		return -fuse.ENOSYS
	}
	return intf.Chflags(path, flags)
}

func (fs *filesystem) Setcrtime(path string, tmsp fuse.Timespec) (errc int) {
	dstfs, path := fs.destination(path)
	intf, ok := dstfs.(fuse.FileSystemSetcrtime)
	if !ok {
		return -fuse.ENOSYS
	}
	return intf.Setcrtime(path, tmsp)
}

func (fs *filesystem) Setchgtime(path string, tmsp fuse.Timespec) (errc int) {
	dstfs, path := fs.destination(path)
	intf, ok := dstfs.(fuse.FileSystemSetchgtime)
	if !ok {
		return -fuse.ENOSYS
	}
	return intf.Setchgtime(path, tmsp)
}

var _ fuse.FileSystemInterface = (*filesystem)(nil)
var _ fuse.FileSystemChflags = (*filesystem)(nil)
var _ fuse.FileSystemSetcrtime = (*filesystem)(nil)
var _ fuse.FileSystemSetchgtime = (*filesystem)(nil)
