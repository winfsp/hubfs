/*
 * main.go
 *
 * Copyright 2021 Bill Zissimopoulos
 */
/*
 * This file is part of Hubfs.
 *
 * It is licensed under the MIT license. The full license text can be found
 * in the License.txt file at the root of this project.
 */

package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/billziss-gh/golib/keyring"
	libtrace "github.com/billziss-gh/golib/trace"
	"github.com/billziss-gh/hubfs/providers"
)

var (
	MyProductName = "hubfs"
	MyDescription = "File system for GitHub"
	MyCopyright   = "2021 Bill Zissimopoulos"
	MyRepository  = "https://github.com/billziss-gh/hubfs"
	MyVersion     = "DEVEL"
)

func warn(format string, a ...interface{}) {
	format = "%s: " + format + "\n"
	a = append([]interface{}{strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")}, a...)
	fmt.Fprintf(os.Stderr, format, a...)
}

type mntopt []string

// String implements flag.Value.String.
func (mntopt *mntopt) String() string {
	return ""
}

// Set implements flag.Value.Set.
func (mntopt *mntopt) Set(s string) error {
	*mntopt = append(*mntopt, s)
	return nil
}

func newClientWithKey(provider providers.Provider, authkey string) (
	client providers.Client, err error) {
	token, err := keyring.Get(MyProductName, authkey)
	if nil == err {
		client, err = provider.NewClient(token)
		if nil != err {
			keyring.Delete(MyProductName, authkey)
		}
	}
	return
}

func authNewClientWithKey(provider providers.Provider, authkey string) (
	client providers.Client, err error) {
	token, err := provider.Auth()
	if nil == err {
		client, err = provider.NewClient(token)
		if nil == err {
			keyring.Set(MyProductName, authkey, token)
		}
	}
	return
}

func run() (ec int) {
	authmeth := "full"
	authkey := ""
	authonly := false
	mntopt := mntopt{}
	remote := "github.com"
	mntpnt := ""

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [options] [remote] mountpoint\n\n",
			strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe"))
		flag.PrintDefaults()
	}

	flag.StringVar(&authmeth, "auth", authmeth, "auth method {full, required, optional, none}")
	flag.StringVar(&authkey, "authkey", authkey, "`name` of key that stores auth token")
	flag.BoolVar(&authonly, "authonly", authonly, "perform auth only; do not mount")
	flag.Var(&mntopt, "o", "FUSE mount `options`")

	flag.Parse()
	switch flag.NArg() {
	case 1:
		mntpnt = flag.Arg(0)
	case 2:
		remote = flag.Arg(0)
		mntpnt = flag.Arg(1)
	default:
		if !authonly {
			flag.Usage()
			return 2
		}
	}
	switch authmeth {
	case "full", "required", "optional":
	case "none":
		if authonly {
			flag.Usage()
			return 2
		}
	default:
		flag.Usage()
		return 2
	}

	uri, err := url.Parse(remote)
	if nil != uri && "" == uri.Scheme {
		uri, err = url.Parse("https://" + remote)
	}
	if nil != err {
		warn("invalid remote: %s", remote)
		return 1
	}

	provname := providers.GetProviderName(uri)
	provider := providers.GetProvider(provname)
	if nil == provider {
		warn("unknown provider: %s", provname)
		return 1
	}

	if "" == authkey {
		authkey = provname
	}

	var client providers.Client
	switch authmeth {
	case "full":
		client, err = newClientWithKey(provider, authkey)
		if nil != err {
			client, err = authNewClientWithKey(provider, authkey)
		}
	case "required":
		client, err = newClientWithKey(provider, authkey)
	case "optional":
		client, err = newClientWithKey(provider, authkey)
		if nil != err {
			client, err = provider.NewClient("")
		}
	case "none":
		client, err = provider.NewClient("")
	}
	if nil != err {
		warn("client error: %v", err)
		return 1
	}

	if !authonly {
		config := []string{}
		for _, m := range mntopt {
			for _, s := range strings.Split(m, ",") {
				config = append(config, s)
			}
		}

		for _, s := range config {
			if "debug" == s {
				libtrace.Verbose = true
				libtrace.Pattern = "*,github.com/billziss-gh/hubfs/*"
				break
			}
		}

		_, err = client.SetConfig([]string{"provider.dir=:"})
		if nil != err {
			warn("config error: %v", err)
			return 1
		}

		config, err = client.SetConfig(config)
		if nil != err {
			warn("config error: %v", err)
			return 1
		}

		if !Mount(client, uri.Path, mntpnt, config) {
			ec = 1
		}
	}

	return 0
}

func main() {
	ec := run()
	os.Exit(ec)
}
