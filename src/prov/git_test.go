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

package prov

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"runtime"
	"testing"

	"github.com/billziss-gh/golib/keyring"
)

const remote = "https://github.com/winfsp/hubfs"
const refName = "master"
const tagName = "v1.0B1"
const entryName = "README.md"
const subtreeName = "src"
const subentryName = "go.mod"
const commitName = "865aad06c4ecde192460b429f810bb84c0d9ca7b"

var testRepository Repository
var caseins bool

func TestGetRefs(t *testing.T) {
	refs, err := testRepository.GetRefs()
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

	refs, err = testRepository.GetRefs()
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
	ref, err := testRepository.GetRef(refName)
	if nil != err {
		t.Error(err)
	}
	if ref.Name() != refName {
		t.Error()
	}

	ref, err = testRepository.GetRef(refName)
	if nil != err {
		t.Error(err)
	}
	if ref.Name() != refName {
		t.Error()
	}
}

func TestGetTempRef(t *testing.T) {
	ref, err := testRepository.GetTempRef(commitName)
	if nil != err {
		t.Error(err)
	}
	if ref.Name() != commitName {
		t.Error()
	}

	ref, err = testRepository.GetTempRef(commitName)
	if nil != err {
		t.Error(err)
	}
	if ref.Name() != commitName {
		t.Error()
	}
}

func testGetRefTree(t *testing.T, name string) {
	ref, err := testRepository.GetRef(name)
	if nil != err {
		t.Error(err)
	}
	if ref.Name() != name {
		t.Error()
	}

	tree, err := testRepository.GetTree(ref, nil)
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

	tree, err = testRepository.GetTree(ref, nil)
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

func TestGetRefTree(t *testing.T) {
	testGetRefTree(t, refName)
	testGetRefTree(t, tagName)
}

func testGetRefTreeEntry(t *testing.T, name string) {
	ref, err := testRepository.GetRef(name)
	if nil != err {
		t.Error(err)
	}
	if ref.Name() != name {
		t.Error()
	}

	entry, err := testRepository.GetTreeEntry(ref, nil, entryName)
	if nil != err {
		t.Error(err)
	}
	if entry.Name() != entryName {
		t.Error()
	}

	entry, err = testRepository.GetTreeEntry(ref, nil, entryName)
	if nil != err {
		t.Error(err)
	}
	if entry.Name() != entryName {
		t.Error()
	}

}

func TestGetRefTreeEntry(t *testing.T) {
	testGetRefTreeEntry(t, refName)
	testGetRefTreeEntry(t, tagName)
}

func testGetTree(t *testing.T, name string) {
	ref, err := testRepository.GetRef(name)
	if nil != err {
		t.Error(err)
	}
	if ref.Name() != name {
		t.Error()
	}

	entry, err := testRepository.GetTreeEntry(ref, nil, subtreeName)
	if nil != err {
		t.Error(err)
	}
	if entry.Name() != subtreeName {
		t.Error()
	}

	tree, err := testRepository.GetTree(nil, entry)
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

	tree, err = testRepository.GetTree(nil, entry)
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

func TestGetTree(t *testing.T) {
	testGetTree(t, refName)
	testGetTree(t, tagName)
}

func testGetTreeEntry(t *testing.T, name string) {
	ref, err := testRepository.GetRef(name)
	if nil != err {
		t.Error(err)
	}
	if ref.Name() != name {
		t.Error()
	}

	entry, err := testRepository.GetTreeEntry(ref, nil, subtreeName)
	if nil != err {
		t.Error(err)
	}
	if entry.Name() != subtreeName {
		t.Error()
	}

	subentry, err := testRepository.GetTreeEntry(nil, entry, subentryName)
	if nil != err {
		t.Error(err)
	}
	if subentry.Name() != subentryName {
		t.Error()
	}

	subentry, err = testRepository.GetTreeEntry(nil, entry, subentryName)
	if nil != err {
		t.Error(err)
	}
	if subentry.Name() != subentryName {
		t.Error()
	}
}

func TestGetTreeEntry(t *testing.T) {
	testGetTreeEntry(t, refName)
	testGetTreeEntry(t, tagName)
}

func TestGetBlobReader(t *testing.T) {
	ref, err := testRepository.GetRef(refName)
	if nil != err {
		t.Error(err)
	}
	if ref.Name() != refName {
		t.Error()
	}

	entry, err := testRepository.GetTreeEntry(ref, nil, subtreeName)
	if nil != err {
		t.Error(err)
	}
	if entry.Name() != subtreeName {
		t.Error()
	}

	subentry, err := testRepository.GetTreeEntry(nil, entry, subentryName)
	if nil != err {
		t.Error(err)
	}
	if subentry.Name() != subentryName {
		t.Error()
	}

	reader, err := testRepository.GetBlobReader(subentry)
	if nil != err {
		t.Error(err)
	}
	content, err := ioutil.ReadAll(reader.(io.Reader))
	reader.(io.Closer).Close()
	if !bytes.Contains(content, []byte("module github.com")) {
		t.Error()
	}

	reader, err = testRepository.GetBlobReader(subentry)
	if nil != err {
		t.Error(err)
	}
	content, err = ioutil.ReadAll(reader.(io.Reader))
	reader.(io.Closer).Close()
	if !bytes.Contains(content, []byte("module github.com")) {
		t.Error()
	}
}

func TestGetModule(t *testing.T) {
	const remote = "https://github.com/winfsp/winfsp"
	const refName = "master"
	const modulePath = "ext/test"
	const moduleTarget = "/billziss-gh/secfs.test"

	repository, err := NewGitRepository(remote, "", "", caseins, false)
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

		token, err := keyring.Get("hubfs", "github.com")
		if nil != err {
			token = ""
		}
		if "" == token {
			token = os.Getenv("HUBFS_TOKEN")
		}

		testRepository, err = NewGitRepository(remote, token, "x-oauth-basic", caseins, false)
		if nil != err {
			return err
		}

		tdir, err := ioutil.TempDir("", "git_test")
		if nil != err {
			return err
		}

		err = testRepository.SetDirectory(tdir)
		if nil != err {
			return err
		}

		atexit(func() {
			testRepository.RemoveDirectory()
			testRepository.Close()
		})

		return nil
	})
}
