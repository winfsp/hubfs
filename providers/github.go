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
	"strings"

	"github.com/billziss-gh/hubfs/httputil"
	"github.com/cli/oauth"
)

const (
	hostname     = "github.com"
	clientId     = "4c24e0557d7103e3c4b0" // safe to embed
	clientSecret = "ClientSecret"
	callbackURI  = "http://127.0.0.1/callback"
	scopes       = "repo"
)

type githubProvider struct {
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
	return nil, nil
}

func init() {
	RegisterProvider("https://"+hostname, &githubProvider{})
}
