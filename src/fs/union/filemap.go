/*
 * filemap.go
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

package union

import (
	pathutil "path"
)

type Filer interface {
	ResetFile(path string, file *interface{}) bool
	ValidateFile(path string, file *interface{})
	InvalidateFile(path string, file *interface{})
}

type Filemap struct {
	Filer
	Caseins bool

	openmap map[uint64]*fileitem
	pathmap map[Pathkey]*fileitem
	nextfh  uint64
}

type fileitem struct {
	prev, next *fileitem
	file       interface{}
}

func NewFilemap(filer Filer, caseins bool) (fm *Filemap) {
	fm = &Filemap{
		Filer:   filer,
		Caseins: caseins,
		openmap: make(map[uint64]*fileitem),
		pathmap: make(map[Pathkey]*fileitem),
	}
	return
}

func (fm *Filemap) NewFile(path string, file interface{}, track bool) (fh uint64) {
	for {
		fh = fm.nextfh
		fm.nextfh++
		_, ok := fm.openmap[fh]
		if !ok && ^uint64(0) != fh {
			break
		}
	}

	f := &fileitem{file: file}
	f.prev = f
	f.next = f
	fm.openmap[fh] = f

	if track {
		k := ComputePathkey(path, fm.Caseins)
		l, ok := fm.pathmap[k]
		if !ok {
			l = &fileitem{}
			l.prev = l
			l.next = l
			fm.pathmap[k] = l
		}
		p := l.prev
		f.next = l
		f.prev = p
		p.next = f
		l.prev = f
	}

	return
}

func (fm *Filemap) DelFile(path string, fh uint64) {
	f, ok := fm.openmap[fh]
	if ok {
		n := f.next
		p := f.prev
		n.prev = p
		p.next = n
		delete(fm.openmap, fh)

		if n != f {
			k := ComputePathkey(path, fm.Caseins)
			l, ok := fm.pathmap[k]
			if ok && l.next == l {
				delete(fm.pathmap, k)
			}
		}
	}
}

func (fm *Filemap) GetFile(path string, fh uint64, okreset bool) (file interface{}) {
	f, ok := fm.openmap[fh]
	if ok && okreset && fm.Filer.ResetFile(path, &f.file) {
		f, ok = fm.openmap[fh]
	}
	if ok {
		fm.Filer.ValidateFile(path, &f.file)
		file = f.file
	}

	return
}

func (fm *Filemap) Remove(path string) {
	k := ComputePathkey(path, fm.Caseins)
	l, ok := fm.pathmap[k]
	if ok {
		for f := l.next; l != f; f = f.next {
			fm.Filer.InvalidateFile(path, &f.file)
			f.prev = f
			f.next = f
		}

		delete(fm.pathmap, k)
	}
}

func (fm *Filemap) Rename(path string, oldpath string, newpath string) {
	k := ComputePathkey(path, fm.Caseins)
	l, ok := fm.pathmap[k]
	if ok {
		for f := l.next; l != f; f = f.next {
			fm.Filer.InvalidateFile(path, &f.file)
		}

		delete(fm.pathmap, k)
		k = ComputePathkey(pathutil.Join(newpath, path[len(oldpath):]), fm.Caseins)
		fm.pathmap[k] = l
	}
}
