/*
 * overlay.go
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
	/* if have Prefix, clean it up and make sure it does not have more than 3 components */
	c.Prefix = pathutil.Clean(c.Prefix)
	switch c.Prefix {
	case "/", ".":
		c.Prefix = ""
	}
	slashes := 0
	for i := 0; len(c.Prefix) > i; i++ {
		if '/' == c.Prefix[i] {
			slashes++
			if 4 == slashes {
				c.Prefix = c.Prefix[:i]
				break
			}
		}
	}

	if c.Overlay {
		return newOverlay(c)
	} else {
		return new(c)
	}
}

func newOverlay(c Config) fuse.FileSystemInterface {
	scope := c.Prefix
	scopeSlashes := strings.Count(c.Prefix, "/")
	caseins := c.Caseins

	topfs := new(Config{
		Client:  c.Client,
		Prefix:  c.Prefix,
		Caseins: c.Caseins,
	}).(*hubfs)

	split := func(path string) (string, string) {
		slashes := scopeSlashes
		for i := 0; len(path) > i; i++ {
			if '/' == path[i] {
				slashes++
				if 4 == slashes {
					if 0 == i {
						return "/", path
					} else {
						return path[:i], path[i:]
					}
				}
			}
		}
		if 3 == slashes && "/" != path {
			return path, "/"
		}
		return "", path
	}

	newfs := func(prefix string) fuse.FileSystemInterface {
		defer func() {
			if r := recover(); nil != r {
				tracef("prefix=%q !PANIC:%v", prefix, r)
			}
		}()

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

		root := filepath.Join(obs.repository.GetDirectory(), "files")
		err := os.MkdirAll(root, 0700)
		if nil != err {
			topfs.release(obs)
			return nil
		}

		root = filepath.Join(root, n)
		err = os.MkdirAll(root, 0755)
		if nil != err {
			topfs.release(obs)
			return nil
		}

		upfs := ptfs.New(root)
		lofs := new(Config{
			Client:  topfs.client,
			Prefix:  pathutil.Join(scope, prefix),
			Caseins: caseins,
		})
		unfs := unionfs.New(unionfs.Config{
			Fslist:  []fuse.FileSystemInterface{upfs, lofs},
			Caseins: caseins,
		})

		return newShardfs(topfs, obs, unfs)
	}

	return overlayfs.New(overlayfs.Config{
		Topfs:      topfs,
		Split:      split,
		Newfs:      newfs,
		Caseins:    caseins,
		TimeToLive: 1 * time.Second,
	})
}
