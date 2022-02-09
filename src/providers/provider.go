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

package providers

import (
	"errors"
	"io"
	"net/url"
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

var lock sync.RWMutex
var providers = make(map[string]Provider)

func GetProviderName(uri *url.URL) string {
	u := &url.URL{
		Scheme: uri.Scheme,
		Host:   uri.Host,
	}
	return u.String()
}

func GetProvider(name string) Provider {
	lock.RLock()
	defer lock.RUnlock()
	return providers[name]
}

func RegisterProvider(name string, provider Provider) {
	lock.Lock()
	defer lock.Unlock()
	providers[name] = provider
}

func trace(vals ...interface{}) func(vals ...interface{}) {
	return libtrace.Trace(1, "", vals...)
}

func tracef(form string, vals ...interface{}) {
	libtrace.Tracef(1, form, vals...)
}
