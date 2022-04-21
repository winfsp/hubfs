/*
 * unionfs_test.go
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
	"errors"
	"fmt"
	"math/rand"
	pathutil "path"
	"sort"
	"testing"
	"time"

	"github.com/billziss-gh/golib/trace"
	"github.com/winfsp/cgofuse/fuse"
)

type testrun struct {
	prefix string
	r      *rand.Rand
	paths  []string
	fs     fuse.FileSystemInterface
}

var (
	testrunCount = 0
	maxcnt       = 100
	pctdir       = 20
	pctsym       = 10
	pctact       = 10
)

func newtestrun(seed int64) *testrun {
	if 0 == seed {
		seed = time.Now().UnixNano()
	}
	testrunCount++
	return &testrun{
		prefix: fmt.Sprintf("TEST%d", testrunCount),
		r:      rand.New(rand.NewSource(seed)),
	}
}

func (t *testrun) randname() string {
	namelen := 8 + t.r.Int()%8
	namebuf := make([]uint8, namelen)
	for j := 0; namelen > j; j++ {
		namebuf[j] = uint8('a' + t.r.Int()%26)
	}
	return string(namebuf)
}

func (t *testrun) inspath(path string) {
	i := sort.SearchStrings(t.paths, path)
	t.paths = append(t.paths, "")
	copy(t.paths[i+1:], t.paths[i:])
	t.paths[i] = path
}

func (t *testrun) delpath(path string) {
	i := sort.SearchStrings(t.paths, path)
	t.paths = append(t.paths[:i], t.paths[i+1:]...)
}

func (t *testrun) mkdir(path string, mode uint32) (errc int) {
	defer trace.Trace(0, t.prefix, path)(&errc)

	errc = t.fs.Mkdir(path, mode)
	if 0 != errc {
		return
	}

	t.inspath(path)

	return
}

func (t *testrun) mkfile(path string, mode uint32, buf []uint8) (errc int) {
	defer trace.Trace(0, t.prefix, path)(&errc)

	errc, fh := t.fs.Create(path, fuse.O_CREAT|fuse.O_RDWR, mode)
	if -fuse.ENOSYS == errc {
		errc = t.fs.Mknod(path, mode, 0)
		if 0 == errc {
			errc, fh = t.fs.Open(path, fuse.O_RDWR)
		}
	}
	if 0 != errc {
		return
	}
	defer t.fs.Release(path, fh)

	n := t.fs.Write(path, buf, 0, fh)
	if 0 > n {
		errc = n
		return
	}

	errc = t.fs.Flush(path, fh)
	if -fuse.ENOSYS == errc {
		errc = 0
	} else if 0 != errc {
		return
	}

	t.inspath(path)

	return
}

func (t *testrun) symlink(path string) (errc int) {
	defer trace.Trace(0, t.prefix, path)(&errc)

	errc = t.fs.Symlink(path, path)
	if 0 != errc {
		return
	}

	t.inspath(path)

	return
}

func (t *testrun) remove(path string) (errc int) {
	defer trace.Trace(0, t.prefix, path)(&errc)

	errc = t.fs.Unlink(path)
	if -fuse.EISDIR == errc {
		errc = t.fs.Rmdir(path)
	}
	if 0 != errc {
		return
	}

	t.delpath(path)

	return
}

func (t *testrun) rmtree(path string) (errc int) {
	return enumerate(t.fs, path, false, func(path string) int {
		return t.remove(path)
	})
}

func (t *testrun) rename(path string, newpath string) (errc int) {
	defer trace.Trace(0, t.prefix, path, newpath)(&errc)
	errc = t.fs.Rename(path, newpath)
	return
}

func (t *testrun) move(path string) (errc int) {
	pdir := pathutil.Dir(path)
	newpath := ""

	errc, fh := t.fs.Opendir(pdir)
	if 0 == errc {
		names := []string{}
		t.fs.Readdir(pdir, func(name string, stat *fuse.Stat_t, ofst int64) bool {
			if "." == name || ".." == name {
				return true
			}
			names = append(names, name)
			return true
		}, 0, fh)
		sort.Strings(names)
		t.fs.Releasedir(pdir, fh)
		if 0 < len(names) {
			newpath = pathutil.Join(pdir, names[t.r.Int()%len(names)])
		}
	}
	if "" == newpath || path == newpath {
		newpath = pathutil.Join(pdir, t.randname())
	}

	files := []*testFile{}
	defer func() {
		for _, file := range files {
			e := testCloseFile(t.fs, file)
			if 0 == errc {
				errc = e
			}
		}
	}()
	for i := sort.SearchStrings(t.paths, path); len(t.paths) > i && hasPathPrefix(t.paths[i], path, false); i++ {
		e, file := testOpenFile(t.fs, t.paths[i])
		if 0 != e {
			errc = e
			return
		}
		files = append(files, file)
	}

	errc = t.rename(path, newpath)
	switch errc {
	case -fuse.EISDIR, -fuse.ENOTDIR, -fuse.ENOTEMPTY, -fuse.EINVAL:
		// retry renaming to a random name
		newpath = pathutil.Join(pdir, t.randname())
		errc = t.rename(path, newpath)
	}
	if 0 != errc {
		return
	}

	if i := sort.SearchStrings(t.paths, newpath); len(t.paths) > i && t.paths[i] == newpath {
		t.paths = append(t.paths[:i], t.paths[i+1:]...)
	}
	for i := sort.SearchStrings(t.paths, path); len(t.paths) > i && hasPathPrefix(t.paths[i], path, false); i++ {
		t.paths[i] = newpath + t.paths[i][len(path):]
	}
	sort.Strings(t.paths)

	return
}

func (t *testrun) link(path string) (errc int) {
	stat := fuse.Stat_t{}
	errc = t.fs.Getattr(path, &stat, ^uint64(0))
	if 0 == errc && fuse.S_IFDIR == stat.Mode&fuse.S_IFMT {
		// cannot link directories
		return
	}

	pdir := pathutil.Dir(path)
	newpath := pathutil.Join(pdir, t.randname())

	defer trace.Trace(0, t.prefix, path, newpath)(&errc)

	errc = t.fs.Link(path, newpath)
	if 0 != errc {
		return
	}

	t.inspath(newpath)

	return
}

func (t *testrun) populate(path string, maxcnt int, pctdir int, pctsym int) (errc int) {
	for i, filecnt := 0, t.r.Int()%maxcnt; filecnt > i; i++ {
		path := pathutil.Join(path, t.randname())

		if 30 > t.r.Int()%100 {
			stat := fuse.Stat_t{}
			errc = t.fs.Getattr(path, &stat, ^uint64(0))
			if -fuse.ENOENT != errc {
				return -fuse.EIO
			}
		}

		action := t.r.Int() % 100
		switch {
		case action < pctdir:
			errc = t.mkdir(path, 0777)
			if 0 == errc {
				errc = t.populate(path, maxcnt/2, pctdir/2, pctsym/2)
			}
		case action < pctdir+pctsym:
			errc = t.symlink(path)
		default:
			errc = t.mkfile(path, 0777, []uint8(path))
		}
		if 0 != errc {
			return
		}
	}

	return
}

func (t *testrun) randaction(pctact int) (errc int) {
	path := t.paths[t.r.Int()%len(t.paths)]
	action := t.r.Int() % 100
	switch {
	case action < pctact*1:
		errc = t.rmtree(path)
		if 0 == errc && action < pctact/2 {
			errc = t.mkdir(path, 0777)
		}
	case action < pctact*2:
		errc = t.move(path)
	case action < pctact*3:
		errc = t.link(path)
	case action < pctact*4:
		stat := fuse.Stat_t{}
		errc = t.fs.Getattr(path, &stat, ^uint64(0))
		if 0 == errc && fuse.S_IFDIR == stat.Mode&fuse.S_IFMT {
			errc = t.populate(path, maxcnt, pctdir, pctsym)
		}
	case action < pctact*5:
		errc = t.fs.Chmod(path, 0742)
	case action < pctact*6:
		errc, _ = t.fs.Readlink(path)
		if 0 != errc {
			fh := uint64(0)
			errc, fh = t.fs.Open(path, fuse.O_RDWR)
			if -fuse.EISDIR == errc {
				errc = 0
			} else if 0 == errc {
				defer t.fs.Release(path, fh)
				buf := [4096]uint8{}
				n := t.fs.Read(path, buf[:], 0, fh)
				if 0 > n {
					return n
				}
				n = t.fs.Write(path, buf[:n], int64(n), fh)
				if 0 > n {
					return n
				}
			}
		}
	}

	return
}

func (t *testrun) randactions(pctact int) (errc int) {
	for i, n := 0, len(t.paths); n > i; i++ {
		errc = t.randaction(pctact)
		if 0 != errc {
			return
		}
	}

	return
}

func (t *testrun) exercise1() (errc int) {
	errc = t.fs.Mkdir("/dir1", 0777)
	if 0 != errc {
		return
	}
	errc, fh := t.fs.Create("/dir1/file1", fuse.O_CREAT|fuse.O_RDWR, 0666)
	if -fuse.ENOSYS == errc {
		errc = t.fs.Mknod("/dir1/file1", 0666, 0)
		if 0 == errc {
			errc, fh = t.fs.Open("/dir1/file1", fuse.O_RDWR)
		}
	}
	if 0 != errc {
		return
	}
	t.fs.Release("/dir1/file1", fh)

	errc = t.fs.Rename("/dir1/file1", "/dir1/file2")
	if 0 != errc {
		return
	}
	errc = t.fs.Rename("/dir1", "/dir2")
	if 0 != errc {
		return
	}

	errc = t.fs.Unlink("/dir2/file2")
	if 0 != errc {
		return
	}
	errc = t.fs.Rmdir("/dir2")
	if 0 != errc {
		return
	}

	return
}

func (t *testrun) exercise(fs1, fs2, fs3 fuse.FileSystemInterface, maxcnt int, pctdir int, pctact int) (errc int) {
	t.fs = fs1
	errc = t.populate("/", maxcnt, pctdir, pctsym)
	if 0 != errc {
		return
	}

	t.fs = fs2
	errc = t.populate("/", maxcnt, pctdir, pctsym)
	if 0 != errc {
		return
	}

	t.fs = fs3
	errc = t.randactions(pctact)
	if 0 != errc {
		return
	}

	errc = t.exercise1()
	if 0 != errc {
		return
	}
	errc = t.exercise1()
	if 0 != errc {
		return
	}

	return
}

func enumerate(fs fuse.FileSystemInterface, path string, pre bool, fn func(path string) int) (errc int) {
	names := []string{}

	errc, fh := fs.Opendir(path)
	if 0 == errc {
		fs.Readdir(path, func(name string, stat *fuse.Stat_t, ofst int64) bool {
			if "." == name || ".." == name {
				return true
			}
			names = append(names, name)
			return true
		}, 0, fh)
		fs.Releasedir(path, fh)
		sort.Strings(names)
	} else if -fuse.ENOTDIR != errc {
		return
	}

	if pre {
		errc = fn(path)
		if 0 != errc {
			return
		}
	}

	for _, name := range names {
		path := pathutil.Join(path, name)
		errc = enumerate(fs, path, pre, fn)
		if 0 != errc {
			return
		}
	}

	if !pre {
		errc = fn(path)
		if 0 != errc {
			return
		}
	}

	return
}

func readstring(fs fuse.FileSystemInterface, path string) (errc int, data string) {
	errc, target := fs.Readlink(path)
	if 0 == errc {
		return 0, "L:" + target
	}

	errc, fh := fs.Open(path, fuse.O_RDONLY)
	if -fuse.EISDIR == errc {
		errc, fh = fs.Opendir(path)
		if 0 != errc {
			return errc, ""
		}
		fs.Releasedir(path, fh)
		return 0, "D:" + path
	}
	if 0 != errc {
		return errc, ""
	}
	defer fs.Release(path, fh)

	buf := [4096]uint8{}
	n := fs.Read(path, buf[:], 0, fh)
	if 0 > n {
		return n, ""
	}

	return 0, "F:" + string(buf[:n])
}

func compare(fs1, fs2 fuse.FileSystemInterface) (err error) {
	paths1 := []string{}
	enumerate(fs1, "/", true, func(path string) int {
		paths1 = append(paths1, path)
		return 0
	})
	sort.Strings(paths1)

	paths2 := []string{}
	enumerate(fs2, "/", true, func(path string) int {
		paths2 = append(paths2, path)
		return 0
	})
	sort.Strings(paths2)

	if len(paths1) != len(paths2) {
		return errors.New("len(paths1) != len(paths2)")
	}
	for i := 0; len(paths1) > i; i++ {
		if paths1[i] != paths2[i] {
			return errors.New("paths1[i] != paths2[i]")
		}
	}

	for i := 0; len(paths1) > i; i++ {
		stat1 := fuse.Stat_t{}
		e1 := fs1.Getattr(paths1[i], &stat1, ^uint64(0))
		if 0 != e1 {
			return errors.New("0 != e1")
		}
		stat2 := fuse.Stat_t{}
		e2 := fs2.Getattr(paths2[i], &stat2, ^uint64(0))
		if 0 != e2 {
			return errors.New("0 != e2")
		}
		if stat1.Mode != stat2.Mode {
			return errors.New("stat1.Mode != stat2.Mode")
		}

		e1, data1 := readstring(fs1, paths1[i])
		if 0 != e1 {
			return errors.New("0 != e1")
		}
		e2, data2 := readstring(fs2, paths2[i])
		if 0 != e2 {
			return errors.New("0 != e2")
		}
		if data1 != data2 {
			return errors.New("data1 != data2")
		}
	}

	return nil
}

type testFile struct {
	path  string
	fh    uint64
	isdir bool
	data  string
}

func testOpenFile(fs fuse.FileSystemInterface, path string) (errc int, file *testFile) {
	errc, _ = fs.Readlink(path)
	if 0 == errc {
		return 0, &testFile{}
	}

	isdir := false
	errc, fh := fs.Open(path, fuse.O_RDWR)
	if -fuse.EISDIR == errc {
		isdir = true
		errc, fh = fs.Opendir(path)
	}
	if 0 != errc {
		return
	}

	data := ""
	if !isdir {
		buf := [4096]uint8{}
		n := fs.Read(path, buf[:], 0, fh)
		if 0 > n {
			return n, nil
		}
		data = string(buf[:n])
	}

	file = &testFile{path, fh, isdir, data}
	return
}

func testCloseFile(fs fuse.FileSystemInterface, file *testFile) (errc int) {
	if "" == file.path {
		return
	}

	if !file.isdir {
		buf := [4096]uint8{}
		n := fs.Read(file.path, buf[:], 0, file.fh)
		if 0 > n {
			return n
		}
		if string(buf[:n]) != file.data {
			return -fuse.EIO
		}

		fs.Flush(file.path, file.fh)
		errc = fs.Release(file.path, file.fh)
	} else {
		errc = fs.Releasedir(file.path, file.fh)
	}

	return
}

func TestUnionfs(t *testing.T) {
	// w := terminal.Stderr
	// if _, file, _, ok := runtime.Caller(0); ok {
	// 	if f, e := os.Create(pathutil.Join(pathutil.Dir(file), "log.txt")); nil == e {
	// 		w = terminal.NewEscapeWriter(f, "{{ }}", terminal.NullEscapeCode)
	// 	}
	// }
	// trace.Logger = log.New(w, "", log.LstdFlags)
	// trace.Pattern = "github.com/winfsp/hubfs/fs.*"
	// trace.Verbose = true

	seed := time.Now().UnixNano()
	fmt.Println("seed =", seed)

	lazytick := 0 * time.Second

	cfs := newTestfs()
	fs1 := newTestfs()
	fs2 := newTestfs()
	ufs := New(Config{Fslist: []fuse.FileSystemInterface{fs1, fs2}, Lazytick: lazytick})
	ufs.Init()
	defer ufs.Destroy()

	var errc int
	var err error

	t1 := newtestrun(seed)
	t2 := newtestrun(seed)

	errc = t1.exercise(cfs, cfs, cfs, maxcnt, pctdir, pctact)
	if 0 != errc {
		t.Errorf("[cfs] exercise: %v (seed=%v)", errc, seed)
	}

	errc = t2.exercise(fs1, fs2, ufs, maxcnt, pctdir, pctact)
	if 0 != errc {
		t.Errorf("[ufs] exercise: %v (seed=%v)", errc, seed)
	}

	err = compare(cfs, ufs)
	if nil != err {
		t.Errorf("%v (seed=%v)", err, seed)
	}
}
