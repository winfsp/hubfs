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

package providers

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"runtime"
	"testing"

	"github.com/billziss-gh/golib/keyring"
)

const remote = "https://github.com/billziss-gh/hubfs"
const refName = "refs/heads/master"
const entryName = "README.md"
const subtreeName = "src"
const subentryName = "go.mod"
const commitName = "865aad06c4ecde192460b429f810bb84c0d9ca7b"

var repository Repository
var caseins bool

func TestGetRefs(t *testing.T) {
	refs, err := repository.GetRefs()
	if nil != err {
		t.Error(err)
	}
	found := false
	for _, ref := range refs {
		if ref.Name() == refName {
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
	for _, ref := range refs {
		if ref.Name() == refName {
			found = true
			break
		}
	}
	if !found {
		t.Error()
	}
}

func TestGetRef(t *testing.T) {
	ref, err := repository.GetRef(refName)
	if nil != err {
		t.Error(err)
	}
	if ref.Name() != refName {
		t.Error()
	}

	ref, err = repository.GetRef(refName)
	if nil != err {
		t.Error(err)
	}
	if ref.Name() != refName {
		t.Error()
	}
}

func TestGetTempRef(t *testing.T) {
	ref, err := repository.GetTempRef(commitName)
	if nil != err {
		t.Error(err)
	}
	if ref.Name() != commitName {
		t.Error()
	}

	ref, err = repository.GetTempRef(commitName)
	if nil != err {
		t.Error(err)
	}
	if ref.Name() != commitName {
		t.Error()
	}
}

func TestGetRefTree(t *testing.T) {
	ref, err := repository.GetRef(refName)
	if nil != err {
		t.Error(err)
	}
	if ref.Name() != refName {
		t.Error()
	}

	tree, err := repository.GetTree(ref, nil)
	if nil != err {
		t.Error(err)
	}
	found := false
	for _, entry := range tree {
		if entry.Name() == entryName {
			found = true
			break
		}
	}
	if !found {
		t.Error()
	}

	tree, err = repository.GetTree(ref, nil)
	if nil != err {
		t.Error(err)
	}
	found = false
	for _, entry := range tree {
		if entry.Name() == entryName {
			found = true
			break
		}
	}
	if !found {
		t.Error()
	}
}

func TestGetRefTreeEntry(t *testing.T) {
	ref, err := repository.GetRef(refName)
	if nil != err {
		t.Error(err)
	}
	if ref.Name() != refName {
		t.Error()
	}

	entry, err := repository.GetTreeEntry(ref, nil, entryName)
	if nil != err {
		t.Error(err)
	}
	if entry.Name() != entryName {
		t.Error()
	}

	entry, err = repository.GetTreeEntry(ref, nil, entryName)
	if nil != err {
		t.Error(err)
	}
	if entry.Name() != entryName {
		t.Error()
	}

}

func TestGetTree(t *testing.T) {
	ref, err := repository.GetRef(refName)
	if nil != err {
		t.Error(err)
	}
	if ref.Name() != refName {
		t.Error()
	}

	entry, err := repository.GetTreeEntry(ref, nil, subtreeName)
	if nil != err {
		t.Error(err)
	}
	if entry.Name() != subtreeName {
		t.Error()
	}

	tree, err := repository.GetTree(nil, entry)
	if nil != err {
		t.Error(err)
	}
	found := false
	for _, entry := range tree {
		if entry.Name() == subentryName {
			found = true
			break
		}
	}
	if !found {
		t.Error()
	}

	tree, err = repository.GetTree(nil, entry)
	if nil != err {
		t.Error(err)
	}
	found = false
	for _, entry := range tree {
		if entry.Name() == subentryName {
			found = true
			break
		}
	}
	if !found {
		t.Error()
	}
}

func TestGetTreeEntry(t *testing.T) {
	ref, err := repository.GetRef(refName)
	if nil != err {
		t.Error(err)
	}
	if ref.Name() != refName {
		t.Error()
	}

	entry, err := repository.GetTreeEntry(ref, nil, subtreeName)
	if nil != err {
		t.Error(err)
	}
	if entry.Name() != subtreeName {
		t.Error()
	}

	subentry, err := repository.GetTreeEntry(nil, entry, subentryName)
	if nil != err {
		t.Error(err)
	}
	if subentry.Name() != subentryName {
		t.Error()
	}

	subentry, err = repository.GetTreeEntry(nil, entry, subentryName)
	if nil != err {
		t.Error(err)
	}
	if subentry.Name() != subentryName {
		t.Error()
	}
}

func TestGetBlobReader(t *testing.T) {
	ref, err := repository.GetRef(refName)
	if nil != err {
		t.Error(err)
	}
	if ref.Name() != refName {
		t.Error()
	}

	entry, err := repository.GetTreeEntry(ref, nil, subtreeName)
	if nil != err {
		t.Error(err)
	}
	if entry.Name() != subtreeName {
		t.Error()
	}

	subentry, err := repository.GetTreeEntry(nil, entry, subentryName)
	if nil != err {
		t.Error(err)
	}
	if subentry.Name() != subentryName {
		t.Error()
	}

	reader, err := repository.GetBlobReader(subentry)
	if nil != err {
		t.Error(err)
	}
	content, err := ioutil.ReadAll(reader.(io.Reader))
	if err != nil {
		t.Error(err)
	}
	reader.(io.Closer).Close()
	if !bytes.Contains(content, []byte("module github.com")) {
		t.Error()
	}

	reader, err = repository.GetBlobReader(subentry)
	if nil != err {
		t.Error(err)
	}
	content, err = ioutil.ReadAll(reader.(io.Reader))
	if err != nil {
		t.Error(err)
	}
	reader.(io.Closer).Close()
	if !bytes.Contains(content, []byte("module github.com")) {
		t.Error()
	}
}

func TestGetModule(t *testing.T) {
	const remote = "https://github.com/billziss-gh/winfsp"
	const refName = "refs/heads/master"
	const modulePath = "ext/test"
	const moduleTarget = "/billziss-gh/secfs.test"

	repository, err := NewGitRepository(remote, "", caseins)
	if nil != err {
		t.Error(err)
	}
	defer repository.Close()

	ref, err := repository.GetRef(refName)
	if nil != err {
		t.Error(err)
	}
	if ref.Name() != refName {
		t.Error()
	}

	module, err := repository.GetModule(ref, modulePath, true)
	if nil != err {
		t.Error(err)
	}
	if module != moduleTarget {
		t.Error()
	}

	module, err = repository.GetModule(ref, modulePath, true)
	if nil != err {
		t.Error(err)
	}
	if module != moduleTarget {
		t.Error()
	}
}

func init() {
	atinit(func() error {
		if "windows" == runtime.GOOS || "darwin" == runtime.GOOS {
			caseins = true
		}

		token, err := keyring.Get("hubfs", "https://github.com")
		if nil != err {
			token = ""
		}
		if "" == token {
			token = os.Getenv("HUBFS_TOKEN")
		}

		repository, err = NewGitRepository(remote, token, caseins)
		if nil != err {
			return err
		}

		tdir, err := ioutil.TempDir("", "git_test")
		if nil != err {
			return err
		}

		err = repository.SetDirectory(tdir)
		if nil != err {
			return err
		}

		atexit(func() {
			repository.RemoveDirectory()
			repository.Close()
		})

		return nil
	})
}
