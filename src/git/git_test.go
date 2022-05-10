/*
 * git_test.go
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

package git

import (
	"os"
	"testing"

	"github.com/billziss-gh/golib/keyring"
	libtrace "github.com/billziss-gh/golib/trace"
)

const remote = "https://github.com/winfsp/hubfs"
const refName = "refs/heads/master"

const hash0 = "90f898ae1f8d3c976f9224d92e3b08d7813e961e"
const hash1 = "609d3b892764952ef69676e653e06b2ca904be18"
const hash2 = "9b3aeb6b08911ee09ecc31c8c87e4905cf8b4dac"

var token string

func TestGetRefs(t *testing.T) {
	repository, err := OpenRepository(remote, token, "x-oauth-basic")
	if nil != err {
		t.Error(err)
	}
	defer repository.Close()

	refs, err := repository.GetRefs()
	if nil != err {
		t.Error(err)
	}
	found := false
	for n := range refs {
		if n == refName {
			found = true
			break
		}
	}
	if !found {
		t.Error()
	}

	refs, err = repository.GetRefs()
	if nil != err {
		t.Error(err)
	}
	found = false
	for n := range refs {
		if n == refName {
			found = true
			break
		}
	}
	if !found {
		t.Error()
	}
}

func TestFetchObjects(t *testing.T) {
	repository, err := OpenRepository(remote, token, "x-oauth-basic")
	if nil != err {
		t.Error(err)
	}
	defer repository.Close()

	wants := []string{
		hash0,
		hash1,
		hash2,
	}
	found0 := false
	found1 := false
	found2 := false
	err = repository.FetchObjects(wants,
		func(hash string, ot ObjectType, content []byte) error {
			if hash0 == hash {
				found0 = true
				_, err := DecodeCommit(content)
				if nil != err {
					return err
				}
			}
			if hash1 == hash {
				found1 = true
				_, err := DecodeTree(content)
				if nil != err {
					return err
				}
			}
			if hash2 == hash {
				found2 = true
				_, err := DecodeTag(content)
				if nil != err {
					return err
				}
			}
			return nil
		})
	if nil != err {
		t.Error(err)
	}
	if !found0 || !found1 || !found2 {
		t.Error()
	}

	wants = []string{
		hash0,
	}
	found0 = false
	err = repository.FetchObjects(wants,
		func(hash string, ot ObjectType, content []byte) error {
			if hash0 == hash {
				found0 = true
				_, err := DecodeCommit(content)
				if nil != err {
					return err
				}
			}
			return nil
		})
	if nil != err {
		t.Error(err)
	}
	if !found0 {
		t.Error()
	}

	wants = []string{
		hash1,
	}
	found1 = false
	err = repository.FetchObjects(wants,
		func(hash string, ot ObjectType, content []byte) error {
			if hash1 == hash {
				found1 = true
				_, err := DecodeTree(content)
				if nil != err {
					return err
				}
			}
			return nil
		})
	if nil != err {
		t.Error(err)
	}
	if !found1 {
		t.Error()
	}
}

func TestMain(m *testing.M) {
	libtrace.Verbose = true
	libtrace.Pattern = "github.com/winfsp/hubfs/*"

	var err error
	token, err = keyring.Get("hubfs", "github.com")
	if nil != err {
		token = ""
	}
	if "" == token {
		token = os.Getenv("HUBFS_TOKEN")
	}

	os.Exit(m.Run())
}
