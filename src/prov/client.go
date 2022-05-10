/*
 * client.go
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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/billziss-gh/golib/appdata"
)

type client struct {
	api      clientApi
	dir      string
	keepdir  bool
	caseins  bool
	fullrefs bool
	ttl      time.Duration
	lock     sync.Mutex
	cache    *cache
	owners   *cacheImap
	filter   *filterType
}

type owner struct {
	cacheItem
	repositories *cacheImap
	FName        string
	FKind        string
}

type repository struct {
	cacheItem
	Repository
	keepdir bool
	FName   string
	FRemote string
}

type clientApi interface {
	getIdent() string
	getGitCredentials() (string, string)
	getOwner(owner string) (res *owner, err error)
	getRepositories(owner string, kind string) (res []*repository, err error)
}

func (c *client) init(api clientApi) {
	c.api = api
	c.cache = newCache(&c.lock)
	c.cache.Value = c
}

func configValue(s string, k string, v *string) bool {
	if len(s) >= len(k) && s[:len(k)] == k {
		*v = s[len(k):]
		return true
	}
	return false
}

func (c *client) SetConfig(config []string) ([]string, error) {
	res := []string{}
	for _, s := range config {
		v := ""
		switch {
		case configValue(s, "config.dir=", &v):
			if ":" == v {
				if d, e := appdata.CacheDir(); nil == e {
					if p, e := os.Executable(); nil == e {
						n := strings.TrimSuffix(filepath.Base(p), ".exe")
						v = filepath.Join(d, n, c.api.getIdent())
						c.dir = v
						c.keepdir = false
					}
				}
			} else {
				c.dir = v
				c.keepdir = true
			}
		case configValue(s, "config.ttl=", &v):
			if ttl, e := time.ParseDuration(v); nil == e && 0 < ttl {
				c.ttl = ttl
			}
		case configValue(s, "config._caseins=", &v):
			if "1" == v {
				c.caseins = true
			} else {
				c.caseins = false
			}
		case configValue(s, "config._fullrefs=", &v):
			if "1" == v {
				c.fullrefs = true
			} else {
				c.fullrefs = false
			}
		case configValue(s, "config._filter=", &v):
			if nil == c.filter {
				c.filter = &filterType{}
			}
			c.filter.addRule(v)
		default:
			res = append(res, s)
		}
	}

	return res, nil
}

func (c *client) GetDirectory() string {
	c.lock.Lock()
	dir := c.dir
	c.lock.Unlock()
	return dir
}

func (c *client) GetOwners() ([]Owner, error) {
	return []Owner{}, nil
}

func (c *client) OpenOwner(name string) (Owner, error) {
	var res *owner
	var err error

	if nil != c.filter && !c.filter.match(name) {
		return nil, ErrNotFound
	}

	c.lock.Lock()
	if nil != c.owners {
		item, ok := c.owners.Get(name)
		if ok {
			res = item.Value.(*owner)
			c.cache.touchCacheItem(&res.cacheItem, +1)
			c.lock.Unlock()
			return res, nil
		}
	}
	c.lock.Unlock()

	res, err = c.api.getOwner(name)
	if nil != err {
		return nil, err
	}

	c.lock.Lock()
	if nil == c.owners {
		c.owners = c.cache.newCacheImap()
	}
	item, ok := c.owners.Get(name)
	if ok {
		res = item.Value.(*owner)
	} else {
		c.owners.Set(name, &res.MapItem, true)
	}
	c.cache.touchCacheItem(&res.cacheItem, +1)
	c.lock.Unlock()
	return res, nil
}

func (c *client) CloseOwner(O Owner) {
	c.lock.Lock()
	c.cache.touchCacheItem(&O.(*owner).cacheItem, -1)
	c.lock.Unlock()
}

func (c *client) ensureRepositories(o *owner, fn func() error) error {
	c.lock.Lock()
	if nil != o.repositories {
		err := fn()
		c.lock.Unlock()
		return err
	}
	c.lock.Unlock()

	repositories, err := c.api.getRepositories(o.FName, o.FKind)
	if nil != err {
		return err
	}

	c.lock.Lock()
	if nil == o.repositories {
		o.repositories = c.cache.newCacheImap()
		for _, elm := range repositories {
			if nil != c.filter && !c.filter.match(o.FName+"/"+elm.FName) {
				continue
			}
			o.repositories.Set(elm.FName, &elm.MapItem, true)
			c.cache.touchCacheItem(&elm.cacheItem, 0)
		}
	}
	err = fn()
	c.lock.Unlock()
	return err
}

func (c *client) GetRepositories(O Owner) ([]Repository, error) {
	var res []Repository
	var err error

	o := O.(*owner)
	err = c.ensureRepositories(o, func() error {
		res = make([]Repository, len(o.repositories.Items()))
		i := 0
		for _, elm := range o.repositories.Items() {
			res[i] = elm.Value.(Repository)
			i++
		}
		return nil
	})

	return res, err
}

func (c *client) OpenRepository(O Owner, name string) (Repository, error) {
	var res *repository
	var err error

	o := O.(*owner)
	err = c.ensureRepositories(o, func() error {
		item, ok := o.repositories.Get(name)
		if !ok {
			return ErrNotFound
		}
		res = item.Value.(*repository)
		if emptyRepository == res.Repository {
			u, p := c.api.getGitCredentials()
			r := newGitRepository(res.FRemote, u, p, c.caseins, c.fullrefs)
			if "" != c.dir {
				err = r.SetDirectory(filepath.Join(c.dir, o.FName, res.FName))
				if nil != err {
					return err
				}
			}
			res.Repository = r
		}
		c.cache.touchCacheItem(&res.cacheItem, +1)
		return nil
	})
	if nil != err {
		return nil, err
	}

	return res, nil
}

func (c *client) CloseRepository(R Repository) {
	c.lock.Lock()
	c.cache.touchCacheItem(&R.(*repository).cacheItem, -1)
	c.lock.Unlock()
}

func (c *client) StartExpiration() {
	ttl := 30 * time.Second
	if 0 != c.ttl {
		ttl = c.ttl
	}
	c.cache.startExpiration(ttl)
}

func (c *client) StopExpiration() {
	c.cache.stopExpiration()

	c.lock.Lock()
	if "" == c.dir || c.keepdir {
		c.lock.Unlock()
		return
	}
	tmpdir := c.dir + time.Now().Format(".20060102T150405.000Z")
	err := os.Rename(c.dir, tmpdir)
	c.lock.Unlock()
	if nil == err {
		os.RemoveAll(tmpdir)
	}
}

func (o *owner) Name() string {
	return o.FName
}

func (o *owner) expire(c *cache, currentTime time.Time) bool {
	return c.expireCacheItem(&o.cacheItem, currentTime, func() {
		if nil != o.repositories {
			for _, elm := range o.repositories.Items() {
				r := elm.Value.(*repository)
				if emptyRepository != r.Repository {
					// do not expire Owner that has unexpired repositories
					return
				}
			}
		}

		c := c.Value.(*client)
		c.owners.Delete(o.FName)
		tracef("%s", o.FName)
	})
}

func (r *repository) Name() string {
	return r.FName
}

func (r *repository) keep() bool {
	var list []string
	if dir := r.GetDirectory(); "" != dir {
		list, _ = filepath.Glob(filepath.Join(dir, "files/*/.keep"))
	}
	return 0 != len(list)
}

func (r *repository) expire(c *cache, currentTime time.Time) bool {
	return c.expireCacheItem(&r.cacheItem, currentTime, func() {
		if emptyRepository == r.Repository {
			return
		}

		if r.keepdir || r.keep() {
			tracef("repo=%#v", r.FRemote)
		} else {
			err := r.RemoveDirectory()
			tracef("repo=%#v [RemoveDirectory() = %v]", r.FRemote, err)
		}
		r.Close()
		r.Repository = emptyRepository
	})
}

var _ Client = (*client)(nil)
var _ Owner = (*owner)(nil)
var _ Repository = (*repository)(nil)
