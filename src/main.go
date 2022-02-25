/*
 * main.go
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

package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/billziss-gh/golib/keyring"
	libtrace "github.com/billziss-gh/golib/trace"
	"github.com/billziss-gh/hubfs/fs/hubfs"
	"github.com/billziss-gh/hubfs/fs/port"
	"github.com/billziss-gh/hubfs/providers"
)

var (
	MyVersion        = "DEVVER"
	MyProductVersion = "PRDVER"
	MyProductName    = "hubfs"
	MyDescription    = "File system for GitHub"
	MyCopyright      = "2021 Bill Zissimopoulos"
)

var progname = strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")

func warn(format string, a ...interface{}) {
	format = "%s: " + format + "\n"
	a = append([]interface{}{progname}, a...)
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

func mount(client providers.Client, prefix string, mntpnt string, config []string) bool {
	mntopt := []string{}
	for _, s := range config {
		mntopt = append(mntopt, "-o"+s)
	}

	caseins := false
	if "windows" == runtime.GOOS || "darwin" == runtime.GOOS {
		caseins = true
	}

	if caseins {
		client.SetConfig([]string{"config._caseins=1"})
	} else {
		client.SetConfig([]string{"config._caseins=0"})
	}
	client.StartExpiration()
	defer client.StopExpiration()

	fs := hubfs.New(hubfs.Config{
		Client:  client,
		Prefix:  prefix,
		Caseins: caseins,
		Overlay: true,
	})
	host := fuse.NewFileSystemHost(fs)
	host.SetCapCaseInsensitive(caseins)
	host.SetCapReaddirPlus(true)
	return host.Mount(mntpnt, mntopt)
}

func run() int {
	default_mntopt := mntopt{}
	switch runtime.GOOS {
	case "windows":
		default_mntopt = mntopt{"uid=-1", "gid=-1", "rellinks", "FileInfoTimeout=-1"}
	case "linux":
		default_mntopt = mntopt{"uid=-1", "gid=-1", "default_permissions"}
	case "darwin":
		default_mntopt = mntopt{"uid=-1", "gid=-1", "default_permissions", "noapplexattr"}
	}

	printver := false
	authmeth := "full"
	authkey := ""
	authonly := false
	mntopt := mntopt{}
	remote := "github.com"
	mntpnt := ""
	config := []string{"config.dir=:"}

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [options] [remote] mountpoint\n\n", progname)
		flag.PrintDefaults()
	}

	flag.BoolVar(&printver, "version", printver, "print version information")
	flag.StringVar(&authmeth, "auth", "",
		"`method` is from list below; auth tokens are stored in system keyring\n"+
			"- force     perform interactive auth even if token present\n"+
			"- full      perform interactive auth if token not present (default)\n"+
			"- required  auth token required to be present\n"+
			"- optional  auth token will be used if present\n"+
			"- none      do not use auth token even if present")
	flag.StringVar(&authkey, "authkey", authkey, "`name` of key that stores auth token in system keyring")
	flag.BoolVar(&authonly, "authonly", authonly, "perform auth only; do not mount")
	flag.Var(&mntopt, "o", "FUSE mount `options`\n(default: "+strings.Join(default_mntopt, ",")+")")

	flag.Parse()

	if printver {
		fmt.Printf("%s - %s - version %s (%s)\nCopyright %s\n",
			MyProductName, MyDescription, MyProductVersion, MyVersion,
			MyCopyright)
		return 0
	}

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
	case "":
		authmeth = "full"
	case "force", "full", "required", "optional":
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
	case "force":
		client, err = authNewClientWithKey(provider, authkey)
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
		if 0 == len(mntopt) {
			mntopt = default_mntopt
		}
		fmt.Printf("%s -o %s %s %s\n", progname, strings.Join(mntopt, ","), remote, mntpnt)

		for _, m := range mntopt {
			for _, s := range strings.Split(m, ",") {
				if "windows" != runtime.GOOS {
					/* on Windows, WinFsp handles uid=-1,gid=-1 for us */
					if "uid=-1" == s {
						u, _ := user.Current()
						s = "uid=" + u.Uid
					} else if "gid=-1" == s {
						u, _ := user.Current()
						s = "gid=" + u.Gid
					}
				}
				config = append(config, s)
			}
		}

		for _, s := range config {
			if "debug" == s {
				libtrace.Verbose = true
				libtrace.Pattern = "*,github.com/billziss-gh/hubfs/*,github.com/billziss-gh/hubfs/fs/*"
				break
			}
		}

		config, err = client.SetConfig(config)
		if nil != err {
			warn("config error: %v", err)
			return 1
		}

		port.Umask(0)

		if !mount(client, uri.Path, mntpnt, config) {
			return 1
		}
	}

	return 0
}

func main() {
	ec := run()
	os.Exit(ec)
}
