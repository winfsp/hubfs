//go:build ent
// +build ent

/*
 * pvt_ent.go
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

package main

import (
	"github.com/winfsp/hubfs/pvt"
)

func init() {
	pvt.Load()
}
