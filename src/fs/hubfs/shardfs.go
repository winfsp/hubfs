/*
 * shardfs.go
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

package hubfs

import (
	"sync"

	"github.com/billziss-gh/cgofuse/fuse"
)

type shardfs struct {
	fuse.FileSystemInterface
	topfs    *hubfs
	obs      *obstack
	keeppath string
	once     sync.Once
}

func newShardfs(topfs *hubfs, obs *obstack, fs fuse.FileSystemInterface) fuse.FileSystemInterface {
	return &shardfs{
		FileSystemInterface: fs,
		topfs:               topfs,
		obs:                 obs,
		keeppath:            "/.keep",
	}
}

func (fs *shardfs) initonce() {
	fs.once.Do(func() {
		errc, fh := fs.FileSystemInterface.Create(fs.keeppath, fuse.O_CREAT|fuse.O_RDWR, 0666)
		if -fuse.ENOSYS == errc {
			errc = fs.FileSystemInterface.Mknod(fs.keeppath, 0666, 0)
			if 0 == errc {
				errc, fh = fs.FileSystemInterface.Open(fs.keeppath, fuse.O_RDWR)
			}
		}
		if 0 == errc {
			fs.FileSystemInterface.Release(fs.keeppath, fh)
		}
	})
}

func (fs *shardfs) Destroy() {
	fs.FileSystemInterface.Destroy()
	fs.topfs.release(fs.obs)
}

func (fs *shardfs) Mknod(path string, mode uint32, dev uint64) (errc int) {
	errc = fs.FileSystemInterface.Mknod(path, mode, dev)
	if 0 == errc {
		fs.initonce()
	}
	return
}

func (fs *shardfs) Mkdir(path string, mode uint32) (errc int) {
	errc = fs.FileSystemInterface.Mkdir(path, mode)
	if 0 == errc {
		fs.initonce()
	}
	return
}

func (fs *shardfs) Unlink(path string) (errc int) {
	errc = fs.FileSystemInterface.Unlink(path)
	if 0 == errc && fs.keeppath != path {
		fs.initonce()
	}
	return
}

func (fs *shardfs) Rmdir(path string) (errc int) {
	errc = fs.FileSystemInterface.Rmdir(path)
	if 0 == errc {
		fs.initonce()
	}
	return
}

func (fs *shardfs) Link(oldpath string, newpath string) (errc int) {
	errc = fs.FileSystemInterface.Link(oldpath, newpath)
	if 0 == errc {
		fs.initonce()
	}
	return
}

func (fs *shardfs) Symlink(target string, newpath string) (errc int) {
	errc = fs.FileSystemInterface.Symlink(target, newpath)
	if 0 == errc {
		fs.initonce()
	}
	return
}

func (fs *shardfs) Rename(oldpath string, newpath string) (errc int) {
	errc = fs.FileSystemInterface.Rename(oldpath, newpath)
	if 0 == errc {
		fs.initonce()
	}
	return
}

func (fs *shardfs) Chmod(path string, mode uint32) (errc int) {
	errc = fs.FileSystemInterface.Chmod(path, mode)
	if 0 == errc {
		fs.initonce()
	}
	return
}

func (fs *shardfs) Chown(path string, uid uint32, gid uint32) (errc int) {
	errc = fs.FileSystemInterface.Chown(path, uid, gid)
	if 0 == errc {
		fs.initonce()
	}
	return
}

func (fs *shardfs) Utimens(path string, tmsp []fuse.Timespec) (errc int) {
	errc = fs.FileSystemInterface.Utimens(path, tmsp)
	if 0 == errc {
		fs.initonce()
	}
	return
}

func (fs *shardfs) Create(path string, flags int, mode uint32) (errc int, fh uint64) {
	errc, fh = fs.FileSystemInterface.Create(path, flags, mode)
	if 0 == errc {
		fs.initonce()
	}
	return
}

func (fs *shardfs) Truncate(path string, size int64, fh uint64) (errc int) {
	errc = fs.FileSystemInterface.Truncate(path, size, fh)
	if 0 == errc {
		fs.initonce()
	}
	return
}

func (fs *shardfs) Write(path string, buff []byte, ofst int64, fh uint64) (errc int) {
	errc = fs.FileSystemInterface.Write(path, buff, ofst, fh)
	if 0 == errc {
		fs.initonce()
	}
	return
}

func (fs *shardfs) Setxattr(path string, name string, value []byte, flags int) (errc int) {
	errc = fs.FileSystemInterface.Setxattr(path, name, value, flags)
	if 0 == errc {
		fs.initonce()
	}
	return
}

func (fs *shardfs) Removexattr(path string, name string) (errc int) {
	errc = fs.FileSystemInterface.Removexattr(path, name)
	if 0 == errc {
		fs.initonce()
	}
	return
}

func (fs *shardfs) Chflags(path string, flags uint32) (errc int) {
	/* lie! */
	return 0
}

func (fs *shardfs) Setcrtime(path string, tmsp fuse.Timespec) (errc int) {
	/* lie! */
	return 0
}

func (fs *shardfs) Setchgtime(path string, tmsp fuse.Timespec) (errc int) {
	/* lie! */
	return 0
}

var _ fuse.FileSystemInterface = (*shardfs)(nil)
var _ fuse.FileSystemChflags = (*shardfs)(nil)
var _ fuse.FileSystemSetcrtime = (*shardfs)(nil)
var _ fuse.FileSystemSetchgtime = (*shardfs)(nil)
