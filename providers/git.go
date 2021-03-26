/*
 * git.go
 *
 * Copyright 2021 Bill Zissimopoulos
 */
/*
 * This file is part of Hubfs.
 *
 * It is licensed under the MIT license. The full license text can be found
 * in the License.txt file at the root of this project.
 */

package providers

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/billziss-gh/hubfs/git"
)

type gitRepository struct {
	remote string
	token  string
	once   sync.Once
	repo   *git.Repository
	lock   sync.RWMutex
	refs   map[string]*gitRef
	dir    string
}

type gitRef struct {
	name       string
	commitHash string
	tree       map[string]*gitTreeEntry
	treeTime   time.Time
}

type gitTreeEntry struct {
	entry  git.TreeEntry
	size   int64
	target string
	tree   map[string]*gitTreeEntry
}

func NewGitRepository(remote string, token string) (Repository, error) {
	r := &gitRepository{
		remote: remote,
		token:  token,
	}

	var err error
	r.once.Do(func() { err = r.open() })
	if nil != err {
		return nil, err
	}

	return r, nil
}

func newGitRepository(remote string, token string) Repository {
	return &gitRepository{
		remote: remote,
		token:  token,
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
		refs[n] = &gitRef{
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
	err = r.ensureRefs(func(refs map[string]*gitRef) error {
		var ok bool
		res, ok = refs[name]
		if !ok {
			return ErrNotFound
		}
		return nil
	})
	return
}

func (r *gitRepository) ensureTree(
	ref0 Ref, entry0 TreeEntry, fn func(tree map[string]*gitTreeEntry) error) error {
	r.once.Do(func() { r.open() })
	if nil == r.repo {
		return ErrNotFound
	}

	ref, _ := ref0.(*gitRef)
	entry, _ := entry0.(*gitTreeEntry)

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
			tree[e.Name] = &gitTreeEntry{entry: *e}
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
	err = r.ensureTree(ref, entry, func(tree map[string]*gitTreeEntry) error {
		var ok bool
		res, ok = tree[name]
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
