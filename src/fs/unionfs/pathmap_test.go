/*
 * pathmap_test.go
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

package unionfs

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/billziss-gh/hubfs/fs/memfs"
)

func TestPathmapOpenClose(t *testing.T) {
	fs := memfs.NewMemfs()

	ec, pm := OpenPathmap(fs, "/.pathmap$", false)
	if 0 != ec {
		t.Error()
	}
	pm.Close()

	ec, pm = OpenPathmap(fs, "/.pathmap$", false)
	if 0 != ec {
		t.Error()
	}
	pm.Close()
}

func TestPathmapGetSet(t *testing.T) {
	fs := memfs.NewMemfs()

	ec, pm := OpenPathmap(fs, "/.pathmap$", false)
	if 0 != ec {
		t.Error()
	}
	defer pm.Close()

	isopq, v := false, UNKNOWN

	pm.Set("/a/bb/ccc", 42)
	isopq, v = pm.Get("/a")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb/ccc")
	if false != isopq || 42 != v {
		t.Error()
	}

	pm.Set("/a/bb", 43)
	isopq, v = pm.Get("/a")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb")
	if false != isopq || 43 != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb/ccc")
	if false != isopq || 42 != v {
		t.Error()
	}

	pm.Set("/a/b", 44)
	isopq, v = pm.Get("/a")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/b")
	if false != isopq || 44 != v {
		t.Error()
	}

	pm.Set("/a/bb/ccc", NOTEXIST)
	pm.Set("/a/bb", NOTEXIST)
	isopq, v = pm.Get("/a")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb")
	if false != isopq || NOTEXIST != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb/ccc")
	if false != isopq || NOTEXIST != v {
		t.Error()
	}

	pm.Set("/a/bb/ccc", 42)
	isopq, v = pm.Get("/a")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb/ccc")
	if false != isopq || 42 != v {
		t.Error()
	}

	if e := pm.SanityCheck(); nil != e {
		t.Error(e)
	}
}

func TestPathmapGetSetOpaque(t *testing.T) {
	fs := memfs.NewMemfs()

	ec, pm := OpenPathmap(fs, "/.pathmap$", false)
	if 0 != ec {
		t.Error()
	}
	defer pm.Close()

	isopq, v := false, UNKNOWN

	pm.Set("/a/bb/ccc", 42)
	isopq, v = pm.Get("/a")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb/ccc")
	if false != isopq || 42 != v {
		t.Error()
	}

	pm.Set("/a/bb", OPAQUE)
	isopq, v = pm.Get("/a")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb")
	if true != isopq || 0 != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb/ccc")
	if true != isopq || 42 != v {
		t.Error()
	}

	if e := pm.SanityCheck(); nil != e {
		t.Error(e)
	}
}

func TestPathmapSetTree(t *testing.T) {
	fs := memfs.NewMemfs()

	ec, pm := OpenPathmap(fs, "/.pathmap$", false)
	if 0 != ec {
		t.Error()
	}
	defer pm.Close()

	isopq, v := false, UNKNOWN

	pm.Set("/a/bb/ccc", 42)
	pm.Set("/a/b/c", 43)
	isopq, v = pm.Get("/a")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb/ccc")
	if false != isopq || 42 != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/b")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/b/c")
	if false != isopq || 43 != v {
		t.Error()
	}

	pm.SetTree("/a", WHITEOUT, NOTEXIST)
	isopq, v = pm.Get("/a")
	if false != isopq || WHITEOUT != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb")
	if false != isopq || NOTEXIST != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb/ccc")
	if false != isopq || NOTEXIST != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/b")
	if false != isopq || NOTEXIST != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/b/c")
	if false != isopq || NOTEXIST != v {
		t.Error()
	}

	if e := pm.SanityCheck(); nil != e {
		t.Error(e)
	}
}

func TestPathmapWriteIncremental(t *testing.T) {
	fs := memfs.NewMemfs()

	ec, pm := OpenPathmap(fs, "/.pathmap$", false)
	if 0 != ec {
		t.Error()
	}
	defer pm.Close()

	isopq, v := false, UNKNOWN

	pm.Set("/a/bb/ccc", 42)
	isopq, v = pm.Get("/a")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb/ccc")
	if false != isopq || 42 != v {
		t.Error()
	}

	n := pm.Write()
	if 0 > n {
		t.Error()
	}

	if e := pm.SanityCheck(); nil != e {
		t.Error(e)
	}

	ec, pm2 := OpenPathmap(fs, "/.pathmap$", false)
	if 0 != ec {
		t.Error()
	}
	isopq, v = pm2.Get("/a")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm2.Get("/a/bb")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm2.Get("/a/bb/ccc")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	if e := pm2.SanityCheck(); nil != e {
		t.Error(e)
	}
	pm2.Close()

	n = pm.Write()
	if 0 > n {
		t.Error()
	}

	if e := pm.SanityCheck(); nil != e {
		t.Error(e)
	}

	ec, pm2 = OpenPathmap(fs, "/.pathmap$", false)
	if 0 != ec {
		t.Error()
	}
	isopq, v = pm2.Get("/a")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm2.Get("/a/bb")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm2.Get("/a/bb/ccc")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	if e := pm2.SanityCheck(); nil != e {
		t.Error(e)
	}
	pm2.Close()

	pm.Set("/a/b/c", 43)
	isopq, v = pm.Get("/a")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb/ccc")
	if false != isopq || 42 != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/b")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/b/c")
	if false != isopq || 43 != v {
		t.Error()
	}

	n = pm.Write()
	if 0 > n {
		t.Error()
	}

	if e := pm.SanityCheck(); nil != e {
		t.Error(e)
	}

	ec, pm2 = OpenPathmap(fs, "/.pathmap$", false)
	if 0 != ec {
		t.Error()
	}
	isopq, v = pm2.Get("/a")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm2.Get("/a/bb")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm2.Get("/a/b")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	if e := pm2.SanityCheck(); nil != e {
		t.Error(e)
	}
	pm2.Close()

	pm.Set("/a/b/c", WHITEOUT)
	pm.Set("/a/b", 50)
	isopq, v = pm.Get("/a")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb/ccc")
	if false != isopq || 42 != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/b")
	if false != isopq || 50 != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/b/c")
	if false != isopq || WHITEOUT != v {
		t.Error()
	}

	n = pm.Write()
	if 0 > n {
		t.Error()
	}

	if e := pm.SanityCheck(); nil != e {
		t.Error(e)
	}

	ec, pm2 = OpenPathmap(fs, "/.pathmap$", false)
	if 0 != ec {
		t.Error()
	}
	isopq, v = pm2.Get("/a")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm2.Get("/a/bb")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm2.Get("/a/bb/ccc")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm2.Get("/a/b")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm2.Get("/a/b/c")
	if false != isopq || WHITEOUT != v {
		t.Error()
	}
	if e := pm2.SanityCheck(); nil != e {
		t.Error(e)
	}
	pm2.Close()

	pm.SetTree("/a", WHITEOUT, NOTEXIST)
	isopq, v = pm.Get("/a")
	if false != isopq || WHITEOUT != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb")
	if false != isopq || NOTEXIST != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb/ccc")
	if false != isopq || NOTEXIST != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/b")
	if false != isopq || NOTEXIST != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/b/c")
	if false != isopq || NOTEXIST != v {
		t.Error()
	}

	n = pm.Write()
	if 0 > n {
		t.Error()
	}

	if e := pm.SanityCheck(); nil != e {
		t.Error(e)
	}

	ec, pm2 = OpenPathmap(fs, "/.pathmap$", false)
	if 0 != ec {
		t.Error()
	}
	isopq, v = pm2.Get("/a")
	if false != isopq || WHITEOUT != v {
		t.Error()
	}
	isopq, v = pm2.Get("/a/bb")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm2.Get("/a/bb/ccc")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm2.Get("/a/b")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm2.Get("/a/b/c")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	if e := pm2.SanityCheck(); nil != e {
		t.Error(e)
	}
	pm2.Close()

	pm.Set("/a", 10)
	pm.Set("/a/bb", 11)
	isopq, v = pm.Get("/a")
	if false != isopq || 10 != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb")
	if false != isopq || 11 != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb/ccc")
	if false != isopq || NOTEXIST != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/b")
	if false != isopq || NOTEXIST != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/b/c")
	if false != isopq || NOTEXIST != v {
		t.Error()
	}

	n = pm.Write()
	if 0 > n {
		t.Error()
	}

	if e := pm.SanityCheck(); nil != e {
		t.Error(e)
	}

	ec, pm2 = OpenPathmap(fs, "/.pathmap$", false)
	if 0 != ec {
		t.Error()
	}
	isopq, v = pm2.Get("/a")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm2.Get("/a/bb")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm2.Get("/a/bb/ccc")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm2.Get("/a/b")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm2.Get("/a/b/c")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	if e := pm2.SanityCheck(); nil != e {
		t.Error(e)
	}
	pm2.Close()
}

func TestPathmapWrite(t *testing.T) {
	fs := memfs.NewMemfs()

	ec, pm := OpenPathmap(fs, "/.pathmap$", false)
	if 0 != ec {
		t.Error()
	}
	defer pm.Close()

	N := 10000

	for i := 0; N > i; i++ {
		path := fmt.Sprintf("/%v", i)

		pm.Set(path, OPAQUE)
		isopq, v := pm.Get(path)
		if true != isopq || 0 != v {
			t.Error()
		}

		if 0 == (i+1)%(N/10) {
			n := pm.Write()
			if 0 > n {
				t.Error()
			}

			ec, pm2 := OpenPathmap(fs, "/.pathmap$", false)
			if 0 != ec {
				t.Error()
			}
			if !reflect.DeepEqual(pm.vm, pm2.vm) {
				t.Error()
			}
			if !reflect.DeepEqual(pm.hm, pm2.hm) {
				t.Error()
			}
			pm2.Close()
		}
	}

	for i := 0; N > i; i++ {
		path := fmt.Sprintf("/%v", i)

		pm.Set(path, 42)
		isopq, v := pm.Get(path)
		if false != isopq || 42 != v {
			t.Error()
		}

		if 0 == (i+1)%(N/10) {
			n := pm.Write()
			if 0 > n {
				t.Error()
			}

			ec, pm2 := OpenPathmap(fs, "/.pathmap$", false)
			if 0 != ec {
				t.Error()
			}
			if len(pm2.vm) != N-i {
				t.Error()
			}
			pm2.Close()
		}
	}

	for i := 0; N > i; i++ {
		path := fmt.Sprintf("/%v", i)

		pm.Set(path, WHITEOUT)
		isopq, v := pm.Get(path)
		if false != isopq || WHITEOUT != v {
			t.Error()
		}

		if 0 == (i+1)%(N/2) {
			n := pm.Write()
			if 0 > n {
				t.Error()
			}

			ec, pm2 := OpenPathmap(fs, "/.pathmap$", false)
			if 0 != ec {
				t.Error()
			}
			if len(pm2.vm) != (i + 2) {
				t.Error()
			}
			pm2.Close()
		}
	}

	ec, pm2 := OpenPathmap(fs, "/.pathmap$", false)
	if 0 != ec {
		t.Error()
	}
	if !reflect.DeepEqual(pm.vm, pm2.vm) {
		t.Error()
	}
	if !reflect.DeepEqual(pm.hm, pm2.hm) {
		t.Error()
	}
	pm2.Close()
}

func TestPathmapPurge(t *testing.T) {
	fs := memfs.NewMemfs()

	ec, pm := OpenPathmap(fs, "/.pathmap$", false)
	if 0 != ec {
		t.Error()
	}
	defer pm.Close()

	isopq, v := false, UNKNOWN

	pm.Set("/a/bb/ccc", 42)
	isopq, v = pm.Get("/a")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb/ccc")
	if false != isopq || 42 != v {
		t.Error()
	}

	n := pm.Write()
	if 0 > n {
		t.Error()
	}

	pm.Purge()

	isopq, v = pm.Get("/a")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}
	isopq, v = pm.Get("/a/bb/ccc")
	if false != isopq || UNKNOWN != v {
		t.Error()
	}

	if 3 != len(pm.vm) {
		t.Error()
	}
	if 3 != len(pm.hm) {
		t.Error()
	}

	ec, pm2 := OpenPathmap(fs, "/.pathmap$", false)
	if 0 != ec {
		t.Error()
	}
	if !reflect.DeepEqual(pm.vm, pm2.vm) {
		t.Error()
	}
	if !reflect.DeepEqual(pm.hm, pm2.hm) {
		t.Error()
	}
	pm2.Close()
}
