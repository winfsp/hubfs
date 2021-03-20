/*
 * git.go
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
	"context"
	"io"
	"time"

	"github.com/billziss-gh/hubfs/httputil"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/packfile"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/sideband"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

type Repository struct {
	session transport.UploadPackSession
	advrefs *packp.AdvRefs
}

type Signature struct {
	Name  string
	Email string
	Time  time.Time
}

type Commit struct {
	Author    Signature
	Committer Signature
	TreeHash  string
}

type TreeEntry struct {
	Name string
	Mode uint32
	Hash string
}

func OpenRepository(remote string, token string) (res *Repository, err error) {
	endpoint, err := transport.NewEndpoint(remote)
	if nil != err {
		return nil, err
	}

	var auth transport.AuthMethod
	if "" != token {
		auth = &http.BasicAuth{
			Username: token,
			Password: "x-oauth-basic",
		}
	}

	client := http.NewClient(httputil.DefaultClient)
	session, err := client.NewUploadPackSession(endpoint, auth)
	if nil != err {
		return nil, err
	}

	advrefs, err := session.AdvertisedReferences()
	if nil != err {
		session.Close()
		return nil, err
	}

	return &Repository{
		session: session,
		advrefs: advrefs,
	}, nil
}

func (repository *Repository) Close() (err error) {
	return repository.session.Close()
}

func (repository *Repository) GetRefs() (res map[string]string, err error) {
	stg, err := repository.advrefs.AllReferences()
	if nil != err {
		return nil, err
	}

	res = make(map[string]string, len(stg))
	for n, r := range stg {
		res[string(n)] = r.Hash().String()
	}

	return res, nil
}

type storemap map[plumbing.Hash]plumbing.EncodedObject

func (m storemap) NewEncodedObject() plumbing.EncodedObject {
	return &plumbing.MemoryObject{}
}

func (m storemap) SetEncodedObject(obj plumbing.EncodedObject) (plumbing.Hash, error) {
	hash := obj.Hash()
	m[hash] = obj
	return hash, nil
}

func (m storemap) EncodedObject(typ plumbing.ObjectType, hash plumbing.Hash) (
	plumbing.EncodedObject, error) {
	obj, ok := m[hash]
	if !ok || (plumbing.AnyObject != typ && obj.Type() != typ) {
		return nil, plumbing.ErrObjectNotFound
	}

	return obj, nil
}

func (m storemap) IterEncodedObjects(typ plumbing.ObjectType) (storer.EncodedObjectIter, error) {
	lst := make([]plumbing.EncodedObject, 0, len(m))
	for _, obj := range m {
		if plumbing.AnyObject == typ || obj.Type() == typ {
			lst = append(lst, obj)
		}
	}
	return storer.NewEncodedObjectSliceIter(lst), nil
}

func (m storemap) HasEncodedObject(hash plumbing.Hash) error {
	_, ok := m[hash]
	if !ok {
		return plumbing.ErrObjectNotFound
	}
	return nil
}

func (m storemap) EncodedObjectSize(hash plumbing.Hash) (int64, error) {
	obj, ok := m[hash]
	if !ok {
		return 0, plumbing.ErrObjectNotFound
	}
	return obj.Size(), nil
}

type observer struct {
	fn func(hash string, content []byte) error
}

func (obs *observer) OnHeader(count uint32) error {
	return nil
}

func (obs *observer) OnInflatedObjectHeader(typ plumbing.ObjectType, objSize int64, pos int64) error {
	return nil
}

func (obs *observer) OnInflatedObjectContent(h plumbing.Hash, pos int64, crc uint32, content []byte) error {
	return obs.fn(h.String(), content)
}

func (obs *observer) OnFooter(h plumbing.Hash) error {
	return nil
}

func (repository *Repository) FetchObjects(wants []string,
	fn func(hash string, content []byte) error) (err error) {
	req := packp.NewUploadPackRequestFromCapabilities(repository.advrefs.Capabilities)

	if nil == req.Capabilities.Set("shallow") {
		req.Depth = packp.DepthCommits(1)
	}
	if repository.advrefs.Capabilities.Supports("no-progress") {
		req.Capabilities.Set("no-progress")
	}

	req.Wants = make([]plumbing.Hash, len(wants))
	for i, w := range wants {
		req.Wants[i] = plumbing.NewHash(w)
	}

	rsp, err := repository.session.UploadPack(context.Background(), req)
	if nil != err {
		return err
	}
	defer rsp.Close()

	var reader io.Reader
	switch {
	case req.Capabilities.Supports("side-band-64k"):
		reader = sideband.NewDemuxer(sideband.Sideband64k, rsp)
	case req.Capabilities.Supports("side-band"):
		reader = sideband.NewDemuxer(sideband.Sideband, rsp)
	default:
		reader = rsp
	}

	scn := packfile.NewScanner(reader)
	stg := storemap{}
	obs := &observer{fn: fn}
	parser, err := packfile.NewParserWithStorage(scn, stg, obs)
	if nil != err {
		return err
	}

	_, err = parser.Parse()
	if nil != err {
		return err
	}

	return nil
}

func DecodeCommit(content []byte) (res *Commit, err error) {
	obj := &plumbing.MemoryObject{}
	obj.SetType(plumbing.CommitObject)
	obj.Write(content)
	c := &object.Commit{}
	err = c.Decode(obj)
	if nil != err {
		return
	}
	res = &Commit{
		Author: Signature{
			Name:  c.Author.Name,
			Email: c.Author.Email,
			Time:  c.Author.When,
		},
		Committer: Signature{
			Name:  c.Committer.Name,
			Email: c.Committer.Email,
			Time:  c.Committer.When,
		},
		TreeHash: c.TreeHash.String(),
	}
	return
}

func DecodeTree(content []byte) (res []*TreeEntry, err error) {
	obj := &plumbing.MemoryObject{}
	obj.SetType(plumbing.TreeObject)
	obj.Write(content)
	t := &object.Tree{}
	err = t.Decode(obj)
	if nil != err {
		return
	}
	res = make([]*TreeEntry, len(t.Entries))
	for i, e := range t.Entries {
		res[i] = &TreeEntry{
			Name: e.Name,
			Mode: uint32(e.Mode),
			Hash: e.Hash.String(),
		}
	}
	return
}
