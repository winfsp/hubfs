/*
 * unionfs.go
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

package unionfs

import (
	pathutil "path"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/billziss-gh/cgofuse/fuse"
)

type filesystem struct {
	fslist    []fuse.FileSystemInterface // file system list
	pmpath    string                     // path map file path
	pmsync    bool                       // perform path map file sync
	lazytick  time.Duration              // lazy writevis tick
	nsmux     sync.RWMutex               // namespace mutex
	pathmap   *Pathmap                   // path map
	filemux   sync.Mutex                 // open file mutex
	filemap   *Filemap                   // open file map
	lazystopC chan struct{}              // lazy writevis stop channel
	lazystopW *sync.WaitGroup            // lazy writevis stop waitgroup

	// lock hierarchy:
	//     nsmux -> pathmap
	//     nsmux -> filemux
}

type file struct {
	isopq bool
	v     uint8
	fh    uint64
	flags int
}

type Config struct {
	Fslist   []fuse.FileSystemInterface
	Pmname   string
	Pmsync   bool
	Lazytick time.Duration
	Caseins  bool
}

func New(c Config) fuse.FileSystemInterface {
	if 0 == len(c.Fslist) {
		c.Fslist = []fuse.FileSystemInterface{&fuse.FileSystemBase{}}
	}
	if "" == c.Pmname {
		c.Pmname = ".unionfs"
	}

	fs := &filesystem{}
	fs.fslist = append(fs.fslist, c.Fslist...)
	fs.pmpath = pathutil.Join("/", c.Pmname)
	fs.pmsync = c.Pmsync
	fs.lazytick = c.Lazytick
	fs.pathmap = nil // OpenPathmap uses fslist[0]; delay initialization until Init time
	fs.filemap = NewFilemap(fs, c.Caseins)

	return fs
}

func (fs *filesystem) getvis(path string, stat *fuse.Stat_t) (errc int, isopq bool, v uint8) {
	fs.pathmap.Lock()
	isopq, v = fs.pathmap.Get(path)
	fs.pathmap.Unlock()

	if "linux" == runtime.GOOS || "darwin" == runtime.GOOS {
		/* Linux/macOS can send us invalid/long paths. Perform check here. */
		for i, c := 0, 0; len(path) > i; i++ {
			if '/' == path[i] {
				c = 0
			} else {
				c++
				if 255 < c {
					return -fuse.ENAMETOOLONG, isopq, NOTEXIST
				}
			}
		}
	}

	if UNKNOWN == v {
		u := NOTEXIST
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

		fs.pathmap.Lock()
		isopq, v = fs.pathmap.Get(path)
		if UNKNOWN == v {
			fs.pathmap.Set(path, u)
			fs.pathmap.Unlock()
			if NOTEXIST == u {
				return -fuse.ENOENT, isopq, NOTEXIST
			}
			if nil != stat {
				*stat = s
			}
			return 0, isopq, u
		}
		fs.pathmap.Unlock()
	}

	switch v {
	case NOTEXIST, WHITEOUT:
		errc = -fuse.ENOENT
	default:
		if nil != stat {
			errc = fs.fslist[v].Getattr(path, stat, ^uint64(0))
			if 0 != errc {
				v = NOTEXIST
			}
		} else {
			errc = 0
		}
	}

	return
}

func (fs *filesystem) hasvis(path string) (res bool) {
	fs.pathmap.Lock()
	_, res = fs.pathmap.TryGet(path)
	fs.pathmap.Unlock()
	return
}

func (fs *filesystem) setvis(path string, v uint8) {
	fs.pathmap.Lock()
	fs.pathmap.Set(path, v)
	fs.pathmap.Unlock()
}

func (fs *filesystem) setvisif(path string, v uint8) {
	fs.pathmap.Lock()
	fs.pathmap.SetIf(path, v)
	fs.pathmap.Unlock()
}

func (fs *filesystem) writevis() (errc int) {
	return fs.pathmap.Write(fs.pmsync)
}

func (fs *filesystem) condwritevis(cond *bool) (errc int) {
	if *cond && 0 == fs.lazytick {
		errc = fs.writevis()
	}
	return
}

func (fs *filesystem) _lazyWritevis() {
	defer fs.lazystopW.Done()
	ticker := time.NewTicker(fs.lazytick)
	for {
		select {
		case <-ticker.C:
			fs.writevis()
		case <-fs.lazystopC:
			ticker.Stop()
			return
		}
	}
}

func (fs *filesystem) lsdir(path string,
	isopq bool, v uint8, fh uint64,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool) {

	pmname := ""
	if "/" == path {
		pmname = pathutil.Base(fs.pmpath)
	}

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
	fs.pathmap.Lock()
	for name := range dirmap {
		if "." == name || ".." == name || pmname == name {
			continue
		}
		_, v = fs.pathmap.Get(pathutil.Join(path, name))
		if WHITEOUT == v {
			continue
		}
		names = append(names, name)
	}
	fs.pathmap.Unlock()
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

func (fs *filesystem) notempty(path string, isopq bool, v uint8) (res bool) {
	fs.lsdir(path, isopq, v, ^uint64(0), func(name string, stat *fuse.Stat_t, ofst int64) bool {
		res = true
		return false
	})

	return
}

func (fs *filesystem) readpath(path string, v uint8) string {
	if !fs.filemap.Caseins {
		return path
	}

	rp, ok := fs.fslist[v].(FileSystemReadpath)
	if !ok {
		return path
	}

	errc, p := rp.Readpath(path)
	if 0 != errc || strings.ToUpper(p) != strings.ToUpper(path) {
		return path
	}

	return p
}

func (fs *filesystem) mkpdir(path string) (errc int) {
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
		case NOTEXIST, WHITEOUT:
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

func (fs *filesystem) _cpdir(path string, v uint8, stat *fuse.Stat_t) (errc int) {
	path = fs.readpath(path, v)

	dstfs := fs.fslist[0]

	mode := stat.Mode & 0777
	errc = dstfs.Mkdir(path, mode)
	if 0 != errc {
		return
	}

	/* Chown is best effort because we may not have privileges to perform this operation */
	errc = dstfs.Chown(path, stat.Uid, stat.Gid)

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

func (fs *filesystem) _cpxattr(path string, v uint8) (errc int) {
	errc = -fuse.ENOSYS
	return
}

func (fs *filesystem) cpdir(path string, v uint8, stat *fuse.Stat_t) (errc int) {
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

func (fs *filesystem) cplink(path string, v uint8, stat *fuse.Stat_t) (errc int) {
	path = fs.readpath(path, v)

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

	/* Chown is best effort because we may not have privileges to perform this operation */
	errc = dstfs.Chown(path, stat.Uid, stat.Gid)

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

func (fs *filesystem) cpfile(path string, v uint8, stat *fuse.Stat_t, srcfh uint64) (errc int) {
	path = fs.readpath(path, v)

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

	/* Chown is best effort because we may not have privileges to perform this operation */
	errc = dstfs.Chown(path, stat.Uid, stat.Gid)

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

func (fs *filesystem) cpany(path string, v uint8, stat *fuse.Stat_t) (errc int) {
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

func (fs *filesystem) cptree(path string, v uint8, stat *fuse.Stat_t, paths *[]string) (errc int) {
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

func (fs *filesystem) mknode(path string, isdir bool, fn func(v uint8) int) (errc int) {
	if hasPathPrefix(path, fs.pmpath, fs.filemap.Caseins) {
		return -fuse.EPERM
	}

	var cond bool
	defer fs.condwritevis(&cond)

	fs.nsmux.Lock()
	defer fs.nsmux.Unlock()

	_, _, v := fs.getvis(path, nil)

	switch v {
	case NOTEXIST, WHITEOUT:
		cond = true

		errc = fs.mkpdir(path)
		if 0 != errc {
			return
		}

		errc = fn(0)
		if 0 == errc {
			if WHITEOUT == v && isdir {
				fs.setvis(path, OPAQUE)
			} else {
				fs.setvis(path, 0)
			}
		}
	default:
		errc = -fuse.EEXIST
	}

	return
}

func (fs *filesystem) rmnode(path string, isdir bool, fn func(v uint8) int) (errc int) {
	if hasPathPrefix(path, fs.pmpath, fs.filemap.Caseins) {
		return -fuse.EPERM
	}

	var cond bool
	defer fs.condwritevis(&cond)

	fs.nsmux.Lock()
	defer fs.nsmux.Unlock()

	var s fuse.Stat_t
	_, isopq, v := fs.getvis(path, &s)

	switch v {
	case NOTEXIST, WHITEOUT:
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

		cond = true

		if 0 == v {
			errc = fn(0)
			if 0 == errc {
				fs.setvis(path, WHITEOUT)
			}
		} else {
			fs.setvis(path, WHITEOUT)
		}
	}

	return
}

func (fs *filesystem) renode(oldpath string, newpath string, link bool, fn func(v uint8) int) (errc int) {
	if hasPathPrefix(oldpath, fs.pmpath, fs.filemap.Caseins) ||
		hasPathPrefix(newpath, fs.pmpath, fs.filemap.Caseins) {
		return -fuse.EPERM
	}

	var cond bool
	defer fs.condwritevis(&cond)

	fs.nsmux.Lock()
	defer fs.nsmux.Unlock()

	var olds, news fuse.Stat_t
	_, _, oldv := fs.getvis(oldpath, &olds)
	_, newisopq, newv := fs.getvis(newpath, &news)

	switch oldv {
	case NOTEXIST, WHITEOUT:
		errc = -fuse.ENOENT
	default:
		switch newv {
		case NOTEXIST, WHITEOUT:
			if link && fuse.S_IFDIR == olds.Mode&fuse.S_IFMT {
				return -fuse.EPERM
			}
		default:
			if link {
				return -fuse.EEXIST
			}
			if fuse.S_IFDIR == olds.Mode&fuse.S_IFMT {
				if fuse.S_IFDIR == news.Mode&fuse.S_IFMT {
					if fs.notempty(newpath, newisopq, newv) {
						return -fuse.ENOTEMPTY
					}
				} else {
					return -fuse.ENOTDIR
				}
			} else {
				if fuse.S_IFDIR == news.Mode&fuse.S_IFMT {
					return -fuse.EISDIR
				}
			}
		}

		cond = true

		paths := make([]string, 0, 128)
		errc = fs.cptree(oldpath, oldv, &olds, &paths)
		if 0 != errc {
			return
		}

		errc = fn(0)
		if 0 == errc {
			fs.pathmap.Lock()
			if !link {
				for _, path := range paths {
					if oldpath == path {
						continue
					}
					v, ok := fs.pathmap.TryGet(path)
					if !ok {
						continue
					}
					fs.pathmap.Set(path, NOTEXIST)
					fs.pathmap.Set(newpath+path[len(oldpath):], v)
				}
				fs.pathmap.Set(oldpath, WHITEOUT)
			}
			fs.pathmap.Set(newpath, 0)
			fs.pathmap.Unlock()
		}
	}

	return
}

func (fs *filesystem) getnode(path string, fn func(isopq bool, v uint8) int) (errc int) {
	if hasPathPrefix(path, fs.pmpath, fs.filemap.Caseins) {
		return -fuse.EPERM
	}

	fs.nsmux.RLock()
	defer fs.nsmux.RUnlock()

	_, isopq, v := fs.getvis(path, nil)

	switch v {
	case NOTEXIST, WHITEOUT:
		errc = -fuse.ENOENT
	default:
		errc = fn(isopq, v)
	}

	return
}

func (fs *filesystem) setnode(path string, fn func(v uint8) int) (errc int) {
	if hasPathPrefix(path, fs.pmpath, fs.filemap.Caseins) {
		return -fuse.EPERM
	}

	var cond bool
	defer fs.condwritevis(&cond)

	fs.nsmux.Lock()
	defer fs.nsmux.Unlock()

	var s fuse.Stat_t
	_, _, v := fs.getvis(path, &s)

	switch v {
	case NOTEXIST, WHITEOUT:
		errc = -fuse.ENOENT
	case 0:
		errc = fn(0)
	default:
		cond = true

		errc = fs.cpany(path, v, &s)
		if 0 != errc {
			return
		}
		errc = fn(0)
	}

	return
}

func (fs *filesystem) CopyFile(path string, f0 interface{}) bool {
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

	var cond = true
	fs.condwritevis(&cond)

	fs.filemux.Lock()

	return true
}

func (fs *filesystem) ReopenFile(oldpath string, newpath string, f0 interface{}) {
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

func (fs *filesystem) newfile(path string, isopq bool, v uint8, fh uint64, flags int) (wrapfh uint64) {
	fs.filemux.Lock()
	f := &file{isopq, v, fh, flags}
	wrapfh = fs.filemap.NewFile(path, f, 0 != v)
	fs.filemux.Unlock()
	return
}

func (fs *filesystem) delfile(path string, wrapfh uint64) {
	fs.filemux.Lock()
	fs.filemap.DelFile(path, wrapfh)
	fs.filemux.Unlock()
}

func (fs *filesystem) getfile(path string, wrapfh uint64) (isopq bool, v uint8, fh uint64) {
	v = UNKNOWN
	fh = ^uint64(0)

	fs.filemux.Lock()
	f := fs.filemap.GetFile(path, wrapfh, false).(*file)
	fs.filemux.Unlock()
	if nil != f {
		isopq, v, fh = f.isopq, f.v, f.fh
	}

	return
}

func (fs *filesystem) getwfile(path string, wrapfh uint64) (v uint8, fh uint64) {
	v = UNKNOWN
	fh = ^uint64(0)

	fs.filemux.Lock()
	f := fs.filemap.GetFile(path, wrapfh, true).(*file)
	fs.filemux.Unlock()
	if nil != f {
		v, fh = f.v, f.fh
	}

	return
}

func (fs *filesystem) invfile(path string) {
	fs.filemux.Lock()
	fs.filemap.Remove(path)
	fs.filemux.Unlock()
}

func (fs *filesystem) Init() {
	for _, fs := range fs.fslist {
		fs.Init()
	}

	_, fs.pathmap = OpenPathmap(fs.fslist[0], fs.pmpath, fs.filemap.Caseins)
	if nil == fs.pathmap {
		_, fs.pathmap = OpenPathmap(nil, "", fs.filemap.Caseins)
	}

	if 0 != fs.lazytick {
		fs.lazystopC = make(chan struct{}, 1)
		fs.lazystopW = &sync.WaitGroup{}
		fs.lazystopW.Add(1)
		go fs._lazyWritevis()
	}
}

func (fs *filesystem) Destroy() {
	if 0 != fs.lazytick {
		fs.lazystopC <- struct{}{}
		fs.lazystopW.Wait()
		close(fs.lazystopC)
		fs.lazystopC = nil
		fs.lazystopW = nil
	}

	fs.writevis()
	fs.pathmap.Close()

	for _, fs := range fs.fslist {
		fs.Destroy()
	}
}

func (fs *filesystem) Statfs(path string, stat *fuse.Statfs_t) (errc int) {
	errc = -fuse.ENOSYS

	for _, fs := range fs.fslist {
		// only report stats for first (i.e. writable) file system
		errc = fs.Statfs(path, stat)
		break
	}

	return
}

func (fs *filesystem) Mknod(path string, mode uint32, dev uint64) (errc int) {
	return fs.mknode(path, false, func(v uint8) int {
		return fs.fslist[v].Mknod(path, mode, dev)
	})
}

func (fs *filesystem) Mkdir(path string, mode uint32) (errc int) {
	return fs.mknode(path, true, func(v uint8) int {
		return fs.fslist[v].Mkdir(path, mode)
	})
}

func (fs *filesystem) Unlink(path string) (errc int) {
	return fs.rmnode(path, false, func(v uint8) int {
		return fs.fslist[v].Unlink(path)
	})
}

func (fs *filesystem) Rmdir(path string) (errc int) {
	return fs.rmnode(path, true, func(v uint8) int {
		return fs.fslist[v].Rmdir(path)
	})
}

func (fs *filesystem) Link(oldpath string, newpath string) (errc int) {
	return fs.renode(oldpath, newpath, true, func(v uint8) int {
		return fs.fslist[v].Link(oldpath, newpath)
	})
}

func (fs *filesystem) Symlink(target string, newpath string) (errc int) {
	return fs.mknode(newpath, false, func(v uint8) int {
		return fs.fslist[v].Symlink(target, newpath)
	})
}

func (fs *filesystem) Readlink(path string) (errc int, target string) {
	errc = fs.getnode(path, func(isopq bool, v uint8) int {
		errc, target = fs.fslist[v].Readlink(path)
		return errc
	})
	return
}

func (fs *filesystem) Rename(oldpath string, newpath string) (errc int) {
	return fs.renode(oldpath, newpath, false, func(v uint8) int {
		return fs.fslist[v].Rename(oldpath, newpath)
	})
}

func (fs *filesystem) Chmod(path string, mode uint32) (errc int) {
	return fs.setnode(path, func(v uint8) int {
		return fs.fslist[v].Chmod(path, mode)
	})
}

func (fs *filesystem) Chown(path string, uid uint32, gid uint32) (errc int) {
	return fs.setnode(path, func(v uint8) int {
		return fs.fslist[v].Chown(path, uid, gid)
	})
}

func (fs *filesystem) Utimens(path string, tmsp []fuse.Timespec) (errc int) {
	return fs.setnode(path, func(v uint8) int {
		return fs.fslist[v].Utimens(path, tmsp)
	})
}

func (fs *filesystem) Access(path string, mask uint32) (errc int) {
	return fs.getnode(path, func(isopq bool, v uint8) int {
		return fs.fslist[v].Access(path, mask)
	})
}

func (fs *filesystem) Create(path string, flags int, mode uint32) (errc int, fh uint64) {
	errc = fs.mknode(path, false, func(v uint8) int {
		errc, fh = fs.fslist[v].Create(path, flags, mode)
		if 0 == errc {
			fh = fs.newfile(path, false, 0, fh, flags&(fuse.O_RDONLY|fuse.O_WRONLY|fuse.O_RDWR))
		}
		return errc
	})
	return
}

func (fs *filesystem) Open(path string, flags int) (errc int, fh uint64) {
	errc = fs.getnode(path, func(isopq bool, v uint8) int {
		errc, fh = fs.fslist[v].Open(path, flags)
		if 0 == errc {
			fh = fs.newfile(path, false, v, fh, flags&(fuse.O_RDONLY|fuse.O_WRONLY|fuse.O_RDWR))
		}
		return errc
	})
	return
}

func (fs *filesystem) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	if ^uint64(0) == fh {
		if hasPathPrefix(path, fs.pmpath, fs.filemap.Caseins) {
			return -fuse.EPERM
		}

		fs.nsmux.RLock()
		defer fs.nsmux.RUnlock()

		errc, _, _ = fs.getvis(path, stat)
		return errc
	} else {
		_, v, fh := fs.getfile(path, fh)
		if UNKNOWN == v {
			return -fuse.EIO
		}

		return fs.fslist[v].Getattr(path, stat, fh)
	}
}

func (fs *filesystem) Truncate(path string, size int64, fh uint64) (errc int) {
	if ^uint64(0) == fh {
		return fs.setnode(path, func(v uint8) int {
			return fs.fslist[v].Truncate(path, size, fh)
		})
	} else {
		v, fh := fs.getwfile(path, fh)
		if UNKNOWN == v {
			return -fuse.EIO
		}

		return fs.fslist[v].Truncate(path, size, fh)
	}
}

func (fs *filesystem) Read(path string, buff []byte, ofst int64, fh uint64) (errc int) {
	_, v, fh := fs.getfile(path, fh)
	if UNKNOWN == v {
		return -fuse.EIO
	}

	return fs.fslist[v].Read(path, buff, ofst, fh)
}

func (fs *filesystem) Write(path string, buff []byte, ofst int64, fh uint64) (errc int) {
	v, fh := fs.getwfile(path, fh)
	if UNKNOWN == v {
		return -fuse.EIO
	}

	return fs.fslist[v].Write(path, buff, ofst, fh)
}

func (fs *filesystem) Flush(path string, fh uint64) (errc int) {
	_, v, fh := fs.getfile(path, fh)
	if 0 != v {
		return 0 // return success if not writable
	}

	return fs.fslist[v].Flush(path, fh)
}

func (fs *filesystem) Release(path string, fh uint64) (errc int) {
	wrapfh := fh

	_, v, fh := fs.getfile("", fh)
	if UNKNOWN == v {
		return -fuse.EIO
	}

	if ^uint64(0) != fh {
		errc = fs.fslist[v].Release(path, fh)
	}

	fs.delfile(path, wrapfh)

	return
}

func (fs *filesystem) Fsync(path string, datasync bool, fh uint64) (errc int) {
	_, v, fh := fs.getfile(path, fh)
	if 0 != v {
		return 0 // return success if not writable
	}

	return fs.fslist[v].Fsync(path, datasync, fh)
}

func (fs *filesystem) Opendir(path string) (errc int, fh uint64) {
	errc = fs.getnode(path, func(isopq bool, v uint8) int {
		errc, fh = fs.fslist[v].Opendir(path)
		if 0 == errc {
			fh = fs.newfile(path, isopq, v, fh, ^int(0))
		}
		return errc
	})
	return
}

func (fs *filesystem) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) (errc int) {

	isopq, v, fh := fs.getfile(path, fh)
	if UNKNOWN == v {
		return -fuse.EIO
	}

	fs.lsdir(path, isopq, v, fh, fill)
	return 0
}

func (fs *filesystem) Releasedir(path string, fh uint64) (errc int) {
	wrapfh := fh

	_, v, fh := fs.getfile("", fh)
	if UNKNOWN == v {
		return -fuse.EIO
	}

	if ^uint64(0) != fh {
		errc = fs.fslist[v].Releasedir(path, fh)
	}

	fs.delfile(path, wrapfh)

	return
}

func (fs *filesystem) Fsyncdir(path string, datasync bool, fh uint64) (errc int) {
	_, v, fh := fs.getfile(path, fh)
	if 0 != v {
		return 0 // return success if not writable
	}

	errc = fs.fslist[v].Fsyncdir(path, datasync, fh)
	if 0 == errc {
		fs.writevis()
	}

	return
}

func (fs *filesystem) Setxattr(path string, name string, value []byte, flags int) (errc int) {
	return fs.setnode(path, func(v uint8) int {
		return fs.fslist[v].Setxattr(path, name, value, flags)
	})
}

func (fs *filesystem) Getxattr(path string, name string) (errc int, value []byte) {
	errc = fs.getnode(path, func(isopq bool, v uint8) int {
		errc, value = fs.fslist[v].Getxattr(path, name)
		return errc
	})
	return
}

func (fs *filesystem) Removexattr(path string, name string) (errc int) {
	return fs.setnode(path, func(v uint8) int {
		return fs.fslist[v].Removexattr(path, name)
	})
}

func (fs *filesystem) Listxattr(path string, fill func(name string) bool) (errc int) {
	return fs.getnode(path, func(isopq bool, v uint8) int {
		return fs.fslist[v].Listxattr(path, fill)
	})
}

func (fs *filesystem) Chflags(path string, flags uint32) (errc int) {
	intf, ok := fs.fslist[0].(fuse.FileSystemChflags)
	if !ok {
		return -fuse.ENOSYS
	}

	return fs.setnode(path, func(v uint8) int {
		return intf.Chflags(path, flags)
	})
}

func (fs *filesystem) Setcrtime(path string, tmsp fuse.Timespec) (errc int) {
	intf, ok := fs.fslist[0].(fuse.FileSystemSetcrtime)
	if !ok {
		return -fuse.ENOSYS
	}

	return fs.setnode(path, func(v uint8) int {
		return intf.Setcrtime(path, tmsp)
	})
}

func (fs *filesystem) Setchgtime(path string, tmsp fuse.Timespec) (errc int) {
	intf, ok := fs.fslist[0].(fuse.FileSystemSetchgtime)
	if !ok {
		return -fuse.ENOSYS
	}

	return fs.setnode(path, func(v uint8) int {
		return intf.Setchgtime(path, tmsp)
	})
}

func hasPathPrefix(path, prefix string, caseins bool) bool {
	if caseins {
		path = strings.ToUpper(path)
		prefix = strings.ToUpper(prefix)
	}
	return path == prefix ||
		(len(path) > len(prefix) && path[:len(prefix)] == prefix && path[len(prefix)] == '/')
}

var _ fuse.FileSystemInterface = (*filesystem)(nil)
var _ fuse.FileSystemChflags = (*filesystem)(nil)
var _ fuse.FileSystemSetcrtime = (*filesystem)(nil)
var _ fuse.FileSystemSetchgtime = (*filesystem)(nil)
