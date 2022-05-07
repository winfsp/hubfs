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

func (provider *GitlabProvider) Auth() (token string, err error) {
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
			AuthorizeURL: fmt.Sprintf("https://%s/oauth/authorize", provider.Hostname),
			TokenURL:     fmt.Sprintf("https://%s/oauth/token", provider.Hostname),
		},
		ClientID:     provider.ClientId,
		ClientSecret: provider.ClientSecret,
		CallbackURI:  provider.CallbackURI,
		Scopes:       strings.Split(provider.Scopes, ","),
		BrowseURL: func(uri string) error {
			return browser.OpenURL(
				fmt.Sprintf("%s&response_type=code&code_challenge=%s&code_challenge_method=S256",
					uri, code_challenge))
		},
		HTTPClient: &gitlabWebAppFlowHttpClient{
			Client:        httputil.DefaultClient,
			callbackURI:   provider.CallbackURI,
			code_verifier: code_verifier,
		},
	}
	accessToken, err := flow.WebAppFlow()
	if nil != accessToken {
		token = accessToken.Token
	}
	return
}

func (provider *GitlabProvider) NewClient(token string) (Client, error) {
	return nil, fmt.Errorf("unimplemented")
}
