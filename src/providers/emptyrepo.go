/*
 * emptyrepo.go
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
	"io"
)

// When using:
//
//     var emptyRepository Repository = &emptyRepositoryT{}
//
// The emptyRepository variable is not initialized (at least during testing).
// May be related to https://github.com/golang/go/issues/44956
var emptyRepository Repository

type emptyRepositoryT struct {
}

func (*emptyRepositoryT) Close() (err error) {
	return nil
}

func (*emptyRepositoryT) GetDirectory() string {
	return ""
}

func (*emptyRepositoryT) SetDirectory(path string) error {
	return nil
}

func (*emptyRepositoryT) RemoveDirectory() error {
	return nil
}

func (*emptyRepositoryT) Name() string {
	return ""
}

func (*emptyRepositoryT) GetRefs() ([]Ref, error) {
	return []Ref{}, nil
}

func (*emptyRepositoryT) GetRef(name string) (Ref, error) {
	return nil, ErrNotFound
}

func (*emptyRepositoryT) GetTempRef(name string) (Ref, error) {
	return nil, ErrNotFound
}

func (*emptyRepositoryT) GetTree(ref Ref, entry TreeEntry) ([]TreeEntry, error) {
	return []TreeEntry{}, nil
}

func (*emptyRepositoryT) GetTreeEntry(ref Ref, entry TreeEntry, name string) (TreeEntry, error) {
	return nil, ErrNotFound
}

func (*emptyRepositoryT) GetBlobReader(entry TreeEntry) (io.ReaderAt, error) {
	return nil, ErrNotFound
}

func (*emptyRepositoryT) GetModule(ref Ref, path string, rootrel bool) (string, error) {
	return "", ErrNotFound
}

func init() {
	emptyRepository = &emptyRepositoryT{}
}
