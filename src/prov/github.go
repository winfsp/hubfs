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

package prov

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	pathutil "path"
	"strings"

	"github.com/cli/oauth"
	"github.com/winfsp/hubfs/httputil"
)

type GithubProvider struct {
	Hostname     string
	ClientId     string
	ClientSecret string
	CallbackURI  string
	Scopes       string
	ApiURI       string
}

func NewGithubComProvider(uri *url.URL) Provider {
	return &GithubProvider{
		Hostname:     "github.com",
		ClientId:     "4c24e0557d7103e3c4b0", // safe to embed
		ClientSecret: "ClientSecret",
		CallbackURI:  "http://127.0.0.1/callback",
		Scopes:       "repo",
		ApiURI:       "https://api.github.com",
	}
}

func init() {
	RegisterProviderClass("github.com", NewGithubComProvider, ""+
		"[https://]github.com[/owner[/repo]]\n"+
		"    \taccess github.com\n"+
		"    \t- owner     file system root is at owner\n"+
		"    \t- repo      file system root is at owner/repo")
}

func (p *GithubProvider) Auth() (token string, err error) {
	flow := &oauth.Flow{
		Host:         oauth.GitHubHost("https://" + p.Hostname),
		ClientID:     p.ClientId,
		ClientSecret: p.ClientSecret,
		CallbackURI:  p.CallbackURI,
		Scopes:       strings.Split(p.Scopes, ","),
		HTTPClient:   httputil.DefaultClient,
	}
	accessToken, err := flow.DetectFlow()
	if nil != accessToken {
		token = accessToken.Token
	}
	return
}

func (p *GithubProvider) NewClient(token string) (Client, error) {
	return NewGithubClient(p.ApiURI, token)
}

type githubClient struct {
	client
	httpClient *http.Client
	ident      string
	apiURI     string
	gqlApiURI  string
	token      string
	login      string
}

func NewGithubClient(apiURI string, token string) (Client, error) {
	uri, err := url.Parse(apiURI)
	if nil != err {
		return nil, err
	}

	c := &githubClient{
		httpClient: httputil.DefaultClient,
		ident:      uri.Hostname(),
		apiURI:     apiURI,
		gqlApiURI:  apiURI + "/graphql",
		token:      token,
	}
	c.client.init(c)

	if m, _ := pathutil.Match("/api/v*", uri.Path); m {
		c.gqlApiURI = uri.Scheme + "://" + uri.Host + "/api/graphql"
	}

	if "" != c.token {
		rsp, err := c.sendrecv("/user")
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

		c.login = content.Login
	}

	return c, nil
}

func (c *githubClient) getIdent() string {
	return c.ident
}

func (c *githubClient) getToken() string {
	return c.token
}

func (c *githubClient) sendrecv(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", c.apiURI+path, nil)
	if nil != err {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if "" != c.token {
		req.Header.Set("Authorization", "token "+c.token)
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

func (c *githubClient) sendrecvGql(query string) (*http.Response, error) {
	var content = struct {
		Query string `json:"query"`
	}{
		Query: query,
	}
	var body bytes.Buffer
	err := json.NewEncoder(&body).Encode(&content)
	if nil != err {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.gqlApiURI, &body)
	if nil != err {
		return nil, err
	}

	req.Header.Set("Content-type", "application/json")
	if "" != c.token {
		req.Header.Set("Authorization", "token "+c.token)
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

func (c *githubClient) getOwner(o string) (res *owner, err error) {
	defer trace(o)(&err)

	rsp, err := c.sendrecv(fmt.Sprintf("/users/%s", o))
	if nil != err {
		return nil, err
	}
	defer rsp.Body.Close()

	var content struct {
		FName string `json:"login"`
		FKind string `json:"type"`
	}
	err = json.NewDecoder(rsp.Body).Decode(&content)
	if nil != err {
		return nil, err
	}

	res = &owner{
		FName: content.FName,
		FKind: content.FKind,
	}
	res.Value = res
	return
}

func (c *githubClient) getRepositoryPageRest(path string) ([]*repository, error) {
	rsp, err := c.sendrecv(path)
	if nil != err {
		return nil, err
	}
	defer rsp.Body.Close()

	var content []struct {
		FName   string `json:"name"`
		FRemote string `json:"clone_url"`
	}
	err = json.NewDecoder(rsp.Body).Decode(&content)
	if nil != err {
		return nil, err
	}

	res := make([]*repository, len(content))
	for i, elm := range content {
		r := &repository{
			FName:   elm.FName,
			FRemote: elm.FRemote,
		}
		r.Value = r
		r.Repository = emptyRepository
		r.keepdir = c.keepdir
		res[i] = r
	}

	return res, nil
}

func (c *githubClient) getRepositoriesRest(owner string, kind string) (res []*repository, err error) {
	defer trace(owner)(&err)

	var path string
	if "Organization" == kind {
		path = fmt.Sprintf("/orgs/%s/repos?type=all&per_page=100", owner)
	} else if c.login == owner {
		path = "/user/repos?visibility=all&affiliation=owner&per_page=100"
	} else {
		path = fmt.Sprintf("/users/%s/repos?type=owner&per_page=100", owner)
	}

	res = make([]*repository, 0)
	for page := 1; ; page++ {
		lst, err := c.getRepositoryPageRest(path + fmt.Sprintf("&page=%d", page))
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

func (c *githubClient) getRepositoryPageGql(query string) ([]*repository, string, error) {
	rsp, err := c.sendrecvGql(query)
	if nil != err {
		return nil, "", err
	}
	defer rsp.Body.Close()

	var content struct {
		Data struct {
			Owner struct {
				Repositories struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []struct {
						FName   string `json:"name"`
						FRemote string `json:"url"`
					} `json:"nodes"`
				} `json:"repositories"`
			} `json:"owner"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	err = json.NewDecoder(rsp.Body).Decode(&content)
	if nil != err {
		return nil, "", err
	}
	if 0 < len(content.Errors) {
		return nil, "", errors.New(fmt.Sprintf("GraphQL: %s", content.Errors[0].Message))
	}

	res := make([]*repository, len(content.Data.Owner.Repositories.Nodes))
	for i, elm := range content.Data.Owner.Repositories.Nodes {
		r := &repository{
			FName:   elm.FName,
			FRemote: elm.FRemote,
		}
		r.Value = r
		r.Repository = emptyRepository
		r.keepdir = c.keepdir
		res[i] = r
	}

	crs := ""
	if content.Data.Owner.Repositories.PageInfo.HasNextPage {
		crs = content.Data.Owner.Repositories.PageInfo.EndCursor
	}

	return res, crs, nil
}

func (c *githubClient) getRepositoriesGql(owner string, kind string) (res []*repository, err error) {
	defer trace(owner)(&err)

	query := `{
		owner: %s {
			repositories(ownerAffiliations: OWNER, first: 100%%s) {
				pageInfo {
					hasNextPage
					endCursor
				}
				nodes {
					name
					url
				}
			}
		}
	}`

	if c.login == owner {
		query = fmt.Sprintf(query, "viewer")
	} else {
		query = fmt.Sprintf(query, `repositoryOwner(login: "`+owner+`")`)
	}

	res = make([]*repository, 0)
	var lst []*repository
	var crs string
	for {
		if "" != crs {
			crs = `, after: "` + crs + `"`
		}
		lst, crs, err = c.getRepositoryPageGql(fmt.Sprintf(query, crs))
		if nil != err {
			return nil, err
		}
		res = append(res, lst...)
		if "" == crs {
			break
		}
	}

	return res, nil
}

func (c *githubClient) getRepositories(owner string, kind string) (res []*repository, err error) {
	if "" != c.token {
		/*
		 * Attempt to list repositories via a GraphQL query because they are much faster for large
		 * listings than REST. For example, listing the GitHub microsoft account takes 1m26s(!)
		 * using REST, but "only" 18s using GraphQL.
		 *
		 * There are however some problems with using GraphQL:
		 *
		 * 1. GraphQL requires authentication.
		 *
		 * 2. Even with authentication GraphQL queries can sometimes fail with OAuth credentials.
		 * The following error message is possible: "Although you appear to have the correct
		 * authorization credentials, the `NAME` organization has enabled OAuth App access
		 * restrictions, meaning that data access to third-parties is limited. For more information
		 * on these restrictions, including how to enable this app, visit
		 * https://docs.github.com/articles/restricting-access-to-your-organization-s-data/"
		 *
		 * For this reason GraphQL queries are not reliable and we always fall back to REST when
		 * encountering an error.
		 *
		 * An alternative solution to this problem was using multiple concurrent requests to fetch
		 * the listing pages. Unfortunately the GitHub API discourages such use, because of
		 * secondary rate limiting:
		 * https://docs.github.com/en/rest/overview/resources-in-the-rest-api#secondary-rate-limits.
		 */
		res, err = c.getRepositoriesGql(owner, kind)
		if nil == err {
			return
		}
	}
	return c.getRepositoriesRest(owner, kind)
}
