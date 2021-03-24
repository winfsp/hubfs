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
	"io/ioutil"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/billziss-gh/hubfs/providers"
)

type Hubfs struct {
	fuse.FileSystemBase
	client  providers.Client
	prefix  string
	lock    sync.RWMutex
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

func fuseStat(stat *fuse.Stat_t, mode uint32, size int, time time.Time) {
	switch mode & fuse.S_IFMT {
	case fuse.S_IFDIR:
		mode = (mode & fuse.S_IFMT) | 0755
	case fuse.S_IFLNK:
		mode = (mode & fuse.S_IFMT) | 0777
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
		Size:     int64(size),
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

func (fs *Hubfs) open(p string) (errc int, res *obstack) {
	obs := &obstack{}
	var err error
	for i, c := range split(path.Join(fs.prefix, p)) {
		switch i {
		case 0:
			obs.owner, err = fs.client.OpenOwner(c)
		case 1:
			obs.repository, err = fs.client.OpenRepository(obs.owner, c)
		case 2:
			obs.ref, err = obs.repository.GetRef(c)
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

func (fs *Hubfs) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	errc, obs := fs.open(path)
	if 0 != errc {
		return
	}

	if nil != obs.entry {
		fuseStat(stat, obs.entry.Mode(), 0 /* elm.Size*/, obs.ref.TreeTime())
	} else if nil != obs.ref {
		fuseStat(stat, fuse.S_IFDIR, 0, obs.ref.TreeTime())
	} else {
		fuseStat(stat, fuse.S_IFDIR, 0, time.Now())
	}

	fs.release(obs)

	return
}

func (fs *Hubfs) Readlink(path string) (errc int, target string) {
	errc, obs := fs.open(path)
	if 0 != errc {
		return
	}

	if nil != obs.entry && fuse.S_IFLNK == obs.entry.Mode()&fuse.S_IFMT {
		reader, err := obs.repository.GetBlobReader(obs.entry)
		if nil == err {
			bytes, err := ioutil.ReadAll(reader.(io.Reader))
			if nil == err {
				target = string(bytes)
			}
		}
		if nil != err {
			errc = -fuse.EIO
			return
		}
	} else {
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

	if nil != obs.ref {
		stat := fuse.Stat_t{}
		fuseStat(&stat, fuse.S_IFDIR, 0, obs.ref.TreeTime())
		fill(".", &stat, 0)
		fill("..", &stat, 0)
		if lst, err := obs.repository.GetTree(obs.ref, obs.entry); nil == err {
			for _, elm := range lst {
				fuseStat(&stat, elm.Mode(), 0 /*elm.Size*/, obs.ref.TreeTime())
				if !fill(elm.Name(), &stat, 0) {
					break
				}
			}
		}
	} else if nil != obs.repository {
		stat := fuse.Stat_t{}
		fuseStat(&stat, fuse.S_IFDIR, 0, time.Now())
		fill(".", &stat, 0)
		fill("..", &stat, 0)
		if lst, err := obs.repository.GetRefs(); nil == err {
			for _, elm := range lst {
				if !fill(elm.Name(), &stat, 0) {
					break
				}
			}
		}
	} else if nil != obs.owner {
		stat := fuse.Stat_t{}
		fuseStat(&stat, fuse.S_IFDIR, 0, time.Now())
		fill(".", &stat, 0)
		fill("..", &stat, 0)
		if lst, err := fs.client.GetRepositories(obs.owner); nil == err {
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
