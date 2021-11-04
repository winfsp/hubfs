/*
 * overlay.go
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

package hubfs

import (
	"os"
	pathutil "path"
	"path/filepath"
	"strings"
	"time"

	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/billziss-gh/hubfs/fs/overlayfs"
	"github.com/billziss-gh/hubfs/fs/ptfs"
	"github.com/billziss-gh/hubfs/fs/unionfs"
)

func New(c Config) fuse.FileSystemInterface {
	if c.Overlay {
		return newOverlay(c)
	} else {
		return new(c)
	}
}

func newOverlay(c Config) fuse.FileSystemInterface {
	topfs := new(c).(*hubfs)

	split := func(path string) (string, string) {
		slashes := 0
		for i := 0; len(path) > i; i++ {
			if '/' == path[i] {
				slashes++
				if 4 == slashes {
					return path[:i], path[i:]
				}
			}
		}
		if 3 == slashes {
			return path, "/"
		}
		return "", path
	}

	newfs := func(prefix string) fuse.FileSystemInterface {
		errc, obs := topfs.open(prefix)
		if 0 != errc {
			return nil
		}

		r := obs.ref.Name()
		n := strings.TrimPrefix(r, "refs/heads/")
		if r == n {
			n = strings.TrimPrefix(r, "refs/tags/")
			if r == n {
				n = r
			}
		}
		n = strings.ReplaceAll(n, "/", " ")

		root := filepath.Join(obs.repository.GetDirectory(), "files", n)

		err := os.MkdirAll(root, 0700)
		if nil != err {
			topfs.release(obs)
			return nil
		}

		readd := func(path string) (string, string) {
			return "", pathutil.Join(prefix, path)
		}

		upfs := ptfs.New(root)
		lofs := overlayfs.New(overlayfs.Config{
			Topfs: topfs,
			Split: readd,
		})
		unfs := unionfs.New(unionfs.Config{
			Fslist: []fuse.FileSystemInterface{upfs, lofs},
		})

		return newShardfs(topfs, obs, unfs)
	}

	return overlayfs.New(overlayfs.Config{
		Topfs:      topfs,
		Split:      split,
		Newfs:      newfs,
		TimeToLive: 1 * time.Second,
	})
}
