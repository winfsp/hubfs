/*
 * github.go
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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/billziss-gh/golib/cache"
	"github.com/billziss-gh/hubfs/httputil"
	"github.com/cli/oauth"
)

const (
	// Auth
	hostname     = "github.com"
	clientId     = "4c24e0557d7103e3c4b0" // safe to embed
	clientSecret = "ClientSecret"
	callbackURI  = "http://127.0.0.1/callback"
	scopes       = "repo"

	// API
	baseurl = "https://api.github.com"
)

type githubProvider struct {
}

type githubClient struct {
	sync.Mutex
	httpClient *http.Client
	baseurl    string
	token      string
	ttl        time.Duration
	lrulist    cache.MapItem
	owners     *cache.Map
}

type githubOwner struct {
	cachedItem
	repositories *cache.Map
	FName        string `json:"login"`
}

type githubRepository struct {
	cachedItem
	FName string `json:"name"`
}

func (provider *githubProvider) Auth() (token string, err error) {
	flow := &oauth.Flow{
		Hostname:     hostname,
		ClientID:     clientId,
		ClientSecret: clientSecret,
		CallbackURI:  callbackURI,
		Scopes:       strings.Split(scopes, ","),
		HTTPClient:   httputil.DefaultClient,
	}
	accessToken, err := flow.DetectFlow()
	if nil != accessToken {
		token = accessToken.Token
	}
	return
}

func (provider *githubProvider) NewClient(token string) (Client, error) {
	client := &githubClient{
		httpClient: httputil.DefaultClient,
		baseurl:    baseurl,
		token:      token,
	}
	client.lrulist.Empty()
	return client, nil
}

func (client *githubClient) sendrecv(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", client.baseurl+path, nil)
	if nil != err {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Authorization", "token "+client.token)

	rsp, err := client.httpClient.Do(req)
	if nil != err {
		return nil, err
	}

	if 400 <= rsp.StatusCode {
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
	}

	return content, nil
}

func (client *githubClient) getRepositories(owner string) (res []*githubRepository, err error) {
	defer trace(owner)(&err)
	path := fmt.Sprintf("/users/%s/repos?type=owner&per_page=100", owner)

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

func (client *githubClient) GetOwner(name string, acquire bool) (Owner, error) {
	var res *githubOwner
	var err error
	var item *cache.MapItem
	var ok bool

	delta := 0
	if acquire {
		delta = +1
	}

	client.Lock()
	if nil != client.owners {
		item, ok = client.owners.Get(name)
		if ok {
			res = item.Value.(*githubOwner)
			res.cachedItem.touchCachedItem(client.ttl, delta)
		}
	}
	client.Unlock()
	if ok {
		return res, nil
	}

	res, err = client.getOwner(name)
	if nil != err {
		return nil, err
	}

	client.Lock()
	if nil == client.owners {
		client.owners = cache.NewMap(&client.lrulist)
	}
	item, ok = client.owners.Get(name)
	if ok {
		res = item.Value.(*githubOwner)
		res.cachedItem.touchCachedItem(client.ttl, delta)
	} else {
		client.owners.Set(name, &res.MapItem, true)
		res.cachedItem.touchCachedItem(client.ttl, delta)
	}
	client.Unlock()
	return res, nil
}

func (client *githubClient) ReleaseOwner(owner Owner) {
	client.Lock()
	owner.(*githubOwner).cachedItem.touchCachedItem(client.ttl, -1)
	client.Unlock()
}

func (client *githubClient) ensureRepositories(own *githubOwner, fn func() error) error {
	client.Lock()
	if nil != own.repositories {
		err := fn()
		client.Unlock()
		return err
	}
	client.Unlock()

	repositories, err := client.getRepositories(own.FName)
	if nil != err {
		return err
	}

	client.Lock()
	if nil == own.repositories {
		own.repositories = cache.NewMap(&client.lrulist)
		for _, elm := range repositories {
			own.repositories.Set(elm.FName, &elm.MapItem, true)
			elm.cachedItem.touchCachedItem(client.ttl, 0)
		}
	}
	err = fn()
	client.Unlock()
	return err
}

func (client *githubClient) GetRepositories(owner Owner) ([]Repository, error) {
	var res []Repository
	var err error

	own := owner.(*githubOwner)
	err = client.ensureRepositories(own, func() error {
		res = make([]Repository, len(own.repositories.Items()))
		i := 0
		for _, elm := range own.repositories.Items() {
			res[i] = elm.Value.(Repository)
			i++
		}
		return nil
	})

	return res, err
}

func (client *githubClient) GetRepository(owner Owner, name string, acquire bool) (Repository, error) {
	var res *githubRepository
	var err error

	own := owner.(*githubOwner)
	delta := 0
	if acquire {
		delta = +1
	}

	err = client.ensureRepositories(own, func() error {
		var item *cache.MapItem
		var ok bool
		item, ok = own.repositories.Get(name)
		if !ok {
			return ErrNotFound
		}
		res = item.Value.(*githubRepository)
		res.cachedItem.touchCachedItem(client.ttl, delta)
		return nil
	})

	return res, err
}

func (client *githubClient) ReleaseRepository(repository Repository) {
	client.Lock()
	repository.(*githubRepository).cachedItem.touchCachedItem(client.ttl, -1)
	client.Unlock()
}

func (owner *githubOwner) Name() string {
	return owner.FName
}

func (repository *githubRepository) Name() string {
	return repository.FName
}

func init() {
	RegisterProvider("https://"+hostname, &githubProvider{})
}
