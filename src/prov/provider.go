/*
 * provider.go
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
	"errors"
	"io"
	"net/url"
	"sort"
	"sync"
	"time"

	libtrace "github.com/billziss-gh/golib/trace"
)

type Provider interface {
	Auth() (string, error)
	NewClient(token string) (Client, error)
}

type Client interface {
	SetConfig(config []string) ([]string, error)
	GetDirectory() string
	GetOwners() ([]Owner, error)
	OpenOwner(name string) (Owner, error)
	CloseOwner(owner Owner)
	GetRepositories(owner Owner) ([]Repository, error)
	OpenRepository(owner Owner, name string) (Repository, error)
	CloseRepository(repository Repository)
	StartExpiration()
	StopExpiration()
}

type Owner interface {
	Name() string
}

type Repository interface {
	io.Closer
	GetDirectory() string
	SetDirectory(path string) error
	RemoveDirectory() error
	Name() string
	GetRefs() ([]Ref, error)
	GetRef(name string) (Ref, error)
	GetTempRef(name string) (Ref, error)
	GetTree(ref Ref, entry TreeEntry) ([]TreeEntry, error)
	GetTreeEntry(ref Ref, entry TreeEntry, name string) (TreeEntry, error)
	GetBlobReader(entry TreeEntry) (io.ReaderAt, error)
	GetModule(ref Ref, path string, rootrel bool) (string, error)
}

type Ref interface {
	Name() string
	TreeTime() time.Time
}

type TreeEntry interface {
	Name() string
	Mode() uint32
	Size() int64
	Target() string
	Hash() string
}

var ErrNotFound = errors.New("not found")

var regmutex sync.RWMutex
var registry = make(map[string]func(uri *url.URL) Provider)
var reghelp = make(map[string]string)

func RegisterProviderClass(name string, ctor func(uri *url.URL) Provider, help string) {
	regmutex.Lock()
	defer regmutex.Unlock()
	registry[name] = ctor
	reghelp[name] = help
}

func GetProviderClassNames() (names []string) {
	regmutex.RLock()
	defer regmutex.RUnlock()
	names = make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return
}

func GetProviderClassHelp(name string) string {
	regmutex.RLock()
	defer regmutex.RUnlock()
	return reghelp[name]
}

func GetProviderInstanceName(uri *url.URL) string {
	regmutex.RLock()
	defer regmutex.RUnlock()
	ctor := registry[uri.Host]
	if nil != ctor {
		return uri.Host
	}
	return uri.Scheme + "://" + uri.Host
}

func NewProviderInstance(uri *url.URL) Provider {
	regmutex.RLock()
	defer regmutex.RUnlock()
	ctor := registry[uri.Host]
	if nil != ctor {
		return ctor(uri)
	}
	ctor = registry[uri.Scheme+":"]
	if nil != ctor {
		return ctor(uri)
	}
	return nil
}

func trace(vals ...interface{}) func(vals ...interface{}) {
	return libtrace.Trace(1, "", vals...)
}

func tracef(form string, vals ...interface{}) {
	libtrace.Tracef(1, form, vals...)
}
