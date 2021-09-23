/*
 * pathkey_test.go
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

package union

import (
	"testing"
)

func TestPathkeyCompute(t *testing.T) {
	var k Pathkey
	var k0 = Pathkey{
		0x00, 0x8a, 0x5e, 0xda, 0xb2, 0x82, 0x63, 0x24,
		0x43, 0x21, 0x9e, 0x05, 0x1e, 0x4a, 0xde, 0x2d,
	}
	var k1 = Pathkey{
		0x00, 0x37, 0x9c, 0x9f, 0x23, 0x42, 0x5a, 0x38,
		0x69, 0x8d, 0x16, 0x4a, 0xbe, 0xb3, 0x39, 0x11,
	}

	k = ComputePathkey("/")
	if k0 != k {
		t.Error()
	}

	k = ComputePathkey("/path")
	if k1 != k {
		t.Error()
	}
}
