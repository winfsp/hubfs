/*
 * git.go
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

package prov

import (
	"bytes"
	"encoding/hex"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/billziss-gh/golib/config"
	"github.com/winfsp/hubfs/git"
)

type gitRepository struct {
	remote  string
	token   string
	caseins bool
	once    sync.Once
	repo    *git.Repository
	lock    sync.RWMutex
	refs    map[string]*gitRef
	dir     string
}

type gitRef struct {
	name       string
	commitHash string
	tree       map[string]*gitTreeEntry
	treeTime   time.Time
	modules    map[string]string
}

type gitTreeEntry struct {
	entry  git.TreeEntry
	size   int64
	target string
	tree   map[string]*gitTreeEntry
}

func NewGitRepository(remote string, token string, caseins bool) (Repository, error) {
	r := &gitRepository{
		remote:  remote,
		token:   token,
		caseins: caseins,
	}

	var err error
	r.once.Do(func() { err = r.open() })
	if nil != err {
		return nil, err
	}

	return r, nil
}

func newGitRepository(remote string, token string, caseins bool) Repository {
	return &gitRepository{
		remote:  remote,
		token:   token,
		caseins: caseins,
	}
}

func (r *gitRepository) open() (err error) {
	r.repo, err = git.OpenRepository(r.remote, r.token)
	return
}

func (r *gitRepository) Close() (err error) {
	if nil != r.repo {
		err = r.repo.Close()
	}
	return
}

func (r *gitRepository) GetDirectory() string {
	r.lock.Lock()
	dir := r.dir
	r.lock.Unlock()
	return dir
}

func (r *gitRepository) SetDirectory(path string) (err error) {
	r.lock.Lock()
	if "" == r.dir {
		err = os.MkdirAll(path, 0700)
		if nil == err {
			r.dir = path
		}
	} else {
		err = os.ErrExist
	}
	r.lock.Unlock()
	return
}

func (r *gitRepository) RemoveDirectory() (err error) {
	r.lock.Lock()
	if "" == r.dir {
		r.lock.Unlock()
		return
	}
	tmpdir := r.dir + time.Now().Format(".20060102T150405.000Z")
	err = os.Rename(r.dir, tmpdir)
	if nil == err {
		r.dir = ""
	}
	r.lock.Unlock()
	if nil == err {
		os.RemoveAll(tmpdir)
	}
	return
}

func objectPath(dir string, hash string) string {
	if 2 < len(hash) {
		return filepath.Join(dir, "objects", hash[:2], hash[2:])
	}
	return ""
}

func writeObject(dir string, hash string, content []byte) {
	p := objectPath(dir, hash)
	if nil == os.MkdirAll(filepath.Dir(p), 0700) {
		err := ioutil.WriteFile(p+".tmp", content, 0700)
		if nil == err {
			err = os.Rename(p+".tmp", p)
		}
		if nil != err {
			os.Remove(p + ".tmp")
		}
	}
}

func containsString(l []string, s string) bool {
	for _, i := range l {
		if i == s {
			return true
		}
	}
	return false
}

func (r *gitRepository) prefetchObjects(dir string, want []string,
	fn func(hash string, size int64) error) error {

	if 0 == len(want) {
		return nil
	}

	if "" != dir {
		w := make([]string, 0, len(want))
		for _, hash := range want {
			info, err := os.Stat(objectPath(dir, hash))
			if nil != err {
				w = append(w, hash)
			} else {
				err = fn(hash, info.Size())
				if nil != err {
					return err
				}
			}
		}

		want = w
		if 0 == len(want) {
			return nil
		}

		return r.repo.FetchObjects(want, func(hash string, ot git.ObjectType, content []byte) error {
			writeObject(dir, hash, content)
			if !containsString(want, hash) {
				return nil
			}
			info, err := os.Stat(objectPath(dir, hash))
			if nil != err {
				return err
			}
			return fn(hash, info.Size())
		})
	} else {
		return r.repo.FetchObjects(want, func(hash string, ot git.ObjectType, content []byte) error {
			if !containsString(want, hash) {
				return nil
			}
			return fn(hash, int64(len(content)))
		})
	}
}

func (r *gitRepository) fetchObjects(dir string, want []string,
	fn func(hash string, content []byte) error) error {

	if 0 == len(want) {
		return nil
	}

	if "" != dir {
		w := make([]string, 0, len(want))
		for _, hash := range want {
			content, err := ioutil.ReadFile(objectPath(dir, hash))
			if nil != err {
				w = append(w, hash)
			} else {
				err = fn(hash, content)
				if nil != err {
					return err
				}
			}
		}

		want = w
		if 0 == len(want) {
			return nil
		}

		return r.repo.FetchObjects(want, func(hash string, ot git.ObjectType, content []byte) error {
			writeObject(dir, hash, content)
			if !containsString(want, hash) {
				return nil
			}
			return fn(hash, content)
		})
	} else {
		return r.repo.FetchObjects(want, func(hash string, ot git.ObjectType, content []byte) error {
			if !containsString(want, hash) {
				return nil
			}
			return fn(hash, content)
		})
	}
}

func (r *gitRepository) refetchObjects(dir string, want []string,
	fn func(hash string, ot git.ObjectType) error) error {

	if 0 == len(want) {
		return nil
	}

	if "" != dir {
		return r.repo.FetchObjects(want, func(hash string, ot git.ObjectType, content []byte) error {
			writeObject(dir, hash, content)
			if !containsString(want, hash) {
				return nil
			}
			return fn(hash, ot)
		})
	} else {
		return r.repo.FetchObjects(want, func(hash string, ot git.ObjectType, content []byte) error {
			if !containsString(want, hash) {
				return nil
			}
			return fn(hash, ot)
		})
	}
}

type readerAt interface {
	io.Reader
	io.ReaderAt
}

type readerAtNopCloser struct {
	readerAt
}

func (readerAtNopCloser) Close() error {
	return nil
}

func (r *gitRepository) fetchReaders(dir string, want []string,
	fn func(hash string, reader io.ReaderAt) error) error {

	if 0 == len(want) {
		return nil
	}

	if "" != dir {
		w := make([]string, 0, len(want))
		for _, hash := range want {
			reader, err := os.Open(objectPath(dir, hash))
			if nil != err {
				w = append(w, hash)
			} else {
				err = fn(hash, reader)
				if nil != err {
					return err
				}
			}
		}

		want = w
		if 0 == len(want) {
			return nil
		}

		return r.repo.FetchObjects(want, func(hash string, ot git.ObjectType, content []byte) error {
			writeObject(dir, hash, content)
			if !containsString(want, hash) {
				return nil
			}
			reader, err := os.Open(objectPath(dir, hash))
			if nil != err {
				return err
			}
			return fn(hash, reader)
		})
	} else {
		return r.repo.FetchObjects(want, func(hash string, ot git.ObjectType, content []byte) error {
			if !containsString(want, hash) {
				return nil
			}
			reader := readerAtNopCloser{bytes.NewReader(content)}
			return fn(hash, reader)
		})
	}
}

func (r *gitRepository) Name() string {
	return path.Base(r.remote)
}

func (r *gitRepository) ensureRefs(fn func(refs map[string]*gitRef) error) error {
	r.once.Do(func() { r.open() })
	if nil == r.repo {
		return ErrNotFound
	}

	r.lock.RLock()
	if nil != r.refs {
		err := fn(r.refs)
		r.lock.RUnlock()
		return err
	}
	r.lock.RUnlock()

	m, err := r.repo.GetRefs()
	if nil != err {
		return err
	}

	refs := make(map[string]*gitRef, len(m))
	for n, h := range m {
		k := n
		if r.caseins {
			k = strings.ToUpper(k)
		}

		refs[k] = &gitRef{
			name:       n,
			commitHash: h,
		}
	}

	r.lock.Lock()
	if nil == r.refs {
		r.refs = refs
	}
	err = fn(r.refs)
	r.lock.Unlock()
	return err
}

func (r *gitRepository) GetRefs() (res []Ref, err error) {
	err = r.ensureRefs(func(refs map[string]*gitRef) error {
		res = make([]Ref, len(refs))
		i := 0
		for _, e := range refs {
			res[i] = e
			i++
		}
		return nil
	})
	return
}

func (r *gitRepository) GetRef(name string) (res Ref, err error) {
	k := name
	if r.caseins {
		k = strings.ToUpper(k)
	}

	err = r.ensureRefs(func(refs map[string]*gitRef) error {
		var ok bool
		res, ok = refs[k]
		if !ok {
			return ErrNotFound
		}
		return nil
	})
	return
}

func (r *gitRepository) GetTempRef(name string) (res Ref, err error) {
	_, err = hex.DecodeString(name)
	if nil != err {
		return nil, ErrNotFound
	}

	k := name
	if r.caseins {
		k = strings.ToUpper(k)
	}

	err = r.ensureRefs(func(refs map[string]*gitRef) error {
		var ok bool
		res, ok = refs[k]
		if !ok {
			return ErrNotFound
		}
		return nil
	})
	if nil == err {
		return
	}

	r.lock.RLock()
	dir := r.dir
	r.lock.RUnlock()

	err = r.refetchObjects(dir, []string{name}, func(hash string, ot git.ObjectType) error {
		if git.CommitObject != ot {
			return ErrNotFound
		}
		return nil
	})
	if nil != err {
		return
	}

	ref := &gitRef{
		name:       name,
		commitHash: name,
	}
	r.lock.Lock()
	r.refs[k] = ref
	r.lock.Unlock()

	return ref, nil
}

func (r *gitRepository) ensureTree(
	ref0 Ref, entry0 TreeEntry, fn func(tree map[string]*gitTreeEntry) error) error {
	r.once.Do(func() { r.open() })
	if nil == r.repo {
		return ErrNotFound
	}

	ref, _ := ref0.(*gitRef)
	entry, ok := entry0.(*gitTreeEntry)
	if ok && 0040000 != entry.entry.Mode {
		return ErrNotFound
	}

	r.lock.RLock()
	if nil == entry {
		if nil != ref.tree {
			err := fn(ref.tree)
			r.lock.RUnlock()
			return err
		}
	} else {
		if nil != entry.tree {
			err := fn(entry.tree)
			r.lock.RUnlock()
			return err
		}
	}
	dir := r.dir
	r.lock.RUnlock()

	var treeTime time.Time
	want := []string{""}
	if nil == entry {
		err := r.fetchObjects(dir, []string{ref.commitHash}, func(hash string, content []byte) error {
			c, err := git.DecodeCommit(content)
			if nil != err {
				return nil
			}
			treeTime = c.Committer.Time
			want[0] = c.TreeHash
			return nil
		})
		if nil != err {
			return err
		}
	} else {
		want[0] = entry.entry.Hash
	}

	tree := make(map[string]*gitTreeEntry)
	err := r.fetchObjects(dir, want, func(hash string, content []byte) error {
		t, err := git.DecodeTree(content)
		if nil != err {
			return nil
		}
		for _, e := range t {
			k := e.Name
			if r.caseins {
				k = strings.ToUpper(k)
			}

			tree[k] = &gitTreeEntry{entry: *e}
		}
		return nil
	})
	if nil != err {
		return err
	}

	want = make([]string, 0, len(tree))
	entm := make(map[string][]*gitTreeEntry, len(tree))
	for _, e := range tree {
		if 0040000 != e.entry.Mode && 0160000 != e.entry.Mode {
			want = append(want, e.entry.Hash)
			entm[e.entry.Hash] = append(entm[e.entry.Hash], e)
		}
	}
	err = r.prefetchObjects(dir, want, func(hash string, size int64) error {
		l, ok := entm[hash]
		if ok {
			for _, e := range l {
				e.size = size
			}
		}
		return nil
	})
	if nil != err {
		return err
	}

	want = make([]string, 0, len(tree))
	entm = make(map[string][]*gitTreeEntry, len(tree))
	for _, e := range tree {
		if 0120000 == e.entry.Mode {
			want = append(want, e.entry.Hash)
			entm[e.entry.Hash] = append(entm[e.entry.Hash], e)
		} else if 0160000 == e.entry.Mode {
			e.target = e.entry.Hash
			e.size = int64(len(e.target))
		}
	}
	err = r.fetchObjects(dir, want, func(hash string, content []byte) error {
		l, ok := entm[hash]
		if ok {
			t := string(content)
			for _, e := range l {
				e.target = t
			}
		}
		return nil
	})
	if nil != err {
		return err
	}

	r.lock.Lock()
	if nil == entry {
		if nil == ref.tree {
			ref.tree = tree
			ref.treeTime = treeTime
		}
		err = fn(ref.tree)
	} else {
		if nil == entry.tree {
			entry.tree = tree
		}
		err = fn(entry.tree)
	}
	r.lock.Unlock()
	return err
}

func (r *gitRepository) GetTree(ref Ref, entry TreeEntry) (res []TreeEntry, err error) {
	err = r.ensureTree(ref, entry, func(tree map[string]*gitTreeEntry) error {
		res = make([]TreeEntry, len(tree))
		i := 0
		for _, e := range tree {
			res[i] = e
			i++
		}
		return nil
	})
	return
}

func (r *gitRepository) GetTreeEntry(ref Ref, entry TreeEntry, name string) (res TreeEntry, err error) {
	k := name
	if r.caseins {
		k = strings.ToUpper(k)
	}

	err = r.ensureTree(ref, entry, func(tree map[string]*gitTreeEntry) error {
		var ok bool
		res, ok = tree[k]
		if !ok {
			return ErrNotFound
		}
		return nil
	})
	return
}

func (r *gitRepository) GetBlobReader(entry TreeEntry) (res io.ReaderAt, err error) {
	r.once.Do(func() { r.open() })
	if nil == r.repo {
		return nil, ErrNotFound
	}

	r.lock.RLock()
	dir := r.dir
	r.lock.RUnlock()

	want := []string{entry.Hash()}
	err = r.fetchReaders(dir, want, func(hash string, reader io.ReaderAt) error {
		res = reader
		return nil
	})
	return
}

func (r *gitRepository) ensureModules(
	ref0 Ref, fn func(modules map[string]string) error) error {
	r.once.Do(func() { r.open() })
	if nil == r.repo {
		return ErrNotFound
	}

	ref, _ := ref0.(*gitRef)

	r.lock.RLock()
	if nil != ref.modules {
		err := fn(ref.modules)
		r.lock.RUnlock()
		return err
	}
	r.lock.RUnlock()

	entry, err := r.GetTreeEntry(ref, nil, ".gitmodules")
	if nil != err {
		return err
	}

	reader, err := r.GetBlobReader(entry)
	if nil != err {
		return err
	}

	c, err := config.Read(reader.(io.Reader))
	reader.(io.Closer).Close()
	if nil != err {
		return err
	}

	modules := make(map[string]string)
	for _, s := range c {
		p := s["path"]
		u := s["url"]
		if "" != p && "" != u {
			k := p
			if r.caseins {
				k = strings.ToUpper(k)
			}

			modules[k] = u
		}
	}

	r.lock.Lock()
	if nil == ref.modules {
		ref.modules = modules
	}
	err = fn(ref.modules)
	r.lock.Unlock()
	return err
}

func (r *gitRepository) GetModule(ref Ref, path string, rootrel bool) (res string, err error) {
	k := path
	if r.caseins {
		k = strings.ToUpper(k)
	}

	err = r.ensureModules(ref, func(modules map[string]string) error {
		var ok bool
		res, ok = modules[k]
		if !ok {
			return ErrNotFound
		}
		if rootrel {
			u0, e0 := url.Parse(r.remote)
			u1, e1 := url.Parse(res)
			if nil == e0 && nil == e1 {
				if u0.Scheme == u1.Scheme && u0.Host == u1.Host {
					res = strings.TrimSuffix(u1.Path, ".git")
				}
			}
		}
		return nil
	})
	return
}

func (r *gitRef) Name() string {
	return r.name
}

func (r *gitRef) TreeTime() time.Time {
	return r.treeTime
}

func (e *gitTreeEntry) Name() string {
	return e.entry.Name
}

func (e *gitTreeEntry) Mode() uint32 {
	return e.entry.Mode
}

func (e *gitTreeEntry) Size() int64 {
	return e.size
}

func (e *gitTreeEntry) Target() string {
	return e.target
}

func (e *gitTreeEntry) Hash() string {
	return e.entry.Hash
}
