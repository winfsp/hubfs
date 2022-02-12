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

package providers

import (
	"os"
	"testing"
	"time"

	"github.com/billziss-gh/golib/keyring"
)

const ownerName = "billziss-gh"
const repositoryName = "hubfs"

var client Client

func TestOpenCloseOwner(t *testing.T) {
	owner, err := client.OpenOwner(ownerName)
	if nil != err {
		t.Error(err)
	}
	if owner.Name() != ownerName {
		t.Error()
	}
	client.CloseOwner(owner)

	owner, err = client.OpenOwner(ownerName)
	if nil != err {
		t.Error(err)
	}
	if owner.Name() != ownerName {
		t.Error()
	}
	client.CloseOwner(owner)
}

func TestGetRepositories(t *testing.T) {
	owner, err := client.OpenOwner(ownerName)
	if nil != err {
		t.Error(err)
	}
	defer client.CloseOwner(owner)
	if owner.Name() != ownerName {
		t.Error()
	}

	repositories, err := client.GetRepositories(owner)
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

	repositories, err = client.GetRepositories(owner)
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
	owner, err := client.OpenOwner(ownerName)
	if nil != err {
		t.Error(err)
	}
	defer client.CloseOwner(owner)
	if owner.Name() != ownerName {
		t.Error()
	}

	repository, err := client.OpenRepository(owner, repositoryName)
	if nil != err {
		t.Error(err)
	}
	if repository.Name() != repositoryName {
		t.Error()
	}
	client.CloseRepository(repository)

	repository, err = client.OpenRepository(owner, repositoryName)
	if nil != err {
		t.Error(err)
	}
	if repository.Name() != repositoryName {
		t.Error()
	}
	client.CloseRepository(repository)
}

func testExpiration(t *testing.T) {
	client.StartExpiration()
	defer client.StopExpiration()

	owner, err := client.OpenOwner(ownerName)
	if nil != err {
		t.Error(err)
	}
	if owner.Name() != ownerName {
		t.Error()
	}

	repository, err := client.OpenRepository(owner, repositoryName)
	if nil != err {
		t.Error(err)
	}
	if repository.Name() != repositoryName {
		t.Error()
	}

	client.CloseRepository(repository)
	client.CloseOwner(owner)

	time.Sleep(3 * time.Second)

	owner, err = client.OpenOwner(ownerName)
	if nil != err {
		t.Error(err)
	}
	if owner.Name() != ownerName {
		t.Error()
	}

	repository, err = client.OpenRepository(owner, repositoryName)
	if nil != err {
		t.Error(err)
	}
	if repository.Name() != repositoryName {
		t.Error()
	}

	client.CloseRepository(repository)
	client.CloseOwner(owner)
}

func TestExpiration(t *testing.T) {
	testExpiration(t)
	testExpiration(t)
}

func init() {
	atinit(func() error {
		token, err := keyring.Get("hubfs", "https://github.com")
		if nil != err {
			token = ""
		}
		if "" == token {
			token = os.Getenv("GITHUB_TOKEN")
		}

		client, err = GetProvider("https://github.com").NewClient(token)
		if nil != err {
			return err
		}

		client.SetConfig([]string{"config.ttl=1s"})

		return nil
	})
}
