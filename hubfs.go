/*
 * hubfs.go
 *
 * Copyright 2021 Bill Zissimopoulos
 */
/*
 * This file is part of Hubfs.
 *
 * It is licensed under the MIT license. The full license text can be found
 * in the License.txt file at the root of this project.
 */

package main

import (
	"io"
	pathutil "path"
	"strings"
	"sync"
	"time"

	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/billziss-gh/golib/config"
	"github.com/billziss-gh/hubfs/providers"
)

type Hubfs struct {
	fuse.FileSystemBase
	client  providers.Client
	prefix  string
	lock    sync.RWMutex
	modules map[string]string
	fh      uint64
	openmap map[uint64]*obstack
}

type obstack struct {
	owner      providers.Owner
	repository providers.Repository
	ref        providers.Ref
	entry      providers.TreeEntry
	reader     io.ReaderAt
}

func fuseErrc(err error) (errc int) {
	errc = -fuse.EIO
	if providers.ErrNotFound == err {
		errc = -fuse.ENOENT
	}
	return
}

func fuseStat(stat *fuse.Stat_t, mode uint32, size int64, time time.Time) {
	switch mode & fuse.S_IFMT {
	case fuse.S_IFDIR:
		mode = fuse.S_IFDIR | 0755
	case fuse.S_IFLNK, 0160000 /* submodule */ :
		mode = fuse.S_IFLNK | 0777
	default:
		mode = fuse.S_IFREG | 0644
		if 0 != mode&0400 {
			mode = fuse.S_IFREG | 0755
		}
	}
	ts := fuse.NewTimespec(time)
	*stat = fuse.Stat_t{
		Mode:     mode,
		Nlink:    1,
		Size:     size,
		Atim:     ts,
		Mtim:     ts,
		Ctim:     ts,
		Birthtim: ts,
	}
}

func split(path string) []string {
	comp := strings.Split(path, "/")[1:]
	if 1 == len(comp) && "" == comp[0] {
		return []string{}
	}
	return comp
}

func (fs *Hubfs) open(path string) (errc int, res *obstack) {
	obs := &obstack{}
	var err error
	for i, c := range split(pathutil.Join(fs.prefix, path)) {
		switch i {
		case 0:
			obs.owner, err = fs.client.OpenOwner(c)
		case 1:
			obs.repository, err = fs.client.OpenRepository(obs.owner, c)
		case 2:
			obs.ref, err = obs.repository.GetRef("refs/heads/" + c)
		default:
			obs.entry, err = obs.repository.GetTreeEntry(obs.ref, obs.entry, c)
		}
		if nil != err {
			fs.release(obs)
			errc = fuseErrc(err)
			return
		}
	}
	res = obs
	return
}

func (fs *Hubfs) release(obs *obstack) {
	if nil != obs.repository {
		fs.client.CloseRepository(obs.repository)
	}
	if nil != obs.owner {
		fs.client.CloseOwner(obs.owner)
	}
}

func (fs *Hubfs) getmodule(obs *obstack, path string) (errc int, module string) {
	path = strings.Join(split(pathutil.Join(fs.prefix, path))[3:], "/")

	fs.lock.RLock()
	if nil != fs.modules {
		module = fs.modules[path]
		fs.lock.RUnlock()
		return
	}
	fs.lock.RUnlock()

	entry, err := obs.repository.GetTreeEntry(obs.ref, nil, ".gitmodules")
	if nil != err {
		errc = -fuse.EIO
		return
	}

	reader, err := obs.repository.GetBlobReader(entry)
	if nil != err {
		errc = -fuse.EIO
		return
	}

	c, err := config.Read(reader.(io.Reader))
	reader.(io.Closer).Close()
	if nil != err {
		errc = -fuse.EIO
		return
	}

	modules := make(map[string]string)
	for _, s := range c {
		p := s["path"]
		u := s["url"]
		if "" != p && "" != u {
			modules[p] = fs.client.ResolveSubmodule(u)
		}
	}

	fs.lock.Lock()
	if nil == fs.modules {
		fs.modules = modules
	}
	module = fs.modules[path]
	fs.lock.Unlock()
	return
}

func (fs *Hubfs) getattr(obs *obstack, entry providers.TreeEntry, path string, stat *fuse.Stat_t) {
	if nil != entry {
		mode := entry.Mode()
		fuseStat(stat, mode, entry.Size(), obs.ref.TreeTime())
		if 0160000 == mode {
			_, module := fs.getmodule(obs, path)
			stat.Size += int64(len(module)) + 1
		}
	} else {
		fuseStat(stat, fuse.S_IFDIR, 0, time.Now())
	}
}

func (fs *Hubfs) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	errc, obs := fs.open(path)
	if 0 != errc {
		return
	}

	fs.getattr(obs, obs.entry, path, stat)

	fs.release(obs)

	return
}

func (fs *Hubfs) Readlink(path string) (errc int, target string) {
	errc, obs := fs.open(path)
	if 0 != errc {
		return
	}

	if nil != obs.entry {
		switch obs.entry.Mode() & fuse.S_IFMT {
		case fuse.S_IFLNK:
			target = obs.entry.Target()
		case 0160000 /* submodule */ :
			_, module := fs.getmodule(obs, path)
			target = module + "/" + obs.entry.Target()
		}
	}

	if "" == target {
		errc = -fuse.EINVAL
	}

	fs.release(obs)

	return
}

func (fs *Hubfs) Opendir(path string) (errc int, fh uint64) {
	errc, obs := fs.open(path)
	if 0 != errc {
		return
	}

	fs.lock.Lock()
	fh = fs.fh
	fs.openmap[fh] = obs
	fs.fh++
	fs.lock.Unlock()

	return
}

func (fs *Hubfs) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) (errc int) {

	fs.lock.RLock()
	obs, ok := fs.openmap[fh]
	fs.lock.RUnlock()
	if !ok {
		errc = -fuse.ENOENT
		return
	}

	stat := fuse.Stat_t{}
	if nil != obs.entry {
		fuseStat(&stat, fuse.S_IFDIR, 0, obs.ref.TreeTime())
	} else {
		fuseStat(&stat, fuse.S_IFDIR, 0, time.Now())
	}
	fill(".", &stat, 0)
	fill("..", &stat, 0)

	if nil != obs.ref {
		if lst, err := obs.repository.GetTree(obs.ref, obs.entry); nil == err {
			for _, elm := range lst {
				n := elm.Name()
				fs.getattr(obs, elm, pathutil.Join(path, n), &stat)
				if !fill(n, &stat, 0) {
					break
				}
			}
		}
	} else if nil != obs.repository {
		if lst, err := obs.repository.GetRefs(); nil == err {
			for _, elm := range lst {
				r := elm.Name()
				n := strings.TrimPrefix(r, "refs/heads/")
				if r == n {
					continue
				}
				if !fill(n, &stat, 0) {
					break
				}
			}
		}
	} else if nil != obs.owner {
		if lst, err := fs.client.GetRepositories(obs.owner); nil == err {
			for _, elm := range lst {
				if !fill(elm.Name(), &stat, 0) {
					break
				}
			}
		}
	} else {
		if lst, err := fs.client.GetOwners(); nil == err {
			for _, elm := range lst {
				if !fill(elm.Name(), &stat, 0) {
					break
				}
			}
		}
	}

	return
}

func (fs *Hubfs) Releasedir(path string, fh uint64) (errc int) {
	fs.lock.Lock()
	obs, ok := fs.openmap[fh]
	if ok {
		delete(fs.openmap, fh)
	}
	fs.lock.Unlock()
	if !ok {
		errc = -fuse.ENOENT
		return
	}

	fs.release(obs)

	return
}

func (fs *Hubfs) Open(path string, flags int) (errc int, fh uint64) {
	errc, obs := fs.open(path)
	if 0 != errc {
		return
	}

	fs.lock.Lock()
	fh = fs.fh
	fs.openmap[fh] = obs
	fs.fh++
	fs.lock.Unlock()

	return
}

func (fs *Hubfs) Read(path string, buff []byte, ofst int64, fh uint64) (n int) {
	var reader io.ReaderAt

	fs.lock.RLock()
	obs, ok := fs.openmap[fh]
	if ok {
		reader = obs.reader
	}
	fs.lock.RUnlock()
	if !ok {
		n = -fuse.ENOENT
		return
	}

	if nil == reader {
		fs.lock.Lock()
		if nil == obs.reader {
			obs.reader, _ = obs.repository.GetBlobReader(obs.entry)
		}
		reader = obs.reader
		fs.lock.Unlock()
		if nil == reader {
			n = -fuse.EIO
			return
		}
	}

	n, err := reader.ReadAt(buff, ofst)
	if nil != err && io.EOF != err {
		n = fuseErrc(err)
		return
	}

	return
}

func (fs *Hubfs) Release(path string, fh uint64) (errc int) {
	fs.lock.Lock()
	obs, ok := fs.openmap[fh]
	if ok {
		delete(fs.openmap, fh)
	}
	fs.lock.Unlock()
	if !ok {
		errc = -fuse.ENOENT
		return
	}

	if closer, ok := obs.reader.(io.Closer); ok {
		closer.Close()
	}

	fs.release(obs)

	return
}

func Mount(client providers.Client, prefix string, mntpnt string, config []string) bool {
	mntopt := []string{}
	for _, s := range config {
		mntopt = append(mntopt, "-o"+s)
	}

	fs := &Hubfs{
		client:  client,
		prefix:  prefix,
		openmap: make(map[uint64]*obstack),
	}
	fs.client.StartExpiration()
	defer fs.client.StopExpiration()

	host := fuse.NewFileSystemHost(fs)
	host.SetCapReaddirPlus(true)
	return host.Mount(mntpnt, mntopt)
}
