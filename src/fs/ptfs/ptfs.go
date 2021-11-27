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

type filesystem struct {
	fuse.FileSystemBase
	root string
}

func (self *filesystem) Statfs(path string, stat *fuse.Statfs_t) (errc int) {
	path = filepath.Join(self.root, path)
	return port.Statfs(path, stat)
}

func (self *filesystem) Mknod(path string, mode uint32, dev uint64) (errc int) {
	defer port.Setuidgid()()
	path = filepath.Join(self.root, path)
	return port.Mknod(path, mode, int(dev))
}

func (self *filesystem) Mkdir(path string, mode uint32) (errc int) {
	defer port.Setuidgid()()
	path = filepath.Join(self.root, path)
	return port.Mkdir(path, mode)
}

func (self *filesystem) Unlink(path string) (errc int) {
	path = filepath.Join(self.root, path)
	return port.Unlink(path)
}

func (self *filesystem) Rmdir(path string) (errc int) {
	path = filepath.Join(self.root, path)
	return port.Rmdir(path)
}

func (self *filesystem) Link(oldpath string, newpath string) (errc int) {
	defer port.Setuidgid()()
	oldpath = filepath.Join(self.root, oldpath)
	newpath = filepath.Join(self.root, newpath)
	return port.Link(oldpath, newpath)
}

func (self *filesystem) Symlink(target string, newpath string) (errc int) {
	defer port.Setuidgid()()
	newpath = filepath.Join(self.root, newpath)
	root := self.root
	if !strings.HasSuffix(root, string(filepath.Separator)) {
		root += string(filepath.Separator)
	}
	dest := filepath.Join(filepath.Dir(newpath), target)
	if !strings.HasPrefix(dest, root) {
		return -fuse.EPERM
	}
	return port.Symlink(target, newpath)
}

func (self *filesystem) Readlink(path string) (errc int, target string) {
	path = filepath.Join(self.root, path)
	return port.Readlink(path)
}

func (self *filesystem) Rename(oldpath string, newpath string) (errc int) {
	defer port.Setuidgid()()
	oldpath = filepath.Join(self.root, oldpath)
	newpath = filepath.Join(self.root, newpath)
	return port.Rename(oldpath, newpath)
}

func (self *filesystem) Chmod(path string, mode uint32) (errc int) {
	path = filepath.Join(self.root, path)
	return port.Chmod(path, mode)
}

func (self *filesystem) Chown(path string, uid uint32, gid uint32) (errc int) {
	path = filepath.Join(self.root, path)
	return port.Lchown(path, int(uid), int(gid))
}

func (self *filesystem) Utimens(path string, tmsp []fuse.Timespec) (errc int) {
	path = filepath.Join(self.root, path)
	return port.UtimesNano(path, tmsp)
}

func (self *filesystem) Create(path string, flags int, mode uint32) (errc int, fh uint64) {
	defer port.Setuidgid()()
	path = filepath.Join(self.root, path)
	return port.Open(path, flags, mode)
}

func (self *filesystem) Open(path string, flags int) (errc int, fh uint64) {
	path = filepath.Join(self.root, path)
	return port.Open(path, flags, 0)
}

func (self *filesystem) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
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

func (self *filesystem) Truncate(path string, size int64, fh uint64) (errc int) {
	if ^uint64(0) == fh {
		path = filepath.Join(self.root, path)
		errc = port.Truncate(path, size)
	} else {
		errc = port.Ftruncate(fh, size)
	}
	return
}

func (self *filesystem) Read(path string, buff []byte, ofst int64, fh uint64) (n int) {
	return port.Pread(fh, buff, ofst)
}

func (self *filesystem) Write(path string, buff []byte, ofst int64, fh uint64) (n int) {
	return port.Pwrite(fh, buff, ofst)
}

func (self *filesystem) Release(path string, fh uint64) (errc int) {
	return port.Close(fh)
}

func (self *filesystem) Fsync(path string, datasync bool, fh uint64) (errc int) {
	return port.Fsync(fh)
}

func (self *filesystem) Opendir(path string) (errc int, fh uint64) {
	path = filepath.Join(self.root, path)
	return port.Opendir(path)
}

func (self *filesystem) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) (errc int) {
	return port.Readdir(fh, fill)
}

func (self *filesystem) Releasedir(path string, fh uint64) (errc int) {
	return port.Closedir(fh)
}

func (self *filesystem) Chflags(path string, flags uint32) (errc int) {
	path = filepath.Join(self.root, path)
	return port.Lchflags(path, flags)
}

func (self *filesystem) Setcrtime(path string, tmsp fuse.Timespec) (errc int) {
	path = filepath.Join(self.root, path)
	arts := [4]fuse.Timespec{}
	arts[3] = tmsp
	return port.UtimesNano(path, arts[:])
}

func New(root string) fuse.FileSystemInterface {
	return &filesystem{root: root}
}
