/*
 * overlayfs.go
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

package overlayfs

import (
	"strings"
	"sync"
	"time"

	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/billziss-gh/hubfs/fs/nullfs"
)

type filesystem struct {
	topfs   *shardfs
	split   func(path string) (string, string)
	newfs   func(prefix string) fuse.FileSystemInterface
	caseins bool
	ttl     time.Duration
	fsmux   sync.Mutex
	fsmap   map[string]*shardfs
	nullfs  *shardfs
}

type shardfs struct {
	fuse.FileSystemInterface
	prefix string
	rc     int
	timer  *time.Timer
}

type Config struct {
	Topfs      fuse.FileSystemInterface
	Split      func(path string) (string, string)
	Newfs      func(prefix string) fuse.FileSystemInterface
	Caseins    bool
	TimeToLive time.Duration
}

func New(c Config) fuse.FileSystemInterface {
	return &filesystem{
		topfs:   &shardfs{FileSystemInterface: c.Topfs, rc: -1},
		split:   c.Split,
		newfs:   c.Newfs,
		caseins: c.Caseins,
		ttl:     c.TimeToLive,
		fsmap:   make(map[string]*shardfs),
		nullfs:  &shardfs{FileSystemInterface: nullfs.New(), rc: -1},
	}
}

func (fs *filesystem) acquirefs(path string, delta int) (dstfs *shardfs, remain string) {
	prefix, remain := fs.split(path)
	if "" == prefix {
		dstfs = fs.topfs
		return
	}

	csprefix := prefix
	if fs.caseins {
		prefix = strings.ToUpper(prefix)
	}

	fs.fsmux.Lock()
	dstfs = fs.fsmap[prefix]
	if nil == dstfs {
		if newfs := fs.newfs(csprefix); nil != newfs {
			dstfs = &shardfs{FileSystemInterface: newfs, prefix: prefix}
			fs.fsmap[prefix] = dstfs
			dstfs.Init()
			dstfs.rc += delta
		} else {
			dstfs = fs.nullfs
		}
	} else {
		dstfs.rc += delta
	}
	fs.fsmux.Unlock()
	return
}

func (fs *filesystem) releasefs(dstfs *shardfs, delta int, errc *int) {
	if (nil == errc || 0 != *errc) &&
		!(0 > dstfs.rc) /* high bit of dstfs.rc is stable in presence of multiple threads */ {
		fs.fsmux.Lock()
		dstfs.rc += delta
		if 0 == dstfs.rc {
			if 0 == fs.ttl {
				dstfs.Destroy()
				delete(fs.fsmap, dstfs.prefix)
			} else {
				if nil == dstfs.timer {
					dstfs.timer = time.AfterFunc(fs.ttl, func() {
						fs._expirefs(dstfs)
					})
				} else {
					dstfs.timer.Reset(fs.ttl)
				}
			}
		}
		fs.fsmux.Unlock()
	}
}

func (fs *filesystem) _expirefs(dstfs *shardfs) {
	fs.fsmux.Lock()
	if 0 == dstfs.rc && dstfs == fs.fsmap[dstfs.prefix] {
		dstfs.Destroy()
		delete(fs.fsmap, dstfs.prefix)
	}
	fs.fsmux.Unlock()
}

func (fs *filesystem) Init() {
	fs.topfs.Init()
}

func (fs *filesystem) Destroy() {
	fs.fsmux.Lock()
	for _, fs := range fs.fsmap {
		fs.Destroy()
	}
	fs.fsmap = make(map[string]*shardfs)
	fs.fsmux.Unlock()

	fs.topfs.Destroy()
}

func (fs *filesystem) Statfs(path string, stat *fuse.Statfs_t) (errc int) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Statfs(path, stat)
}

func (fs *filesystem) Mknod(path string, mode uint32, dev uint64) (errc int) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Mknod(path, mode, dev)
}

func (fs *filesystem) Mkdir(path string, mode uint32) (errc int) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Mkdir(path, mode)
}

func (fs *filesystem) Unlink(path string) (errc int) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Unlink(path)
}

func (fs *filesystem) Rmdir(path string) (errc int) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Rmdir(path)
}

func (fs *filesystem) Link(oldpath string, newpath string) (errc int) {
	oldprefix, _ := fs.split(oldpath)
	newprefix, newpath := fs.split(newpath)
	if fs.caseins {
		if strings.ToUpper(oldprefix) != strings.ToUpper(newprefix) {
			return -fuse.EXDEV
		}
	} else {
		if oldprefix != newprefix {
			return -fuse.EXDEV
		}
	}
	dstfs, oldpath := fs.acquirefs(oldpath, +1)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Link(oldpath, newpath)
}

func (fs *filesystem) Symlink(target string, newpath string) (errc int) {
	dstfs, newpath := fs.acquirefs(newpath, +1)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Symlink(target, newpath)
}

func (fs *filesystem) Readlink(path string) (errc int, target string) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Readlink(path)
}

func (fs *filesystem) Rename(oldpath string, newpath string) (errc int) {
	oldprefix, _ := fs.split(oldpath)
	newprefix, newpath := fs.split(newpath)
	if fs.caseins {
		if strings.ToUpper(oldprefix) != strings.ToUpper(newprefix) {
			return -fuse.EXDEV
		}
	} else {
		if oldprefix != newprefix {
			return -fuse.EXDEV
		}
	}
	dstfs, oldpath := fs.acquirefs(oldpath, +1)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Rename(oldpath, newpath)
}

func (fs *filesystem) Chmod(path string, mode uint32) (errc int) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Chmod(path, mode)
}

func (fs *filesystem) Chown(path string, uid uint32, gid uint32) (errc int) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Chown(path, uid, gid)
}

func (fs *filesystem) Utimens(path string, tmsp []fuse.Timespec) (errc int) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Utimens(path, tmsp)
}

func (fs *filesystem) Access(path string, mask uint32) (errc int) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Access(path, mask)
}

func (fs *filesystem) Create(path string, flags int, mode uint32) (errc int, fh uint64) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, &errc)
	return dstfs.Create(path, flags, mode)
}

func (fs *filesystem) Open(path string, flags int) (errc int, fh uint64) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, &errc)
	return dstfs.Open(path, flags)
}

func (fs *filesystem) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Getattr(path, stat, fh)
}

func (fs *filesystem) Truncate(path string, size int64, fh uint64) (errc int) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Truncate(path, size, fh)
}

func (fs *filesystem) Read(path string, buff []byte, ofst int64, fh uint64) (errc int) {
	dstfs, path := fs.acquirefs(path, 0)
	return dstfs.Read(path, buff, ofst, fh)
}

func (fs *filesystem) Write(path string, buff []byte, ofst int64, fh uint64) (errc int) {
	dstfs, path := fs.acquirefs(path, 0)
	return dstfs.Write(path, buff, ofst, fh)
}

func (fs *filesystem) Flush(path string, fh uint64) (errc int) {
	dstfs, path := fs.acquirefs(path, 0)
	return dstfs.Flush(path, fh)
}

func (fs *filesystem) Release(path string, fh uint64) (errc int) {
	dstfs, path := fs.acquirefs(path, 0)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Release(path, fh)
}

func (fs *filesystem) Fsync(path string, datasync bool, fh uint64) (errc int) {
	dstfs, path := fs.acquirefs(path, 0)
	return dstfs.Fsync(path, datasync, fh)
}

func (fs *filesystem) Opendir(path string) (errc int, fh uint64) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, &errc)
	return dstfs.Opendir(path)
}

func (fs *filesystem) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) (errc int) {
	dstfs, path := fs.acquirefs(path, 0)
	return dstfs.Readdir(path, fill, ofst, fh)
}

func (fs *filesystem) Releasedir(path string, fh uint64) (errc int) {
	dstfs, path := fs.acquirefs(path, 0)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Releasedir(path, fh)
}

func (fs *filesystem) Fsyncdir(path string, datasync bool, fh uint64) (errc int) {
	dstfs, path := fs.acquirefs(path, 0)
	return dstfs.Fsyncdir(path, datasync, fh)
}

func (fs *filesystem) Setxattr(path string, name string, value []byte, flags int) (errc int) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Setxattr(path, name, value, flags)
}

func (fs *filesystem) Getxattr(path string, name string) (errc int, value []byte) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Getxattr(path, name)
}

func (fs *filesystem) Removexattr(path string, name string) (errc int) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Removexattr(path, name)
}

func (fs *filesystem) Listxattr(path string, fill func(name string) bool) (errc int) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, nil)
	return dstfs.Listxattr(path, fill)
}

func (fs *filesystem) Chflags(path string, flags uint32) (errc int) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, nil)
	intf, ok := dstfs.FileSystemInterface.(fuse.FileSystemChflags)
	if !ok {
		return -fuse.ENOSYS
	}
	return intf.Chflags(path, flags)
}

func (fs *filesystem) Setcrtime(path string, tmsp fuse.Timespec) (errc int) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, nil)
	intf, ok := dstfs.FileSystemInterface.(fuse.FileSystemSetcrtime)
	if !ok {
		return -fuse.ENOSYS
	}
	return intf.Setcrtime(path, tmsp)
}

func (fs *filesystem) Setchgtime(path string, tmsp fuse.Timespec) (errc int) {
	dstfs, path := fs.acquirefs(path, +1)
	defer fs.releasefs(dstfs, -1, nil)
	intf, ok := dstfs.FileSystemInterface.(fuse.FileSystemSetchgtime)
	if !ok {
		return -fuse.ENOSYS
	}
	return intf.Setchgtime(path, tmsp)
}

var _ fuse.FileSystemInterface = (*filesystem)(nil)
var _ fuse.FileSystemChflags = (*filesystem)(nil)
var _ fuse.FileSystemSetcrtime = (*filesystem)(nil)
var _ fuse.FileSystemSetchgtime = (*filesystem)(nil)
