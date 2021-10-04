/*
 * unionfs_test.go
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
	"errors"
	"fmt"
	"math/rand"
	pathutil "path"
	"sort"
	"testing"
	"time"

	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/billziss-gh/golib/trace"
)

type testrun struct {
	prefix string
	r      *rand.Rand
	paths  map[string]struct{}
	fs     fuse.FileSystemInterface
}

var testrunCount = 0

func newtestrun(seed int64) *testrun {
	if 0 == seed {
		seed = time.Now().UnixNano()
	}
	testrunCount++
	return &testrun{
		prefix: fmt.Sprintf("TEST%d", testrunCount),
		r:      rand.New(rand.NewSource(seed)),
		paths:  make(map[string]struct{}),
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

func (t *testrun) mkdir(path string, mode uint32) (errc int) {
	defer trace.Trace(0, t.prefix, path)(&errc)

	errc = t.fs.Mkdir(path, mode)
	if 0 != errc {
		return
	}

	t.paths[path] = struct{}{}

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

	t.paths[path] = struct{}{}

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

	delete(t.paths, path)

	return
}

func (t *testrun) rmtree(path string) (errc int) {
	return enumerate(t.fs, path, false, func(path string) int {
		return t.remove(path)
	})
}

func (t *testrun) rename(path string) (errc int) {
	newpath := pathutil.Join(pathutil.Dir(path), t.randname())
	defer trace.Trace(0, t.prefix, path, newpath)(&errc)

	renpaths := []string{}
	errc += enumerate(t.fs, path, false, func(path string) int {
		renpaths = append(renpaths, path)
		return 0
	})

	errc = t.fs.Rename(path, newpath)
	if 0 != errc {
		return
	}

	for _, p := range renpaths {
		newp := newpath + p[len(path):]
		delete(t.paths, p)
		t.paths[newp] = struct{}{}
	}

	return
}

func (t *testrun) populate(path string, maxcnt int, pctdir int) (errc int) {
	for i, filecnt := 0, t.r.Int()%maxcnt; filecnt > i; i++ {
		path := pathutil.Join(path, t.randname())
		if pctdir > t.r.Int()%100 {
			errc = t.mkdir(path, 0777)
			if 0 == errc {
				errc = t.populate(path, maxcnt/2, pctdir/2)
			}
		} else {
			errc = t.mkfile(path, 0777, []uint8(path))
		}
		if 0 != errc {
			return
		}
	}

	return
}

func (t *testrun) randaction(pctact int) (errc int) {
	paths := []string{}
	for path := range t.paths {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	t.r.Shuffle(len(paths), func(i, j int) {
		paths[i], paths[j] = paths[j], paths[i]
	})

	for _, path := range paths {
		if _, ok := t.paths[path]; !ok {
			continue
		}

		action := t.r.Int() % 100

		switch {
		case action < pctact*1:
			errc = t.rmtree(path)
			if 0 == errc && action < pctact/2 {
				errc = t.mkdir(path, 0777)
			}
		case action < pctact*2:
			errc = t.rename(path)
		}

		if 0 != errc {
			return
		}
	}

	return
}

func (t *testrun) exercise(fs1, fs2, fs3 fuse.FileSystemInterface, maxcnt int, pctdir int, pctact int) (errc int) {
	t.fs = fs1
	errc = t.populate("/", maxcnt, pctdir)
	if 0 != errc {
		return
	}

	t.fs = fs2
	errc = t.populate("/", maxcnt, pctdir)
	if 0 != errc {
		return
	}

	t.fs = fs3
	errc = t.randaction(pctact)
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

	buf := [1024]uint8{}
	n := fs.Read(path, buf[:], 0, fh)
	if 0 > n {
		return n, ""
	}

	return 0, "F:" + string(buf[:n])
}

func compare(fs1, fs2 fuse.FileSystemInterface) (err error) {
	paths1 := []string{}
	enumerate(fs1, "/", true, func(path string) int {
		if "/.unionfs" == path {
			return 0
		}
		paths1 = append(paths1, path)
		return 0
	})
	sort.Strings(paths1)

	paths2 := []string{}
	enumerate(fs2, "/", true, func(path string) int {
		if "/.unionfs" == path {
			return 0
		}
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

func TestUnionfs(t *testing.T) {
	//trace.Pattern = "github.com/billziss-gh/hubfs/fs.*"
	//trace.Verbose = true

	seed := time.Now().UnixNano()
	//seed = int64(1633357707045800000)
	//seed = int64(1633360140471599000)
	fmt.Println("seed =", seed)

	maxcnt := 100
	pctdir := 20
	pctact := 10

	cfs := NewTestfs()
	fs1 := NewTestfs()
	fs2 := NewTestfs()
	ufs := NewUnionfs([]fuse.FileSystemInterface{fs1, fs2}, false)
	ufs.Init()

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

	errc = t1.exercise(cfs, cfs, cfs, maxcnt, pctdir, pctact)
	if 0 != errc {
		t.Errorf("[cfs] exercise: %v (seed=%v)", errc, seed)
	}

	errc = t2.exercise(ufs, ufs, ufs, maxcnt, pctdir, pctact)
	if 0 != errc {
		t.Errorf("[ufs] exercise: %v  (seed=%v)", errc, seed)
	}

	err = compare(cfs, ufs)
	if nil != err {
		t.Errorf("%v (seed=%v)", err, seed)
	}
}
