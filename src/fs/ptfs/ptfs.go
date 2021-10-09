// +build windows darwin linux

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

package ptfs

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/billziss-gh/hubfs/fs/port"
)

type Ptfs struct {
	fuse.FileSystemBase
	root string
}

func (self *Ptfs) Statfs(path string, stat *fuse.Statfs_t) (errc int) {
	path = filepath.Join(self.root, path)
	return port.Statfs(path, stat)
}

func (self *Ptfs) Mknod(path string, mode uint32, dev uint64) (errc int) {
	defer port.Setuidgid()()
	path = filepath.Join(self.root, path)
	return port.Mknod(path, mode, int(dev))
}

func (self *Ptfs) Mkdir(path string, mode uint32) (errc int) {
	defer port.Setuidgid()()
	path = filepath.Join(self.root, path)
	return port.Mkdir(path, mode)
}

func (self *Ptfs) Unlink(path string) (errc int) {
	path = filepath.Join(self.root, path)
	return port.Unlink(path)
}

func (self *Ptfs) Rmdir(path string) (errc int) {
	path = filepath.Join(self.root, path)
	return port.Rmdir(path)
}

func (self *Ptfs) Link(oldpath string, newpath string) (errc int) {
	defer port.Setuidgid()()
	oldpath = filepath.Join(self.root, oldpath)
	newpath = filepath.Join(self.root, newpath)
	return port.Link(oldpath, newpath)
}

func (self *Ptfs) Symlink(target string, newpath string) (errc int) {
	defer port.Setuidgid()()
	newpath = filepath.Join(self.root, newpath)
	target = filepath.Join(self.root, target)

	root := self.root
	if !strings.HasSuffix(root, string(filepath.Separator)) {
		root += string(filepath.Separator)
	}
	if !strings.HasPrefix(target, root) {
		return -fuse.EPERM
	}
	target, e := filepath.Rel(self.root, target)
	if nil != e {
		return -fuse.EPERM
	}

	return port.Symlink(target, newpath)
}

func (self *Ptfs) Readlink(path string) (errc int, target string) {
	path = filepath.Join(self.root, path)
	return port.Readlink(path)
}

func (self *Ptfs) Rename(oldpath string, newpath string) (errc int) {
	defer port.Setuidgid()()
	oldpath = filepath.Join(self.root, oldpath)
	newpath = filepath.Join(self.root, newpath)
	return port.Rename(oldpath, newpath)
}

func (self *Ptfs) Chmod(path string, mode uint32) (errc int) {
	path = filepath.Join(self.root, path)
	return port.Chmod(path, mode)
}

func (self *Ptfs) Chown(path string, uid uint32, gid uint32) (errc int) {
	path = filepath.Join(self.root, path)
	return port.Lchown(path, int(uid), int(gid))
}

func (self *Ptfs) Utimens(path string, tmsp []fuse.Timespec) (errc int) {
	path = filepath.Join(self.root, path)
	return port.UtimesNano(path, tmsp)
}

func (self *Ptfs) Create(path string, flags int, mode uint32) (errc int, fh uint64) {
	defer port.Setuidgid()()
	path = filepath.Join(self.root, path)
	return port.Open(path, flags, mode)
}

func (self *Ptfs) Open(path string, flags int) (errc int, fh uint64) {
	path = filepath.Join(self.root, path)
	return port.Open(path, flags, 0)
}

func (self *Ptfs) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	if ^uint64(0) == fh {
		if "windows" == runtime.GOOS {
			slashdot := strings.HasSuffix(path, "/.")
			path = filepath.Join(self.root, path)
			if slashdot {
				path += `\`
			}
		} else {
			path = filepath.Join(self.root, path)
		}
		return port.Lstat(path, stat)
	} else {
		return port.Fstat(fh, stat)
	}
}

func (self *Ptfs) Truncate(path string, size int64, fh uint64) (errc int) {
	if ^uint64(0) == fh {
		path = filepath.Join(self.root, path)
		errc = port.Truncate(path, size)
	} else {
		errc = port.Ftruncate(fh, size)
	}
	return
}

func (self *Ptfs) Read(path string, buff []byte, ofst int64, fh uint64) (n int) {
	return port.Pread(fh, buff, ofst)
}

func (self *Ptfs) Write(path string, buff []byte, ofst int64, fh uint64) (n int) {
	return port.Pwrite(fh, buff, ofst)
}

func (self *Ptfs) Release(path string, fh uint64) (errc int) {
	return port.Close(fh)
}

func (self *Ptfs) Fsync(path string, datasync bool, fh uint64) (errc int) {
	return port.Fsync(fh)
}

func (self *Ptfs) Opendir(path string) (errc int, fh uint64) {
	path = filepath.Join(self.root, path)
	return port.Opendir(path)
}

func (self *Ptfs) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) (errc int) {
	return port.Readdir(fh, fill)
}

func (self *Ptfs) Releasedir(path string, fh uint64) (errc int) {
	return port.Closedir(fh)
}

func NewPtfs(root string) *Ptfs {
	return &Ptfs{root: root}
}
