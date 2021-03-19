/*
 * provider.go
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
	"errors"
	"net/url"
	"sync"

	libtrace "github.com/billziss-gh/golib/trace"
)

type Provider interface {
	Auth() (string, error)
	NewClient(token string) (Client, error)
}

type Client interface {
	GetOwners() ([]Owner, error)
	GetOwner(name string, acquire bool) (Owner, error)
	ReleaseOwner(owner Owner)
	GetRepositories(owner Owner) ([]Repository, error)
	GetRepository(owner Owner, name string, acquire bool) (Repository, error)
	ReleaseRepository(repository Repository)
}

type Owner interface {
	Name() string
}

type Repository interface {
	Name() string
}

var ErrNotFound = errors.New("not found")

var lock sync.Mutex
var providers = make(map[string]Provider)

func GetProviderName(uri *url.URL) string {
	u := &url.URL{
		Scheme: uri.Scheme,
		Host:   uri.Host,
	}
	return u.String()
}

func GetProvider(name string) Provider {
	lock.Lock()
	defer lock.Unlock()
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
