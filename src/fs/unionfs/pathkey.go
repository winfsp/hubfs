/*
 * pathkey.go
 *
 * Copyright 2021 Bill Zissimopoulos
 */
/*
 * This file is part of Hubfs.
 *
 * You can redistribute it and/or modify it under the terms of the GNU
 * Affero General Public License version 3 as published by the Free
 * Software Foundation.
 */

package unionfs

import (
	"crypto/sha256"
	"hash"
	"strings"
)

const Pathkeylen = 16

type Pathkey [Pathkeylen]uint8

// Function ComputePathkey computes the path key for a path.
func ComputePathkey(path string, caseins bool) (k Pathkey) {
	if caseins {
		path = strings.ToUpper(path)
	}
	sum := sha256.Sum256([]uint8(path))
	copy(k[1:], sum[:])
	return
}

type PathkeyHash struct {
	hash.Hash
	caseins bool
}

func NewPathkeyHash(caseins bool) PathkeyHash {
	return PathkeyHash{sha256.New(), caseins}
}

func (h PathkeyHash) Write(s string) {
	if h.caseins {
		s = strings.ToUpper(s)
	}
	h.Hash.Write([]uint8(s))
}

func (h PathkeyHash) ComputePathkey() (k Pathkey) {
	copy(k[1:], h.Hash.Sum(nil))
	return
}
