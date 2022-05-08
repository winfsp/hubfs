/*
 * github_test.go
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
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/billziss-gh/golib/keyring"
)

const ownerName = "winfsp"
const repositoryName = "hubfs"

var testClient Client

func TestOpenCloseOwner(t *testing.T) {
	owner, err := testClient.OpenOwner(ownerName)
	if nil != err {
		t.Error(err)
	}
	if owner.Name() != ownerName {
		t.Error()
	}
	testClient.CloseOwner(owner)

	owner, err = testClient.OpenOwner(ownerName)
	if nil != err {
		t.Error(err)
	}
	if owner.Name() != ownerName {
		t.Error()
	}
	testClient.CloseOwner(owner)
}

func TestGetRepositories(t *testing.T) {
	owner, err := testClient.OpenOwner(ownerName)
	if nil != err {
		t.Error(err)
	}
	defer testClient.CloseOwner(owner)
	if owner.Name() != ownerName {
		t.Error()
	}

	repositories, err := testClient.GetRepositories(owner)
	if nil != err {
		t.Error(err)
	}
	found := false
	for _, e := range repositories {
		if e.Name() == repositoryName {
			found = true
			break
		}
	}
	if !found {
		t.Error()
	}

	repositories, err = testClient.GetRepositories(owner)
	if nil != err {
		t.Error(err)
	}
	found = false
	for _, e := range repositories {
		if e.Name() == repositoryName {
			found = true
			break
		}
	}
	if !found {
		t.Error()
	}
}

func TestOpenCloseRepository(t *testing.T) {
	owner, err := testClient.OpenOwner(ownerName)
	if nil != err {
		t.Error(err)
	}
	defer testClient.CloseOwner(owner)
	if owner.Name() != ownerName {
		t.Error()
	}

	repository, err := testClient.OpenRepository(owner, repositoryName)
	if nil != err {
		t.Error(err)
	}
	if repository.Name() != repositoryName {
		t.Error()
	}
	testClient.CloseRepository(repository)

	repository, err = testClient.OpenRepository(owner, repositoryName)
	if nil != err {
		t.Error(err)
	}
	if repository.Name() != repositoryName {
		t.Error()
	}
	testClient.CloseRepository(repository)
}

func testExpiration(t *testing.T) {
	testClient.StartExpiration()
	defer testClient.StopExpiration()

	owner, err := testClient.OpenOwner(ownerName)
	if nil != err {
		t.Error(err)
	}
	if owner.Name() != ownerName {
		t.Error()
	}

	repository, err := testClient.OpenRepository(owner, repositoryName)
	if nil != err {
		t.Error(err)
	}
	if repository.Name() != repositoryName {
		t.Error()
	}

	testClient.CloseRepository(repository)
	testClient.CloseOwner(owner)

	time.Sleep(3 * time.Second)

	owner, err = testClient.OpenOwner(ownerName)
	if nil != err {
		t.Error(err)
	}
	if owner.Name() != ownerName {
		t.Error()
	}

	repository, err = testClient.OpenRepository(owner, repositoryName)
	if nil != err {
		t.Error(err)
	}
	if repository.Name() != repositoryName {
		t.Error()
	}

	testClient.CloseRepository(repository)
	testClient.CloseOwner(owner)
}

func TestExpiration(t *testing.T) {
	testExpiration(t)
	testExpiration(t)
}

func init() {
	atinit(func() error {
		token, err := keyring.Get("hubfs", "github.com")
		if nil != err {
			token = ""
		}
		if "" == token {
			token = os.Getenv("HUBFS_TOKEN")
		}

		uri, _ := url.Parse("https://github.com")
		testClient, err = NewProviderInstance(uri).NewClient(token)
		if nil != err {
			return err
		}

		testClient.SetConfig([]string{"config.ttl=1s"})

		return nil
	})
}
