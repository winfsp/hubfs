/*
 * github_test.go
 *
 * Copyright 2021 Bill Zissimopoulos
 */
/*
 * This file is part of Hubfs.
 *
 * It is licensed under the MIT license. The full license text can be found
 * in the License.txt file at the root of this project.
 */

package providers

import (
	"fmt"
	"os"
	"testing"

	"github.com/billziss-gh/golib/keyring"
	libtrace "github.com/billziss-gh/golib/trace"
)

const ownerName = "billziss-gh"
const repositoryName = "winfsp"

var client Client

func TestGetOwner(t *testing.T) {
	owner, err := client.GetOwner(ownerName, false)
	if nil != err {
		t.Error(err)
	}
	if owner.Name() != ownerName {
		t.Error()
	}

	owner, err = client.GetOwner(ownerName, false)
	if nil != err {
		t.Error(err)
	}
	if owner.Name() != ownerName {
		t.Error()
	}
}

func TestGetRepositories(t *testing.T) {
	owner, err := client.GetOwner(ownerName, false)
	if nil != err {
		t.Error(err)
	}
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

func TestGetRepository(t *testing.T) {
	owner, err := client.GetOwner(ownerName, false)
	if nil != err {
		t.Error(err)
	}
	if owner.Name() != ownerName {
		t.Error()
	}

	repository, err := client.GetRepository(owner, repositoryName, false)
	if nil != err {
		t.Error(err)
	}
	if repository.Name() != repositoryName {
		t.Error()
	}

	repository, err = client.GetRepository(owner, repositoryName, false)
	if nil != err {
		t.Error(err)
	}
	if repository.Name() != repositoryName {
		t.Error()
	}
}

func TestMain(m *testing.M) {
	libtrace.Verbose = true
	libtrace.Pattern = "github.com/billziss-gh/hubfs/*"

	token, err := keyring.Get("hubfs", "https://github.com")
	if nil == err {
		client, err = GetProvider("https://github.com").NewClient(token)
	}
	if nil != err {
		fmt.Fprintf(os.Stderr, "unable to create GitHub client: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}
