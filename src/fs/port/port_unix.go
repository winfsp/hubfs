// +build darwin linux

/*
 * port_unix.go
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

package port

import (
	"syscall"

	"github.com/billziss-gh/cgofuse/fuse"
)

func Chdir(path string) (errc int) {
	return Errno(syscall.Chdir(path))
}

func Statfs(path string, stat *fuse.Statfs_t) (errc int) {
	gost := syscall.Statfs_t{}
	errc = Errno(syscall.Statfs(path, &gost))
	copyFusestatfsFromGostatfs(stat, &gost)
	return
}

func Mknod(path string, mode uint32, dev int) (errc int) {
	return Errno(syscall.Mknod(path, mode, dev))
}

func Mkdir(path string, mode uint32) (errc int) {
	return Errno(syscall.Mkdir(path, mode))
}

func Unlink(path string) (errc int) {
	return Errno(syscall.Unlink(path))
}

func Rmdir(path string) (errc int) {
	return Errno(syscall.Rmdir(path))
}

func Link(oldpath string, newpath string) (errc int) {
	return Errno(syscall.Link(oldpath, newpath))
}

func Symlink(oldpath string, newpath string) (errc int) {
	return Errno(syscall.Symlink(oldpath, newpath))
}

func Readlink(path string) (errc int, target string) {
	buf := [4096]byte{}
	n, e := syscall.Readlink(path, buf[:])
	if nil != e {
		return Errno(e), ""
	}
	return 0, string(buf[:n])
}

func Rename(oldpath string, newpath string) (errc int) {
	return Errno(syscall.Rename(oldpath, newpath))
}

func Chmod(path string, mode uint32) (errc int) {
	return Errno(syscall.Chmod(path, mode))
}

func Lchown(path string, uid int, gid int) (errc int) {
	return Errno(syscall.Lchown(path, uid, gid))
}

func UtimesNano(path string, tmsp []fuse.Timespec) (errc int) {
	gots := [2]syscall.Timespec{}
	gots[0].Sec, gots[0].Nsec = tmsp[0].Sec, tmsp[0].Nsec
	gots[1].Sec, gots[1].Nsec = tmsp[1].Sec, tmsp[1].Nsec
	return Errno(syscall.UtimesNano(path, gots[:]))
}

func Open(path string, flags int, mode uint32) (errc int, fh uint64) {
	fd, e := syscall.Open(path, flags, mode)
	if nil != e {
		return Errno(e), ^uint64(0)
	}
	return 0, uint64(fd)
}

func Lstat(path string, stat *fuse.Stat_t) (errc int) {
	gost := syscall.Stat_t{}
	errc = Errno(syscall.Lstat(path, &gost))
	copyFusestatFromGostat(stat, &gost)
	return
}

func Fstat(fh uint64, stat *fuse.Stat_t) (errc int) {
	gost := syscall.Stat_t{}
	errc = Errno(syscall.Fstat(int(fh), &gost))
	copyFusestatFromGostat(stat, &gost)
	return
}

func Truncate(path string, length int64) (errc int) {
	return Errno(syscall.Truncate(path, length))
}

func Ftruncate(fh uint64, length int64) (errc int) {
	return Errno(syscall.Ftruncate(int(fh), length))
}

func Pread(fh uint64, p []byte, offset int64) (n int) {
	n, e := syscall.Pread(int(fh), p, offset)
	if nil != e {
		return Errno(e)
	}

	return n
}

func Pwrite(fh uint64, p []byte, offset int64) (n int) {
	n, e := syscall.Pwrite(int(fh), p, offset)
	if nil != e {
		return Errno(e)
	}

	return n
}

func Close(fh uint64) (errc int) {
	return Errno(syscall.Close(int(fh)))
}

func Fsync(fh uint64) (errc int) {
	return Errno(syscall.Fsync(int(fh)))
}

func Opendir(path string) (errc int, fh uint64) {
	fd, e := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY, 0)
	if nil != e {
		return Errno(e), ^uint64(0)
	}

	return 0, uint64(fd)
}

func Readdir(fh uint64, fill func(name string, stat *fuse.Stat_t, ofst int64) bool) (errc int) {
	buf := [8 * 1024]byte{}
	ptr := 0
	end := 0

	for {
		if end <= ptr {
			ptr = 0
			var e error
			end, e = syscall.ReadDirent(int(fh), buf[:])
			if nil != e {
				return Errno(e)
			}
			if 0 >= end {
				return 0
			}
		}

		n, _, names := syscall.ParseDirent(buf[ptr:end], -1, nil)
		ptr += n

		for _, name := range names {
			if !fill(name, nil, 0) {
				return 0
			}
		}
	}
}

func Closedir(fh uint64) (errc int) {
	return Errno(syscall.Close(int(fh)))
}

func Umask(mask int) (oldmask int) {
	return syscall.Umask(mask)
}

func Errno(err error) int {
	if nil == err {
		return 0
	}

	if e, ok := err.(syscall.Errno); ok {
		return -int(e)
	}

	return -fuse.EIO
}
