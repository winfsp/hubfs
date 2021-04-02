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
	libtrace "github.com/billziss-gh/golib/trace"
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
			// We disallow some names to speed up operations:
			//
			// - All names containing dots: e.g. ".git", ".DS_Store", "autorun.inf"
			// - The special git name HEAD
			if -1 != strings.IndexFunc(c, func(r rune) bool { return '.' == r }) || "HEAD" == c {
				obs.owner, err = nil, providers.ErrNotFound
			} else {
				obs.owner, err = fs.client.OpenOwner(c)
			}
		case 1:
			obs.repository, err = fs.client.OpenRepository(obs.owner, c)
		case 2:
			c = strings.ReplaceAll(c, " ", "/")
			obs.ref, err = obs.repository.GetRef("refs/heads/" + c)
			if providers.ErrNotFound == err {
				obs.ref, err = obs.repository.GetRef("refs/tags/" + c)
				if providers.ErrNotFound == err {
					obs.ref, err = obs.repository.GetTempRef(c)
				}
			}
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

func (fs *Hubfs) getattr(obs *obstack, entry providers.TreeEntry, path string, stat *fuse.Stat_t) (
	target string) {

	if nil != entry {
		mode := entry.Mode()
		fuseStat(stat, mode, entry.Size(), obs.ref.TreeTime())
		switch mode & fuse.S_IFMT {
		case fuse.S_IFLNK:
			target = entry.Target()
			stat.Size = int64(len(target))
		case 0160000 /* submodule */ :
			target = entry.Target()
			path = strings.Join(split(pathutil.Join(fs.prefix, path))[3:], "/")
			module, err := obs.repository.GetModule(obs.ref, path, true)
			module = strings.TrimPrefix(module, strings.TrimSuffix(fs.prefix, "/"))
			if "" != module {
				target = module + "/" + entry.Target()
			} else {
				tracef("repo=%#v Getmodule(ref=%#v, %#v) = %v",
					obs.repository.Name(), obs.ref.Name(), path, err)
			}
			stat.Size = int64(len(target))
		}
	} else {
		fuseStat(stat, fuse.S_IFDIR, 0, time.Now())
	}

	return
}

func (fs *Hubfs) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	defer trace(path, fh)(&errc, stat)

	// The resolve logic below is specific to Windows and WinFsp. An
	// explanation follows.
	//
	// On Windows symbolic links (symlinks) are marked as directory symlinks
	// or file symlinks. This is important for some apps on Windows; for
	// example CMD.EXE is unable to properly CD into a symlink that points to
	// a directory if the symlink is not marked as a directory symlink.
	//
	// When WinFsp-FUSE (the FUSE layer of WinFsp) issues Getattr and sees a
	// symlink it must also inform Windows if it is a directory (see above).
	// At the time I was writing the WinFsp-FUSE layer I got a bit lazy: I
	// should have written the code to issue all the necessary Readlink calls
	// to properly resolve the symlink and then issue Getattr on the result.
	// Instead I punted on this and wrote simple logic to issue a Getattr on
	// the original path+"/." and expected the file system to deal with it.
	//
	// WinFsp-FUSE will only ever send path+"/." in this particular case. The
	// file system is supposed to fill the Stat_t struct with the appropriate
	// file mode that shows whether the (pointed) file is a directory. WinFsp-
	// FUSE will then mark the symlink appropriately.
	//
	// Our resolve logic below works well for the case where the last path
	// component is a symlink. This covers the important use case of
	// submodules. However the logic does not currently handle cases where
	// a symlink is at the middle of the path:
	//
	// path = /name/.../name/LINK1/.                --resolves-to->
	// path = /name/.../name/LINK2/name/.../name    --resolves-to-> ?

	resolve := strings.HasSuffix(path, "/.")
	retries := 0

retry:
	errc, obs := fs.open(path)
	if 0 != errc {
		return
	}

	target := fs.getattr(obs, obs.entry, path, stat)

	fs.release(obs)

	if resolve && "" != target && 16 > retries {
		if '/' == target[0] {
			path = target
		} else {
			path = pathutil.Join(path, "..", target)
		}
		retries++
		goto retry
	}

	return
}

func (fs *Hubfs) Readlink(path string) (errc int, target string) {
	defer trace(path)(&errc, &target)

	errc, obs := fs.open(path)
	if 0 != errc {
		return
	}

	stat := fuse.Stat_t{}
	target = fs.getattr(obs, obs.entry, path, &stat)
	if "" == target {
		errc = -fuse.EINVAL
	}

	fs.release(obs)

	return
}

func (fs *Hubfs) Opendir(path string) (errc int, fh uint64) {
	defer trace(path)(&errc, &fh)

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
	defer trace(path, ofst, fh)(&errc)

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
				n = strings.ReplaceAll(n, "/", " ")
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
	defer trace(path, fh)(&errc)

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
	defer trace(path, flags)(&errc, &fh)

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
	defer trace(path, ofst, fh)(&n)

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
		reader, _ = obs.repository.GetBlobReader(obs.entry)
		if nil == reader {
			n = -fuse.EIO
			return
		}

		var closer io.Closer
		fs.lock.Lock()
		if nil == obs.reader {
			obs.reader = reader
		} else {
			closer = reader.(io.Closer)
			reader = obs.reader
		}
		fs.lock.Unlock()
		if nil != closer {
			closer.Close()
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
	defer trace(path, fh)(&errc)

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
		prefix:  pathutil.Clean(prefix),
		openmap: make(map[uint64]*obstack),
	}
	fs.client.StartExpiration()
	defer fs.client.StopExpiration()

	host := fuse.NewFileSystemHost(fs)
	host.SetCapReaddirPlus(true)
	return host.Mount(mntpnt, mntopt)
}

func trace(vals ...interface{}) func(vals ...interface{}) {
	return libtrace.Trace(1, "", vals...)
}

func tracef(form string, vals ...interface{}) {
	libtrace.Tracef(1, form, vals...)
}
