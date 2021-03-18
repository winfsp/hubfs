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
	"net/url"
	"sync"
)

type Owner struct {
	Name string
}

type Repository struct {
	OwnerName string
	Name      string
}

type Client interface {
	GetOwners() ([]*Owner, error)
	GetOwner(name string) (*Owner, error)
	GetRepositories(owner *Owner) ([]*Repository, error)
	GetRepository(owner *Owner, name string) (*Repository, error)
}

type Provider interface {
	Auth() (string, error)
	NewClient(token string) (Client, error)
}

var lock sync.Mutex
var providers = make(map[string]Provider)

func GetProviderName(uri *url.URL) string {
	u := &url.URL{
		Scheme: uri.Scheme,
		Host:   uri.Host,
	}
	return u.String()
}

func GetProvider(uri string) Provider {
	lock.Lock()
	defer lock.Unlock()
	return providers[uri]
}

func RegisterProvider(uri string, provider Provider) {
	lock.Lock()
	defer lock.Unlock()
	providers[uri] = provider
}
