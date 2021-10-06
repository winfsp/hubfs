/*
 * unionfs.go
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

package fs

import (
	pathutil "path"
	"sort"
	"sync"

	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/billziss-gh/hubfs/fs/union"
)

type Unionfs struct {
	fslist  []fuse.FileSystemInterface // file system list
	nsmux   sync.RWMutex               // namespace mutex
	pathmux sync.Mutex                 // path map mutex
	pathmap *union.Pathmap             // path map
	filemux sync.Mutex                 // open file mutex
	filemap *union.Filemap             // open file map

	// lock hierarchy:
	//     nsmux -> pathmux
	//     nsmux -> filemux
}

type file struct {
	isopq bool
	v     uint8
	fh    uint64
	flags int
}

func NewUnionfs(fslist []fuse.FileSystemInterface, caseins bool) *Unionfs {
	if 0 == len(fslist) {
		fslist = []fuse.FileSystemInterface{&fuse.FileSystemBase{}}
	}

	fs := &Unionfs{}
	fs.fslist = append(fs.fslist, fslist...)
	_, fs.pathmap = union.OpenPathmap(fs.fslist[0], "/.unionfs", caseins)
	if nil == fs.pathmap {
		return nil
	}
	fs.filemap = union.NewFilemap(fs, caseins)

	return fs
}

func (fs *Unionfs) getvis(path string, stat *fuse.Stat_t) (errc int, isopq bool, v uint8) {
	fs.pathmux.Lock()
	isopq, v = fs.pathmap.Get(path)
	fs.pathmux.Unlock()

	if union.UNKNOWN == v {
		u := union.NOTEXIST
		var s fuse.Stat_t
		for i, fs := range fs.fslist {
			e := fs.Getattr(path, &s, ^uint64(0))
			if 0 == e {
				u = uint8(i)
				break
			}
			if isopq {
				break
			}
		}

		fs.pathmux.Lock()
		isopq, v = fs.pathmap.Get(path)
		if union.UNKNOWN == v {
			fs.pathmap.Set(path, u)
			fs.pathmux.Unlock()
			if union.NOTEXIST == u {
				return -fuse.ENOENT, isopq, union.NOTEXIST
			}
			if nil != stat {
				*stat = s
			}
			return 0, isopq, u
		}
		fs.pathmux.Unlock()
	}

	switch v {
	case union.NOTEXIST, union.WHITEOUT:
		errc = -fuse.ENOENT
	default:
		if nil != stat {
			errc = fs.fslist[v].Getattr(path, stat, ^uint64(0))
			if 0 != errc {
				v = union.NOTEXIST
			}
		} else {
			errc = 0
		}
	}

	return
}

func (fs *Unionfs) hasvis(path string) (res bool) {
	fs.pathmux.Lock()
	res = fs.pathmap.Has(path)
	fs.pathmux.Unlock()
	return
}

func (fs *Unionfs) setvis(path string, v uint8) {
	fs.pathmux.Lock()
	fs.pathmap.Set(path, v)
	fs.pathmux.Unlock()
}

func (fs *Unionfs) setvisif(path string, v uint8) {
	fs.pathmux.Lock()
	fs.pathmap.SetIf(path, v)
	fs.pathmux.Unlock()
}

func (fs *Unionfs) lsdir(path string,
	isopq bool, v uint8, fh uint64,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool) {

	type dirent struct {
		stat *fuse.Stat_t
		v    uint8
	}
	dirmap := make(map[string]dirent)
	dirfill := func(name string, stat *fuse.Stat_t, ofst int64) bool {
		if _, ok := dirmap[name]; ok {
			return true
		}
		if nil != stat {
			s := *stat
			stat = &s
		}
		dirmap[name] = dirent{stat, v}
		return true
	}

	if ^uint64(0) != fh {
		fs.fslist[v].Readdir(path, dirfill, 0, fh)
		v++
	}

	n := len(fs.fslist)
	if isopq {
		n = 1
	}
	for ; n > int(v); v++ {
		e, fh := fs.fslist[v].Opendir(path)
		if 0 == e {
			fs.fslist[v].Readdir(path, dirfill, 0, fh)
			fs.fslist[v].Releasedir(path, fh)
		}
	}

	names := make([]string, 0, len(dirmap))
	fs.pathmux.Lock()
	for name := range dirmap {
		if "." == name || ".." == name {
			continue
		}
		_, v = fs.pathmap.Get(pathutil.Join(path, name))
		if union.WHITEOUT == v {
			continue
		}
		names = append(names, name)
	}
	fs.pathmux.Unlock()
	sort.Strings(names)

	if ^uint64(0) != fh {
		// Readdir: pass dot dirs and 0 offset as required by FUSE
		if dot, ok := dirmap["."]; ok {
			fill(".", dot.stat, 0)
			fill("..", nil, 0)
		}
		for _, name := range names {
			ent := dirmap[name]
			if !fill(name, ent.stat, 0) {
				break
			}
		}
	} else {
		// !Readdir: use offset parameter to pass visibility information
		for _, name := range names {
			ent := dirmap[name]
			if !fill(name, ent.stat, int64(ent.v)) {
				break
			}
		}
	}
}

func (fs *Unionfs) notempty(path string, isopq bool, v uint8) (res bool) {
	fs.lsdir(path, isopq, v, ^uint64(0), func(name string, stat *fuse.Stat_t, ofst int64) bool {
		res = true
		return false
	})

	return
}

func (fs *Unionfs) mkpdir(path string) (errc int) {
	path = pathutil.Dir(path)

	pdir := path
	for "/" != pdir {
		_, _, v := fs.getvis(pdir, nil)

		if 0 == v {
			break
		}

		pdir = pathutil.Dir(pdir)
	}

	for i := len(pdir); len(path) > i; {
		for ; len(path) > i && '/' == path[i]; i++ {
		}
		for ; len(path) > i && '/' != path[i]; i++ {
		}

		var s fuse.Stat_t
		_, _, v := fs.getvis(path[:i], &s)

		switch v {
		case union.NOTEXIST, union.WHITEOUT:
			errc = -fuse.ENOENT
			break
		default:
			if fuse.S_IFDIR != s.Mode&fuse.S_IFMT {
				errc = -fuse.ENOTDIR
				break
			}
			if 0 == v {
				continue
			}
		}

		errc = fs._cpdir(path[:i], v, &s)
		if 0 != errc {
			break
		}
	}

	return
}

func (fs *Unionfs) _cpdir(path string, v uint8, stat *fuse.Stat_t) (errc int) {
	dstfs := fs.fslist[0]

	mode := stat.Mode & 0777
	errc = dstfs.Mkdir(path, mode)
	if 0 != errc {
		return
	}

	errc = dstfs.Chown(path, stat.Uid, stat.Gid)
	if -fuse.ENOSYS == errc {
		errc = 0
	} else if 0 != errc {
		return
	}

	errc = fs._cpxattr(path, v)
	if -fuse.ENOSYS == errc {
		errc = 0
	} else if 0 != errc {
		return
	}

	fs.setvisif(path, 0)
	fs.invfile(path)

	return
}

func (fs *Unionfs) _cpxattr(path string, v uint8) (errc int) {
	errc = -fuse.ENOSYS
	return
}

func (fs *Unionfs) cpdir(path string, v uint8, stat *fuse.Stat_t) (errc int) {
	if nil == stat {
		stat = &fuse.Stat_t{}
		errc = fs.fslist[v].Getattr(path, stat, ^uint64(0))
		if 0 != errc {
			return
		}
	}

	errc = fs.mkpdir(path)
	if 0 != errc {
		return
	}

	return fs._cpdir(path, v, stat)
}

func (fs *Unionfs) cplink(path string, v uint8, stat *fuse.Stat_t) (errc int) {
	srcfs := fs.fslist[v]
	dstfs := fs.fslist[0]

	if nil == stat {
		stat = &fuse.Stat_t{}
		errc = srcfs.Getattr(path, stat, ^uint64(0))
		if 0 != errc {
			return
		}
	}

	errc = fs.mkpdir(path)
	if 0 != errc {
		return
	}

	errc, target := srcfs.Readlink(path)
	if 0 != errc {
		return
	}

	errc = dstfs.Symlink(target, path)
	if 0 != errc {
		return
	}

	errc = dstfs.Chown(path, stat.Uid, stat.Gid)
	if -fuse.ENOSYS == errc {
		errc = 0
	} else if 0 != errc {
		return
	}

	errc = fs._cpxattr(path, v)
	if -fuse.ENOSYS == errc {
		errc = 0
	} else if 0 != errc {
		return
	}

	fs.setvisif(path, 0)
	fs.invfile(path)

	return
}

func (fs *Unionfs) cpfile(path string, v uint8, stat *fuse.Stat_t, srcfh uint64) (errc int) {
	srcfs := fs.fslist[v]
	dstfs := fs.fslist[0]

	if ^uint64(0) == srcfh {
		errc, srcfh = srcfs.Open(path, fuse.O_RDONLY)
		if 0 != errc {
			return
		}
		defer srcfs.Release(path, srcfh)
	}

	if nil == stat {
		stat = &fuse.Stat_t{}
		errc = srcfs.Getattr(path, stat, srcfh)
		if 0 != errc {
			return
		}
	}

	errc = fs.mkpdir(path)
	if 0 != errc {
		return
	}

	mode := stat.Mode & 0777
	errc, dstfh := dstfs.Create(path, fuse.O_CREAT|fuse.O_RDWR, mode)
	if -fuse.ENOSYS == errc {
		errc = dstfs.Mknod(path, mode, 0)
		if 0 == errc {
			errc, dstfh = dstfs.Open(path, fuse.O_RDWR)
		}
	}
	if 0 != errc {
		return
	}
	defer dstfs.Release(path, dstfh)

	errc = dstfs.Chown(path, stat.Uid, stat.Gid)
	if -fuse.ENOSYS == errc {
		errc = 0
	} else if 0 != errc {
		return
	}

	errc = fs._cpxattr(path, v)
	if -fuse.ENOSYS == errc {
		errc = 0
	} else if 0 != errc {
		return
	}

	buf := make([]byte, 64*1024)
	ofs := int64(0)
	for {
		n := srcfs.Read(path, buf, ofs, srcfh)
		if 0 > n {
			errc = n
			return
		}
		if 0 == n {
			break
		}
		m := dstfs.Write(path, buf[:n], ofs, dstfh)
		if 0 > m {
			errc = m
			return
		}
		if n != m {
			errc = -fuse.EIO
			return
		}
		ofs += int64(n)
	}

	errc = dstfs.Flush(path, dstfh)
	if -fuse.ENOSYS == errc {
		errc = 0
	} else if 0 != errc {
		return
	}

	fs.setvisif(path, 0)
	fs.invfile(path)

	return
}

func (fs *Unionfs) cpany(path string, v uint8, stat *fuse.Stat_t) (errc int) {
	if nil == stat {
		stat = &fuse.Stat_t{}
		errc = fs.fslist[v].Getattr(path, stat, ^uint64(0))
		if 0 != errc {
			return
		}
	}

	switch stat.Mode & fuse.S_IFMT {
	case fuse.S_IFDIR:
		errc = fs.cpdir(path, v, stat)
	case fuse.S_IFLNK:
		errc = fs.cplink(path, v, stat)
	default:
		errc = fs.cpfile(path, v, stat, ^uint64(0))
	}

	return
}

func (fs *Unionfs) cptree(path string, v uint8, stat *fuse.Stat_t, paths *[]string) (errc int) {
	if nil == stat {
		stat = &fuse.Stat_t{}
		errc = fs.fslist[v].Getattr(path, stat, ^uint64(0))
		if 0 != errc {
			return
		}
	}

	if 0 != v {
		errc = fs.cpany(path, v, stat)
		if -fuse.EEXIST == errc {
			errc = 0
		} else if 0 != errc {
			return
		}
	}

	if fuse.S_IFDIR == stat.Mode&fuse.S_IFMT {
		if 0 == v {
			v++
		}
		fs.lsdir(path, false, v, ^uint64(0), func(name string, stat *fuse.Stat_t, ofst int64) bool {
			errc = fs.cptree(pathutil.Join(path, name), uint8(ofst), stat, paths)
			return 0 == errc
		})
	}

	if 0 == errc && fs.hasvis(path) {
		*paths = append(*paths, path)
	}

	return
}

func (fs *Unionfs) mknode(path string, isdir bool, fn func(v uint8) int) (errc int) {
	fs.nsmux.Lock()
	defer fs.nsmux.Unlock()

	_, _, v := fs.getvis(path, nil)

	switch v {
	case union.NOTEXIST, union.WHITEOUT:
		errc = fs.mkpdir(path)
		if 0 != errc {
			return
		}

		errc = fn(0)
		if 0 == errc {
			if union.WHITEOUT == v && isdir {
				fs.setvis(path, union.OPAQUE)
			} else {
				fs.setvis(path, 0)
			}
		}
	default:
		errc = -fuse.EEXIST
	}

	return
}

func (fs *Unionfs) rmnode(path string, isdir bool, fn func(v uint8) int) (errc int) {
	fs.nsmux.Lock()
	defer fs.nsmux.Unlock()

	var s fuse.Stat_t
	_, isopq, v := fs.getvis(path, &s)

	switch v {
	case union.NOTEXIST, union.WHITEOUT:
		errc = -fuse.ENOENT
	default:
		if isdir {
			if fuse.S_IFDIR == s.Mode&fuse.S_IFMT {
				if fs.notempty(path, isopq, v) {
					return -fuse.ENOTEMPTY
				}
			} else {
				return -fuse.ENOTDIR
			}
		} else {
			if fuse.S_IFDIR == s.Mode&fuse.S_IFMT {
				return -fuse.EISDIR
			}
		}

		if 0 == v {
			errc = fn(0)
			if 0 == errc {
				fs.setvis(path, union.WHITEOUT)
			}
		} else {
			fs.setvis(path, union.WHITEOUT)
		}
	}

	return
}

func (fs *Unionfs) rename(oldpath string, newpath string, fn func(v uint8) int) (errc int) {
	fs.nsmux.Lock()
	defer fs.nsmux.Unlock()

	var news fuse.Stat_t
	_, oldisopq, oldv := fs.getvis(oldpath, nil)
	_, newisopq, newv := fs.getvis(newpath, &news)

	switch oldv {
	case union.NOTEXIST, union.WHITEOUT:
		errc = -fuse.ENOENT
	default:
		switch newv {
		case union.NOTEXIST, union.WHITEOUT:
		default:
			if fuse.S_IFDIR == news.Mode&fuse.S_IFMT && fs.notempty(newpath, newisopq, newv) {
				return -fuse.ENOTEMPTY
			}
		}

		paths := make([]string, 0, 128)

		if !oldisopq {
			errc = fs.cptree(oldpath, oldv, nil, &paths)
			if 0 != errc {
				return
			}
		}

		errc = fn(0)
		if 0 == errc {
			for _, path := range paths {
				fs.setvis(path, union.NOTEXIST)
			}
			fs.setvis(oldpath, union.WHITEOUT)
			fs.setvis(newpath, 0)
		}
	}

	return
}

func (fs *Unionfs) getnode(path string, fn func(isopq bool, v uint8) int) (errc int) {
	fs.nsmux.RLock()
	defer fs.nsmux.RUnlock()

	_, isopq, v := fs.getvis(path, nil)

	switch v {
	case union.NOTEXIST, union.WHITEOUT:
		errc = -fuse.ENOENT
	default:
		errc = fn(isopq, v)
	}

	return
}

func (fs *Unionfs) setnode(path string, fn func(v uint8) int) (errc int) {
	fs.nsmux.Lock()
	defer fs.nsmux.Unlock()

	var s fuse.Stat_t
	_, _, v := fs.getvis(path, &s)

	switch v {
	case union.NOTEXIST, union.WHITEOUT:
		errc = -fuse.ENOENT
	case 0:
		errc = fn(0)
	default:
		errc = fs.cpany(path, v, &s)
		if 0 != errc {
			return
		}
		errc = fn(0)
	}

	return
}

func (fs *Unionfs) CopyFile(path string, f0 interface{}) bool {
	f := f0.(*file)
	if 0 == f.v {
		return false
	}

	v := f.v
	fh := ^uint64(0)
	switch f.flags & (fuse.O_RDONLY | fuse.O_WRONLY | fuse.O_RDWR) {
	case fuse.O_RDONLY, fuse.O_RDWR:
		fh = f.fh
	}

	fs.filemux.Unlock()

	fs.nsmux.Lock()
	fs.cpfile(path, v, nil, fh)
	fs.nsmux.Unlock()

	fs.filemux.Lock()

	return true
}

func (fs *Unionfs) ReopenFile(oldpath string, newpath string, f0 interface{}) {
	f := f0.(*file)

	if "" != oldpath && ^uint64(0) != f.fh {
		if ^int(0) != f.flags {
			fs.fslist[f.v].Release(oldpath, f.fh)
		} else {
			fs.fslist[f.v].Releasedir(oldpath, f.fh)
		}

		f.v = 0
		f.fh = ^uint64(0)
	}

	if "" != newpath && ^uint64(0) == f.fh {
		if ^int(0) != f.flags {
			_, f.fh = fs.fslist[f.v].Open(newpath, f.flags)
		} else {
			_, f.fh = fs.fslist[f.v].Opendir(newpath)
		}
	}
}

func (fs *Unionfs) newfile(path string, isopq bool, v uint8, fh uint64, flags int) (wrapfh uint64) {
	fs.filemux.Lock()
	f := &file{isopq, v, fh, flags}
	wrapfh = fs.filemap.NewFile(path, f, 0 != v)
	fs.filemux.Unlock()
	return
}

func (fs *Unionfs) delfile(path string, wrapfh uint64) {
	fs.filemux.Lock()
	fs.filemap.DelFile(path, wrapfh)
	fs.filemux.Unlock()
}

func (fs *Unionfs) getfile(path string, wrapfh uint64) (isopq bool, v uint8, fh uint64) {
	v = union.UNKNOWN
	fh = ^uint64(0)

	fs.filemux.Lock()
	f := fs.filemap.GetFile(path, wrapfh, false).(*file)
	fs.filemux.Unlock()
	if nil != f {
		isopq, v, fh = f.isopq, f.v, f.fh
	}

	return
}

func (fs *Unionfs) getwfile(path string, wrapfh uint64) (v uint8, fh uint64) {
	v = union.UNKNOWN
	fh = ^uint64(0)

	fs.filemux.Lock()
	f := fs.filemap.GetFile(path, wrapfh, true).(*file)
	fs.filemux.Unlock()
	if nil != f {
		v, fh = f.v, f.fh
	}

	return
}

func (fs *Unionfs) invfile(path string) {
	fs.filemux.Lock()
	fs.filemap.Remove(path)
	fs.filemux.Unlock()
}

func (fs *Unionfs) Init() {
	for _, fs := range fs.fslist {
		fs.Init()
	}
}

func (fs *Unionfs) Destroy() {
	fs.pathmap.Write()
	for _, fs := range fs.fslist {
		fs.Destroy()
	}
	fs.pathmap.Close()
}

func (fs *Unionfs) Statfs(path string, stat *fuse.Statfs_t) (errc int) {
	errc = -fuse.ENOSYS

	for i, fs := range fs.fslist {
		if 0 == i {
			errc = fs.Statfs(path, stat)
			if 0 != errc {
				break
			}
		} else {
			s := fuse.Statfs_t{}
			e := fs.Statfs(path, &s)
			if 0 == e {
				if 0 != stat.Frsize {
					stat.Blocks += s.Frsize * s.Blocks / stat.Frsize
				}
				stat.Files += s.Files
			}
		}
	}

	return
}

func (fs *Unionfs) Mknod(path string, mode uint32, dev uint64) (errc int) {
	return fs.mknode(path, false, func(v uint8) int {
		return fs.fslist[v].Mknod(path, mode, dev)
	})
}

func (fs *Unionfs) Mkdir(path string, mode uint32) (errc int) {
	return fs.mknode(path, true, func(v uint8) int {
		return fs.fslist[v].Mkdir(path, mode)
	})
}

func (fs *Unionfs) Unlink(path string) (errc int) {
	return fs.rmnode(path, false, func(v uint8) int {
		return fs.fslist[v].Unlink(path)
	})
}

func (fs *Unionfs) Rmdir(path string) (errc int) {
	return fs.rmnode(path, true, func(v uint8) int {
		return fs.fslist[v].Rmdir(path)
	})
}

func (fs *Unionfs) Link(oldpath string, newpath string) (errc int) {
	return fs.mknode(newpath, false, func(v uint8) int {
		var s fuse.Stat_t
		_, _, oldv := fs.getvis(oldpath, &s)
		switch oldv {
		case union.NOTEXIST, union.WHITEOUT:
		case 0:
		default:
			e := fs.cpany(oldpath, oldv, &s)
			if 0 != e {
				return e
			}
		}
		return fs.fslist[v].Link(oldpath, newpath)
	})
}

func (fs *Unionfs) Symlink(target string, newpath string) (errc int) {
	return fs.mknode(newpath, false, func(v uint8) int {
		return fs.fslist[v].Symlink(target, newpath)
	})
}

func (fs *Unionfs) Readlink(path string) (errc int, target string) {
	errc = fs.getnode(path, func(isopq bool, v uint8) int {
		errc, target = fs.fslist[v].Readlink(path)
		return errc
	})
	return
}

func (fs *Unionfs) Rename(oldpath string, newpath string) (errc int) {
	return fs.rename(oldpath, newpath, func(v uint8) int {
		return fs.fslist[v].Rename(oldpath, newpath)
	})
}

func (fs *Unionfs) Chmod(path string, mode uint32) (errc int) {
	return fs.setnode(path, func(v uint8) int {
		return fs.fslist[v].Chmod(path, mode)
	})
}

func (fs *Unionfs) Chown(path string, uid uint32, gid uint32) (errc int) {
	return fs.setnode(path, func(v uint8) int {
		return fs.fslist[v].Chown(path, uid, gid)
	})
}

func (fs *Unionfs) Utimens(path string, tmsp []fuse.Timespec) (errc int) {
	return fs.setnode(path, func(v uint8) int {
		return fs.fslist[v].Utimens(path, tmsp)
	})
}

func (fs *Unionfs) Access(path string, mask uint32) (errc int) {
	return fs.getnode(path, func(isopq bool, v uint8) int {
		return fs.fslist[v].Access(path, mask)
	})
}

func (fs *Unionfs) Create(path string, flags int, mode uint32) (errc int, fh uint64) {
	errc = fs.mknode(path, false, func(v uint8) int {
		errc, fh = fs.fslist[v].Create(path, flags, mode)
		if 0 == errc {
			fh = fs.newfile(path, false, 0, fh, flags&(fuse.O_RDONLY|fuse.O_WRONLY|fuse.O_RDWR))
		}
		return errc
	})
	return
}

func (fs *Unionfs) Open(path string, flags int) (errc int, fh uint64) {
	errc = fs.getnode(path, func(isopq bool, v uint8) int {
		errc, fh = fs.fslist[v].Open(path, flags)
		if 0 == errc {
			fh = fs.newfile(path, false, v, fh, flags&(fuse.O_RDONLY|fuse.O_WRONLY|fuse.O_RDWR))
		}
		return errc
	})
	return
}

func (fs *Unionfs) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	if ^uint64(0) == fh {
		fs.nsmux.RLock()
		defer fs.nsmux.RUnlock()

		errc, _, _ = fs.getvis(path, stat)
		return errc
	} else {
		_, v, fh := fs.getfile(path, fh)
		if union.UNKNOWN == v {
			return -fuse.EIO
		}

		return fs.fslist[v].Getattr(path, stat, fh)
	}
}

func (fs *Unionfs) Truncate(path string, size int64, fh uint64) (errc int) {
	if ^uint64(0) == fh {
		return fs.setnode(path, func(v uint8) int {
			return fs.fslist[v].Truncate(path, size, fh)
		})
	} else {
		v, fh := fs.getwfile(path, fh)
		if union.UNKNOWN == v {
			return -fuse.EIO
		}

		return fs.fslist[v].Truncate(path, size, fh)
	}
}

func (fs *Unionfs) Read(path string, buff []byte, ofst int64, fh uint64) (errc int) {
	_, v, fh := fs.getfile(path, fh)
	if union.UNKNOWN == v {
		return -fuse.EIO
	}

	return fs.fslist[v].Read(path, buff, ofst, fh)
}

func (fs *Unionfs) Write(path string, buff []byte, ofst int64, fh uint64) (errc int) {
	v, fh := fs.getwfile(path, fh)
	if union.UNKNOWN == v {
		return -fuse.EIO
	}

	return fs.fslist[v].Write(path, buff, ofst, fh)
}

func (fs *Unionfs) Flush(path string, fh uint64) (errc int) {
	_, v, fh := fs.getfile(path, fh)
	if 0 != v {
		return 0 // return success if not writable
	}

	return fs.fslist[v].Flush(path, fh)
}

func (fs *Unionfs) Release(path string, fh uint64) (errc int) {
	wrapfh := fh

	_, v, fh := fs.getfile("", fh)
	if union.UNKNOWN == v {
		return -fuse.EIO
	}

	if ^uint64(0) != fh {
		errc = fs.fslist[v].Release(path, fh)
	}

	fs.delfile(path, wrapfh)

	return
}

func (fs *Unionfs) Fsync(path string, datasync bool, fh uint64) (errc int) {
	_, v, fh := fs.getfile(path, fh)
	if 0 != v {
		return 0 // return success if not writable
	}

	return fs.fslist[v].Fsync(path, datasync, fh)
}

func (fs *Unionfs) Opendir(path string) (errc int, fh uint64) {
	errc = fs.getnode(path, func(isopq bool, v uint8) int {
		errc, fh = fs.fslist[v].Opendir(path)
		if 0 == errc {
			fh = fs.newfile(path, isopq, v, fh, ^int(0))
		}
		return errc
	})
	return
}

func (fs *Unionfs) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) (errc int) {

	isopq, v, fh := fs.getfile(path, fh)
	if union.UNKNOWN == v {
		return -fuse.EIO
	}

	fs.lsdir(path, isopq, v, fh, fill)
	return 0
}

func (fs *Unionfs) Releasedir(path string, fh uint64) (errc int) {
	wrapfh := fh

	_, v, fh := fs.getfile("", fh)
	if union.UNKNOWN == v {
		return -fuse.EIO
	}

	if ^uint64(0) != fh {
		errc = fs.fslist[v].Releasedir(path, fh)
	}

	fs.delfile(path, wrapfh)

	return
}

func (fs *Unionfs) Fsyncdir(path string, datasync bool, fh uint64) (errc int) {
	_, v, fh := fs.getfile(path, fh)
	if 0 != v {
		return 0 // return success if not writable
	}

	return fs.fslist[v].Fsyncdir(path, datasync, fh)
}

func (fs *Unionfs) Setxattr(path string, name string, value []byte, flags int) (errc int) {
	return fs.setnode(path, func(v uint8) int {
		return fs.fslist[v].Setxattr(path, name, value, flags)
	})
}

func (fs *Unionfs) Getxattr(path string, name string) (errc int, value []byte) {
	errc = fs.getnode(path, func(isopq bool, v uint8) int {
		errc, value = fs.fslist[v].Getxattr(path, name)
		return errc
	})
	return
}

func (fs *Unionfs) Removexattr(path string, name string) (errc int) {
	return fs.setnode(path, func(v uint8) int {
		return fs.fslist[v].Removexattr(path, name)
	})
}

func (fs *Unionfs) Listxattr(path string, fill func(name string) bool) (errc int) {
	return fs.getnode(path, func(isopq bool, v uint8) int {
		return fs.fslist[v].Listxattr(path, fill)
	})
}

func (fs *Unionfs) Chflags(path string, flags uint32) (errc int) {
	intf, ok := fs.fslist[0].(fuse.FileSystemChflags)
	if !ok {
		return -fuse.ENOSYS
	}

	return fs.setnode(path, func(v uint8) int {
		return intf.Chflags(path, flags)
	})
}

func (fs *Unionfs) Setcrtime(path string, tmsp fuse.Timespec) (errc int) {
	intf, ok := fs.fslist[0].(fuse.FileSystemSetcrtime)
	if !ok {
		return -fuse.ENOSYS
	}

	return fs.setnode(path, func(v uint8) int {
		return intf.Setcrtime(path, tmsp)
	})
}

func (fs *Unionfs) Setchgtime(path string, tmsp fuse.Timespec) (errc int) {
	intf, ok := fs.fslist[0].(fuse.FileSystemSetchgtime)
	if !ok {
		return -fuse.ENOSYS
	}

	return fs.setnode(path, func(v uint8) int {
		return intf.Setchgtime(path, tmsp)
	})
}

var _ fuse.FileSystemInterface = (*Unionfs)(nil)
var _ fuse.FileSystemChflags = (*Unionfs)(nil)
var _ fuse.FileSystemSetcrtime = (*Unionfs)(nil)
var _ fuse.FileSystemSetchgtime = (*Unionfs)(nil)
