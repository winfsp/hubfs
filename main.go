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

func run() (ec int) {
	doauth := false
	noauth := false
	grname := ""
	mntopt := mntopt{}
	remote := "github.com"
	mntpnt := ""

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [-o options] [remote] mountpoint\n\n",
			strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe"))
		flag.PrintDefaults()
	}

	flag.BoolVar(&doauth, "doauth", doauth, "perform auth only; do not mount")
	flag.BoolVar(&noauth, "noauth", noauth, "do not perform auth; fail if no auth present")
	flag.StringVar(&grname, "grant", grname, "`name` of key that stores auth grant")
	flag.Var(&mntopt, "o", "FUSE mount `options`")

	flag.Parse()
	switch flag.NArg() {
	case 1:
		mntpnt = flag.Arg(0)
	case 2:
		remote = flag.Arg(0)
		mntpnt = flag.Arg(1)
	default:
		if !doauth {
			flag.Usage()
			return 2
		}
	}
	if doauth && noauth {
		flag.Usage()
		return 2
	}

	// fmt.Printf("doauth=%#v\n", doauth)
	// fmt.Printf("noauth=%#v\n", noauth)
	// fmt.Printf("keynam=%#v\n", grname)
	// fmt.Printf("mntopt=%#v\n", mntopt)
	// fmt.Printf("remote=%#v\n", remote)
	// fmt.Printf("mntpnt=%#v\n", mntpnt)

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

	if "" == grname {
		grname = provname
	}

	var client providers.Client
	token, err := keyring.Get(MyProductName, grname)
	if nil == err {
		client, err = provider.NewClient(token)
		if nil != err {
			keyring.Delete(MyProductName, grname)
		}
	}
	if !noauth && nil != err {
		token, err = provider.Auth()
		if nil == err {
			client, err = provider.NewClient(token)
			if nil == err {
				keyring.Set(MyProductName, grname, token)
			}
		}
	}
	if nil != err {
		warn("client error: %v", err)
		return 1
	}

	// fmt.Printf("token =%#v\n", token)

	if !doauth {
		for _, m := range mntopt {
			for _, s := range strings.Split(m, ",") {
				if "debug" == s {
					libtrace.Verbose = true
					libtrace.Pattern = "github.com/billziss-gh/hubfs/*"
				}
			}
		}

		if !Mount(client, mntpnt, mntopt) {
			ec = 1
		}
	}

	return 0
}

func main() {
	ec := run()
	os.Exit(ec)
}
