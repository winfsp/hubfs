/*
 * gitlab.go
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
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/cli/browser"
	"github.com/cli/oauth"
	"github.com/winfsp/hubfs/httputil"
)

type GitlabProvider struct {
	Hostname     string
	ClientId     string
	ClientSecret string
	CallbackURI  string
	Scopes       string
	ApiURI       string
}

func NewGitlabComProvider(uri *url.URL) Provider {
	return &GitlabProvider{
		Hostname:     "gitlab.com",
		ClientId:     "034c04be0f0e17bc02fca5dce2b3448fde93a1c06f1a1187825e7140ebc58118", // safe to embed
		ClientSecret: "ClientSecret",
		CallbackURI:  "http://127.0.0.1/callback",
		Scopes:       "read_api,read_user,read_repository",
		ApiURI:       "https://gitlab.com/api/v4",
	}
}

func init() {
	RegisterProviderClass("gitlab.com", NewGitlabComProvider, ""+
		"[https://]gitlab.com[/owner[/repo]]\n"+
		"    \taccess gitlab.com\n"+
		"    \t- owner     file system root is at owner\n"+
		"    \t- repo      file system root is at owner/repo")
}

type gitlabWebAppFlowHttpClient struct {
	*http.Client
	callbackURI   string
	code_verifier string
}

func (c *gitlabWebAppFlowHttpClient) PostForm(url string, data url.Values) (*http.Response, error) {
	data.Del("client_secret")
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", c.callbackURI)
	data.Set("code_verifier", c.code_verifier)
	return c.Client.PostForm(url, data)
}

func (p *GitlabProvider) Auth() (token string, err error) {
	// PKCE (RFC 7636) for GitLab
	buf := make([]byte, 80)
	_, err = rand.Read(buf)
	if nil != err {
		return "", err
	}
	b64 := make([]byte, base64.RawURLEncoding.EncodedLen(len(buf)))
	base64.RawURLEncoding.Encode(b64, buf)
	sum := sha256.Sum256(b64)
	code_verifier := string(b64)
	code_challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	flow := &oauth.Flow{
		Host: &oauth.Host{
			AuthorizeURL: fmt.Sprintf("https://%s/oauth/authorize", p.Hostname),
			TokenURL:     fmt.Sprintf("https://%s/oauth/token", p.Hostname),
		},
		ClientID:     p.ClientId,
		ClientSecret: p.ClientSecret,
		CallbackURI:  p.CallbackURI,
		Scopes:       strings.Split(p.Scopes, ","),
		BrowseURL: func(uri string) error {
			return browser.OpenURL(
				fmt.Sprintf("%s&response_type=code&code_challenge=%s&code_challenge_method=S256",
					uri, code_challenge))
		},
		HTTPClient: &gitlabWebAppFlowHttpClient{
			Client:        httputil.DefaultClient,
			callbackURI:   p.CallbackURI,
			code_verifier: code_verifier,
		},
	}
	accessToken, err := flow.WebAppFlow()
	if nil != accessToken {
		token = accessToken.Token
	}
	return
}

func (p *GitlabProvider) NewClient(token string) (Client, error) {
	return NewGitlabClient(p.ApiURI, token)
}

type gitlabClient struct {
	client
	httpClient *http.Client
	ident      string
	apiURI     string
	token      string
	login      string
}

func NewGitlabClient(apiURI string, token string) (Client, error) {
	uri, err := url.Parse(apiURI)
	if nil != err {
		return nil, err
	}

	c := &gitlabClient{
		httpClient: httputil.DefaultClient,
		ident:      uri.Hostname(),
		apiURI:     apiURI,
		token:      token,
	}
	c.client.init(c)

	if "" != c.token {
		rsp, err := c.sendrecv("/user")
		if nil != err {
			return nil, err
		}
		defer rsp.Body.Close()

		var content struct {
			Login string `json:"username"`
		}
		err = json.NewDecoder(rsp.Body).Decode(&content)
		if nil != err {
			return nil, err
		}

		c.login = content.Login
	}

	return c, nil
}

func (c *gitlabClient) getIdent() string {
	return c.ident
}

func (c *gitlabClient) getGitCredentials() (string, string) {
	return "oauth2", c.token
}

func (c *gitlabClient) sendrecv(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", c.apiURI+path, nil)
	if nil != err {
		return nil, err
	}

	if "" != c.token {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	rsp, err := c.httpClient.Do(req)
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

func (c *gitlabClient) getUser(o string) (res *owner, err error) {
	defer trace(o)(&err)

	rsp, err := c.sendrecv(fmt.Sprintf("/users?username=%s", o))
	if nil != err {
		return nil, err
	}
	defer rsp.Body.Close()

	var content []struct {
		FName string `json:"username"`
	}
	err = json.NewDecoder(rsp.Body).Decode(&content)
	if nil != err {
		return nil, err
	}
	if 0 == len(content) {
		return nil, ErrNotFound
	}

	res = &owner{
		FName: content[0].FName,
		FKind: "user",
	}
	res.Value = res
	return
}

func (c *gitlabClient) getGroup(o string) (res *owner, err error) {
	defer trace(o)(&err)

	rsp, err := c.sendrecv(fmt.Sprintf("/groups/%s?with_projects=false", o))
	if nil != err {
		return nil, err
	}
	defer rsp.Body.Close()

	var content struct {
		FName string `json:"path"`
	}
	err = json.NewDecoder(rsp.Body).Decode(&content)
	if nil != err {
		return nil, err
	}

	res = &owner{
		FName: content.FName,
		FKind: "group",
	}
	res.Value = res
	return
}

func (c *gitlabClient) getOwner(o string) (res *owner, err error) {
	res, err = c.getUser(o)
	if ErrNotFound != err {
		return
	}
	res, err = c.getGroup(o)
	return
}

func (c *gitlabClient) getRepositoryPage(prefix string, path string) ([]*repository, error) {
	rsp, err := c.sendrecv(path)
	if nil != err {
		return nil, err
	}
	defer rsp.Body.Close()

	var content []struct {
		FName   string `json:"path_with_namespace"`
		FRemote string `json:"http_url_to_repo"`
	}
	err = json.NewDecoder(rsp.Body).Decode(&content)
	if nil != err {
		return nil, err
	}

	res := make([]*repository, len(content))
	for i, elm := range content {
		n := elm.FName
		n = strings.TrimPrefix(n, prefix)
		n = strings.ReplaceAll(n, "/", string(AltPathSeparator))
		r := &repository{
			FName:   n,
			FRemote: elm.FRemote,
		}
		r.Value = r
		r.Repository = emptyRepository
		r.keepdir = c.keepdir
		res[i] = r
	}

	return res, nil
}

func (c *gitlabClient) getRepositories(owner string, kind string) (res []*repository, err error) {
	defer trace(owner)(&err)

	var path string
	if "group" == kind {
		path = fmt.Sprintf("/groups/%s/projects?"+
			"include_subgroups=true&simple=true&order_by=id&per_page=100", owner)
	} else {
		path = fmt.Sprintf("/users/%s/projects?"+
			"simple=true&order_by=id&per_page=100", owner)
	}

	res = make([]*repository, 0)
	for page := 1; ; page++ {
		lst, err := c.getRepositoryPage(owner+"/", path+fmt.Sprintf("&page=%d", page))
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
