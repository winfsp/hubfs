/*
 * fetch.go
 *
 * Copyright 2021 Bill Zissimopoulos
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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/billziss-gh/hubfs/git"
)

func warn(format string, a ...interface{}) {
	format = "%s: " + format + "\n"
	a = append([]interface{}{strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")}, a...)
	fmt.Fprintf(os.Stderr, format, a...)
}

func fail(format string, a ...interface{}) {
	warn(format, a...)
	os.Exit(1)
}

func usage() {
	fmt.Println("usage: go run fetch.go repository [want...]")
	os.Exit(2)
}

func main() {
	if 2 > len(os.Args) {
		usage()
	}
	remote := os.Args[1]
	wants := []string{}
	if 2 < len(os.Args) {
		wants = os.Args[2:]
	}

	repository, err := git.OpenRepository(remote, "")
	if nil != err {
		fail("repository error: %v", err)
	}
	defer repository.Close()

	if 0 == len(wants) {
		if m, err := repository.GetRefs(); nil == err {
			for n, h := range m {
				fmt.Println(h, n)
			}
		}
	} else {
		err := repository.FetchObjects(wants, func(hash string, ot git.ObjectType, content []byte) error {
			switch ot {
			case git.CommitObject:
				if c, err := git.DecodeCommit(content); nil == err {
					fmt.Printf("commit %s\n", hash)
					fmt.Printf("Author   : %s <%s> at %s\n",
						c.Author.Name, c.Author.Email, c.Author.Time)
					fmt.Printf("Committer: %s <%s> at %s\n",
						c.Committer.Name, c.Committer.Email, c.Committer.Time)
					fmt.Printf("TreeHash : %s\n",
						c.TreeHash)
					fmt.Println()
				}
			case git.TreeObject:
				if t, err := git.DecodeTree(content); nil == err {
					fmt.Printf("tree %s\n", hash)
					for _, e := range t {
						fmt.Printf("%06o %s %s\n", e.Mode, e.Hash, e.Name)
					}
					fmt.Println()
				}
			case git.BlobObject, git.TagObject:
				fmt.Printf("blob/tag %s\n", hash)
				if 240 < len(content) {
					fmt.Println(string(content[:240]))
				} else {
					fmt.Println(string(content))
				}
				fmt.Println()
			}
			return nil
		})
		if nil != err {
			fail("repository error: %v", err)
		}
	}
}
