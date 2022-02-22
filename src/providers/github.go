/*
 * github.go
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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/billziss-gh/golib/appdata"
	"github.com/billziss-gh/hubfs/httputil"
	"github.com/cli/oauth"
)

type GithubProvider struct {
	Hostname     string
	ClientId     string
	ClientSecret string
	CallbackURI  string
	Scopes       string
	ApiURI       string
}

func NewGithubProvider() *GithubProvider {
	return &GithubProvider{
		Hostname:     "github.com",
		ClientId:     "4c24e0557d7103e3c4b0", // safe to embed
		ClientSecret: "ClientSecret",
		CallbackURI:  "http://127.0.0.1/callback",
		Scopes:       "repo",
		ApiURI:       "https://api.github.com",
	}
}

func (provider *GithubProvider) Auth() (token string, err error) {
	flow := &oauth.Flow{
		Hostname:     provider.Hostname,
		ClientID:     provider.ClientId,
		ClientSecret: provider.ClientSecret,
		CallbackURI:  provider.CallbackURI,
		Scopes:       strings.Split(provider.Scopes, ","),
		HTTPClient:   httputil.DefaultClient,
	}
	accessToken, err := flow.DetectFlow()
	if nil != accessToken {
		token = accessToken.Token
	}
	return
}

func (provider *GithubProvider) NewClient(token string) (Client, error) {
	return NewGithubClient(provider.ApiURI, token)
}

func init() {
	provider := NewGithubProvider()
	RegisterProvider("https://"+provider.Hostname, provider)
}

type githubClient struct {
	httpClient *http.Client
	apiURI     string
	token      string
	login      string
	dir        string
	keepdir    bool
	caseins    bool
	ttl        time.Duration
	lock       sync.Mutex
	cache      *cache
	owners     *cacheImap
}

type githubOwner struct {
	cacheItem
	repositories *cacheImap
	FName        string `json:"login"`
}

type githubRepository struct {
	cacheItem
	Repository
	keepdir bool
	FName   string `json:"name"`
	FRemote string `json:"clone_url"`
}

func NewGithubClient(apiURI string, token string) (Client, error) {
	client := &githubClient{
		httpClient: httputil.DefaultClient,
		apiURI:     apiURI,
		token:      token,
	}
	client.cache = newCache(&client.lock)
	client.cache.Value = client

	if "" != client.token {
		rsp, err := client.sendrecv("/user")
		if nil != err {
			return nil, err
		}
		defer rsp.Body.Close()

		var content struct {
			Login string `json:"login"`
		}
		err = json.NewDecoder(rsp.Body).Decode(&content)
		if nil != err {
			return nil, err
		}

		client.login = content.Login
	}

	return client, nil
}

func configValue(s string, k string, v *string) bool {
	if len(s) >= len(k) && s[:len(k)] == k {
		*v = s[len(k):]
		return true
	}
	return false
}

func (client *githubClient) SetConfig(config []string) ([]string, error) {
	res := []string{}
	for _, s := range config {
		v := ""
		switch {
		case configValue(s, "config.dir=", &v):
			if ":" == v {
				if d, e := appdata.CacheDir(); nil == e {
					if p, e := os.Executable(); nil == e {
						if u, e := url.Parse(client.apiURI); nil == e {
							n := strings.TrimSuffix(filepath.Base(p), ".exe")
							v = filepath.Join(d, n, u.Hostname())
							client.dir = v
							client.keepdir = false
						}
					}
				}
			} else {
				client.dir = v
				client.keepdir = true
			}
		case configValue(s, "config.ttl=", &v):
			if ttl, e := time.ParseDuration(v); nil == e && 0 < ttl {
				client.ttl = ttl
			}
		case configValue(s, "config._caseins=", &v):
			if "1" == v {
				client.caseins = true
			} else {
				client.caseins = false
			}
		default:
			res = append(res, s)
		}
	}

	return res, nil
}

func (client *githubClient) sendrecv(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", client.apiURI+path, nil)
	if nil != err {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if "" != client.token {
		req.Header.Set("Authorization", "token "+client.token)
	}

	rsp, err := client.httpClient.Do(req)
	if nil != err {
		return nil, err
	}

	if 404 == rsp.StatusCode {
		return nil, ErrNotFound
	} else if 400 <= rsp.StatusCode {
		return nil, errors.New(fmt.Sprintf("HTTP %d", rsp.StatusCode))
	}

	return rsp, nil
}

func (client *githubClient) getOwner(owner string) (res *githubOwner, err error) {
	defer trace(owner)(&err)

	rsp, err := client.sendrecv(fmt.Sprintf("/users/%s", owner))
	if nil != err {
		return nil, err
	}
	defer rsp.Body.Close()

	var content githubOwner
	err = json.NewDecoder(rsp.Body).Decode(&content)
	if nil != err {
		return nil, err
	}

	content.Value = &content

	return &content, nil
}

func (client *githubClient) getRepositoryPage(path string) ([]*githubRepository, error) {
	rsp, err := client.sendrecv(path)
	if nil != err {
		return nil, err
	}
	defer rsp.Body.Close()

	var content []*githubRepository
	err = json.NewDecoder(rsp.Body).Decode(&content)
	if nil != err {
		return nil, err
	}

	for _, elm := range content {
		elm.Value = elm
		elm.Repository = emptyRepository
		elm.keepdir = client.keepdir
	}

	return content, nil
}

func (client *githubClient) getRepositories(owner string) (res []*githubRepository, err error) {
	defer trace(owner)(&err)

	var path string
	if client.login == owner {
		path = "/user/repos?visibility=all&affiliation=owner,organization_member&per_page=100"
	} else {
		path = fmt.Sprintf("/users/%s/repos?type=owner&per_page=100", owner)
	}

	res = make([]*githubRepository, 0)
	for page := 1; ; page++ {
		lst, err := client.getRepositoryPage(path + fmt.Sprintf("&page=%d", page))
		if nil != err {
			return nil, err
		}
		res = append(res, lst...)
		if len(lst) < 100 {
			break
		}
	}

	return res, nil
}

func (client *githubClient) GetOwners() ([]Owner, error) {
	return []Owner{}, nil
}

func (client *githubClient) OpenOwner(name string) (Owner, error) {
	var res *githubOwner
	var err error

	client.lock.Lock()
	if nil != client.owners {
		item, ok := client.owners.Get(name)
		if ok {
			res = item.Value.(*githubOwner)
			client.cache.touchCacheItem(&res.cacheItem, +1)
			client.lock.Unlock()
			return res, nil
		}
	}
	client.lock.Unlock()

	res, err = client.getOwner(name)
	if nil != err {
		return nil, err
	}

	client.lock.Lock()
	if nil == client.owners {
		client.owners = client.cache.newCacheImap()
	}
	item, ok := client.owners.Get(name)
	if ok {
		res = item.Value.(*githubOwner)
	} else {
		client.owners.Set(name, &res.MapItem, true)
	}
	client.cache.touchCacheItem(&res.cacheItem, +1)
	client.lock.Unlock()
	return res, nil
}

func (client *githubClient) CloseOwner(owner Owner) {
	client.lock.Lock()
	client.cache.touchCacheItem(&owner.(*githubOwner).cacheItem, -1)
	client.lock.Unlock()
}

func (client *githubClient) ensureRepositories(owner *githubOwner, fn func() error) error {
	client.lock.Lock()
	if nil != owner.repositories {
		err := fn()
		client.lock.Unlock()
		return err
	}
	client.lock.Unlock()

	repositories, err := client.getRepositories(owner.FName)
	if nil != err {
		return err
	}

	client.lock.Lock()
	if nil == owner.repositories {
		owner.repositories = client.cache.newCacheImap()
		for _, elm := range repositories {
			owner.repositories.Set(elm.FName, &elm.MapItem, true)
			client.cache.touchCacheItem(&elm.cacheItem, 0)
		}
	}
	err = fn()
	client.lock.Unlock()
	return err
}

func (client *githubClient) GetRepositories(owner0 Owner) ([]Repository, error) {
	var res []Repository
	var err error

	owner := owner0.(*githubOwner)
	err = client.ensureRepositories(owner, func() error {
		res = make([]Repository, len(owner.repositories.Items()))
		i := 0
		for _, elm := range owner.repositories.Items() {
			res[i] = elm.Value.(Repository)
			i++
		}
		return nil
	})

	return res, err
}

func (client *githubClient) OpenRepository(owner0 Owner, name string) (Repository, error) {
	var res *githubRepository
	var err error

	owner := owner0.(*githubOwner)
	err = client.ensureRepositories(owner, func() error {
		item, ok := owner.repositories.Get(name)
		if !ok {
			return ErrNotFound
		}
		res = item.Value.(*githubRepository)
		if emptyRepository == res.Repository {
			r := newGitRepository(res.FRemote, client.token, client.caseins)
			if "" != client.dir {
				err = r.SetDirectory(filepath.Join(client.dir, owner.FName, res.FName))
				if nil != err {
					return err
				}
			}
			res.Repository = r
		}
		client.cache.touchCacheItem(&res.cacheItem, +1)
		return nil
	})
	if nil != err {
		return nil, err
	}

	return res, nil
}

func (client *githubClient) CloseRepository(repository Repository) {
	client.lock.Lock()
	client.cache.touchCacheItem(&repository.(*githubRepository).cacheItem, -1)
	client.lock.Unlock()
}

func (client *githubClient) StartExpiration() {
	ttl := 30 * time.Second
	if 0 != client.ttl {
		ttl = client.ttl
	}
	client.cache.startExpiration(ttl)
}

func (client *githubClient) StopExpiration() {
	client.cache.stopExpiration()

	client.lock.Lock()
	if "" == client.dir || client.keepdir {
		client.lock.Unlock()
		return
	}
	tmpdir := client.dir + time.Now().Format(".20060102T150405.000Z")
	err := os.Rename(client.dir, tmpdir)
	client.lock.Unlock()
	if nil == err {
		os.RemoveAll(tmpdir)
	}
}

func (o *githubOwner) Name() string {
	return o.FName
}

func (o *githubOwner) expire(c *cache, currentTime time.Time) bool {
	return c.expireCacheItem(&o.cacheItem, currentTime, func() {
		if nil != o.repositories {
			for _, elm := range o.repositories.Items() {
				r := elm.Value.(*githubRepository)
				if emptyRepository != r.Repository {
					// do not expire Owner that has unexpired repositories
					return
				}
			}
		}

		client := c.Value.(*githubClient)
		client.owners.Delete(o.FName)
		tracef("%s", o.FName)
	})
}

func (r *githubRepository) Name() string {
	return r.FName
}

func (r *githubRepository) keep() bool {
	var list []string
	if dir := r.GetDirectory(); "" != dir {
		list, _ = filepath.Glob(filepath.Join(dir, "files/*/.keep"))
	}
	return 0 != len(list)
}

func (r *githubRepository) expire(c *cache, currentTime time.Time) bool {
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
