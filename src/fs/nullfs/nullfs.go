/*
 * nullfs.go
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

package nullfs

import (
	"github.com/billziss-gh/cgofuse/fuse"
)

type filesystem struct {
}

func New() fuse.FileSystemInterface {
	return &filesystem{}
}

func (fs *filesystem) Init() {
}

func (fs *filesystem) Destroy() {
}

func (fs *filesystem) Statfs(path string, stat *fuse.Statfs_t) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Mknod(path string, mode uint32, dev uint64) (errc int) {
	return -fuse.EPERM
}

func (fs *filesystem) Mkdir(path string, mode uint32) (errc int) {
	return -fuse.EPERM
}

func (fs *filesystem) Unlink(path string) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Rmdir(path string) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Link(oldpath string, newpath string) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Symlink(target string, newpath string) (errc int) {
	return -fuse.EPERM
}

func (fs *filesystem) Readlink(path string) (errc int, target string) {
	return -fuse.ENOENT, ""
}

func (fs *filesystem) Rename(oldpath string, newpath string) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Chmod(path string, mode uint32) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Chown(path string, uid uint32, gid uint32) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Utimens(path string, tmsp []fuse.Timespec) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Access(path string, mask uint32) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Create(path string, flags int, mode uint32) (errc int, fh uint64) {
	return -fuse.EPERM, ^uint64(0)
}

func (fs *filesystem) Open(path string, flags int) (errc int, fh uint64) {
	return -fuse.ENOENT, ^uint64(0)
}

func (fs *filesystem) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Truncate(path string, size int64, fh uint64) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Read(path string, buff []byte, ofst int64, fh uint64) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Write(path string, buff []byte, ofst int64, fh uint64) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Flush(path string, fh uint64) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Release(path string, fh uint64) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Fsync(path string, datasync bool, fh uint64) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Opendir(path string) (errc int, fh uint64) {
	return -fuse.ENOENT, ^uint64(0)
}

func (fs *filesystem) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Releasedir(path string, fh uint64) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Fsyncdir(path string, datasync bool, fh uint64) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Setxattr(path string, name string, value []byte, flags int) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Getxattr(path string, name string) (errc int, value []byte) {
	return -fuse.ENOENT, nil
}

func (fs *filesystem) Removexattr(path string, name string) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Listxattr(path string, fill func(name string) bool) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Chflags(path string, flags uint32) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Setcrtime(path string, tmsp fuse.Timespec) (errc int) {
	return -fuse.ENOENT
}

func (fs *filesystem) Setchgtime(path string, tmsp fuse.Timespec) (errc int) {
	return -fuse.ENOENT
}

var _ fuse.FileSystemInterface = (*filesystem)(nil)
var _ fuse.FileSystemChflags = (*filesystem)(nil)
var _ fuse.FileSystemSetcrtime = (*filesystem)(nil)
var _ fuse.FileSystemSetchgtime = (*filesystem)(nil)
