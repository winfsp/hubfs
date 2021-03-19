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
	"path"

	libtrace "github.com/billziss-gh/golib/trace"
	"github.com/billziss-gh/hubfs/httputil"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/packfile"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/sideband"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
)

type Type int

const (
	Commit Type = 1
	Tree   Type = 2
	Blob   Type = 3
	Tag    Type = 4
)

type Client struct {
	transport transport.Transport
	endpoint  *transport.Endpoint
	token     string
}

type Repository struct {
	session transport.UploadPackSession
	advrefs *packp.AdvRefs
}

type Ref struct {
	Name string
	Hash string
}

func (client *Client) OpenRepository(repo string) (res *Repository, err error) {
	defer trace(repo)(&err)

	endpoint := *client.endpoint
	endpoint.Path = path.Join(endpoint.Path, repo)

	auth := &githttp.BasicAuth{
		Username: client.token,
		Password: "x-oauth-basic",
	}

	session, err := client.transport.NewUploadPackSession(&endpoint, auth)
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
	defer trace()(&err)

	return repository.session.Close()
}

func (repository *Repository) GetRefs() (res []*Ref, err error) {
	defer trace()(&err)

	stg, err := repository.advrefs.AllReferences()
	if nil != err {
		return nil, err
	}

	res = make([]*Ref, len(stg))
	i := 0
	for n, r := range stg {
		ref := &Ref{
			Name: string(n),
			Hash: r.Hash().String(),
		}
		res[i] = ref
		i++
	}

	return res, nil
}

type observer struct {
	fn  func(hash string, typ Type, content []byte) error
	typ plumbing.ObjectType
}

func (obs *observer) OnHeader(count uint32) error {
	return nil
}
func (obs *observer) OnInflatedObjectHeader(typ plumbing.ObjectType, objSize int64, pos int64) error {
	obs.typ = typ
	return nil
}
func (obs *observer) OnInflatedObjectContent(h plumbing.Hash, pos int64, crc uint32, content []byte) error {
	switch obs.typ {
	case plumbing.CommitObject, plumbing.TreeObject, plumbing.BlobObject, plumbing.TagObject:
		return obs.fn(h.String(), Type(obs.typ), content)
	default:
		return nil
	}
}
func (obs *observer) OnFooter(h plumbing.Hash) error {
	return nil
}

func (repository *Repository) FetchObjects(wants []string,
	fn func(hash string, typ Type, content []byte) error) (err error) {
	defer trace(len(wants))(&err)

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
	stg := memory.NewStorage()
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

func NewClient(remote string, token string) (*Client, error) {
	ept, err := transport.NewEndpoint(remote)
	if nil != err {
		return nil, err
	}

	return &Client{
		transport: githttp.NewClient(httputil.DefaultClient),
		endpoint:  ept,
		token:     token,
	}, nil
}

func trace(vals ...interface{}) func(vals ...interface{}) {
	return libtrace.Trace(1, "", vals...)
}
