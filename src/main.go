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

	"github.com/billziss-gh/golib/keyring"
	libtrace "github.com/billziss-gh/golib/trace"
	"github.com/winfsp/cgofuse/fuse"
	"github.com/winfsp/hubfs/fs/hubfs"
	"github.com/winfsp/hubfs/fs/port"
	"github.com/winfsp/hubfs/prov"
)

var (
	MyProductName    = "HUBFS"
	MyDescription    = "File system for GitHub"
	MyCopyright      = "2021-2022 Bill Zissimopoulos"
	MyVersion        = "DEVVER"
	MyProductVersion = "PRDVER"
	MyProductTag     = ""
)

var progname = strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")

func warn(format string, a ...interface{}) {
	format = "%s: " + format + "\n"
	a = append([]interface{}{progname}, a...)
	fmt.Fprintf(os.Stderr, format, a...)
}

type optlist []string

// String implements flag.Value.String.
func (mntopt *optlist) String() string {
	return ""
}

// Set implements flag.Value.Set.
func (mntopt *optlist) Set(s string) error {
	*mntopt = append(*mntopt, s)
	return nil
}

func newClientWithKey(provider prov.Provider, authkey string) (
	client prov.Client, err error) {
	token, err := keyring.Get(MyProductName, authkey)
	if nil == err {
		client, err = provider.NewClient(token)
		if nil != err {
			keyring.Delete(MyProductName, authkey)
		}
	}
	return
}

func authNewClientWithKey(provider prov.Provider, authkey string) (
	client prov.Client, err error) {
	token, err := provider.Auth()
	if nil == err {
		client, err = provider.NewClient(token)
		if nil == err {
			keyring.Set(MyProductName, authkey, token)
		}
	}
	return
}

func mount(client prov.Client, overlay bool, prefix string, mntpnt string, config []string) bool {
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
		Overlay: overlay,
	})
	host := fuse.NewFileSystemHost(fs)
	host.SetCapCaseInsensitive(caseins)
	host.SetCapReaddirPlus(true)
	return host.Mount(mntpnt, mntopt)
}

func run() int {
	default_mntopt := optlist{}
	switch runtime.GOOS {
	case "windows":
		default_mntopt = optlist{"uid=-1", "gid=-1", "rellinks", "FileInfoTimeout=-1"}
	case "linux":
		default_mntopt = optlist{"uid=-1", "gid=-1", "default_permissions"}
	case "darwin":
		default_mntopt = optlist{"uid=-1", "gid=-1", "default_permissions", "noapplexattr"}
	}

	debug := false
	printver := false
	authmeth := "full"
	authkey := ""
	authonly := false
	readonly := false
	filter := optlist{}
	mntopt := optlist{}
	remote := "github.com"
	mntpnt := ""
	config := []string{"config.dir=:"}

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [options] [remote] mountpoint\n\n", progname)
		flag.PrintDefaults()
	}

	flag.BoolVar(&debug, "d", debug, "debug output")
	flag.BoolVar(&printver, "version", printver, "print version information")
	flag.StringVar(&authmeth, "auth", "",
		"`method` is from list below; auth tokens are stored in system keyring\n"+
			"- force     perform interactive auth even if token present\n"+
			"- full      perform interactive auth if token not present (default)\n"+
			"- required  auth token required to be present\n"+
			"- optional  auth token will be used if present\n"+
			"- none      do not use auth token even if present\n"+
			"- token=T   use specified auth token T; do not use system keyring")
	flag.StringVar(&authkey, "authkey", authkey, "`name` of key that stores auth token in system keyring")
	flag.BoolVar(&authonly, "authonly", authonly, "perform auth only; do not mount")
	flag.BoolVar(&readonly, "readonly", readonly, "read only file system")
	flag.Var(&filter, "filter",
		"list of `rules` that determine repo availability\n"+
			"- list form: rule1,rule2,...\n"+
			"- rule form: [+-]owner or [+-]owner/repo\n"+
			"- rule is include (+) or exclude (-) (default: include)\n"+
			"- rule owner/repo can use wildcards for pattern matching")
	flag.Var(&mntopt, "o", "FUSE mount `options`\n(default: "+strings.Join(default_mntopt, ",")+")")

	flag.Parse()

	if printver {
		name := MyProductName
		if "" != MyProductTag {
			name += " " + MyProductTag
		}
		fmt.Printf("%s %s (%s) - %s\nCopyright %s\n",
			name, MyProductVersion, MyVersion, MyDescription,
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
		if strings.HasPrefix(authmeth, "token=") {
			break
		}
		flag.Usage()
		return 2
	}

	if debug {
		libtrace.Verbose = true
		libtrace.Pattern = "*,github.com/winfsp/hubfs/*,github.com/winfsp/hubfs/fs/*"
	}

	uri, err := url.Parse(remote)
	if nil != uri && "" == uri.Scheme {
		uri, err = url.Parse("https://" + remote)
	}
	if nil != err {
		warn("invalid remote: %s", remote)
		return 1
	}

	provname := prov.GetProviderName(uri)
	provider := prov.GetProvider(provname)
	if nil == provider {
		warn("unknown provider: %s", provname)
		return 1
	}

	if "" == authkey {
		authkey = provname
	}

	var client prov.Client
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
	default:
		if strings.HasPrefix(authmeth, "token=") {
			client, err = provider.NewClient(strings.TrimPrefix(authmeth, "token="))
		}
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

		if debug {
			mntopt = append(mntopt, "debug")
		}

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

		for _, f := range filter {
			for _, s := range strings.Split(f, ",") {
				config = append(config, "config._filter="+s)
			}
		}

		config, err = client.SetConfig(config)
		if nil != err {
			warn("config error: %v", err)
			return 1
		}

		port.Umask(0)

		if !mount(client, !readonly, uri.Path, mntpnt, config) {
			return 1
		}
	}

	return 0
}

func main() {
	ec := run()
	os.Exit(ec)
}
