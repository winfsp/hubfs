/*
 * git_test.go
 *
 * Copyright 2021 Bill Zissimopoulos
 */
/*
 * This file is part of Hubfs.
 *
 * It is licensed under the MIT license. The full license text can be found
 * in the License.txt file at the root of this project.
 */

package git

import (
	"fmt"
	"os"
	"testing"

	"github.com/billziss-gh/golib/keyring"
	libtrace "github.com/billziss-gh/golib/trace"
)

const repositoryName = "billziss-gh/hubfs"
const refName = "refs/heads/master"

const hash0 = "a06526668730e385e3eccecafce0840f3e63c1fb"
const hash1 = "9bcabcfe97184ee68a8bb98d556b6aa726c119f8"

var client *Client

func TestGetRefs(t *testing.T) {
	repository, err := client.OpenRepository(repositoryName)
	if nil != err {
		t.Error(err)
	}
	defer repository.Close()

	refs, err := repository.GetRefs()
	if nil != err {
		t.Error(err)
	}
	found := false
	for _, e := range refs {
		if e.Name == refName {
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
	for _, e := range refs {
		if e.Name == refName {
			found = true
			break
		}
	}
	if !found {
		t.Error()
	}
}

func TestFetchObjects(t *testing.T) {
	repository, err := client.OpenRepository(repositoryName)
	if nil != err {
		t.Error(err)
	}
	defer repository.Close()

	wants := []string{
		hash0,
		hash1,
	}
	found := false
	err = repository.FetchObjects(wants,
		func(hash string, typ Type, content []byte) error {
			if hash0 == hash || hash1 == hash {
				found = true
			}
			return nil
		})
	if nil != err {
		t.Error(err)
	}
	if !found {
		t.Error()
	}

	wants = []string{
		hash0,
	}
	found = false
	err = repository.FetchObjects(wants,
		func(hash string, typ Type, content []byte) error {
			if hash0 == hash {
				found = true
			}
			return nil
		})
	if nil != err {
		t.Error(err)
	}
	if !found {
		t.Error()
	}

	wants = []string{
		hash1,
	}
	found = false
	err = repository.FetchObjects(wants,
		func(hash string, typ Type, content []byte) error {
			if hash1 == hash {
				found = true
			}
			return nil
		})
	if nil != err {
		t.Error(err)
	}
	if !found {
		t.Error()
	}
}

func TestMain(m *testing.M) {
	libtrace.Verbose = true
	libtrace.Pattern = "github.com/billziss-gh/hubfs/*"

	token, err := keyring.Get("hubfs", "https://github.com")
	if nil == err {
		client, err = NewClient("https://github.com", token)
	}
	if nil != err {
		fmt.Fprintf(os.Stderr, "unable to create git client: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}
