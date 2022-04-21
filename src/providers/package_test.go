/*
 * test.go
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
	"fmt"
	"os"
	"testing"

	libtrace "github.com/billziss-gh/golib/trace"
)

var atinitFn []func() error
var atexitFn []func()

func atinit(fn func() error) {
	atinitFn = append(atinitFn, fn)
}

func atexit(fn func()) {
	atexitFn = append(atexitFn, fn)
}

func TestMain(m *testing.M) {
	libtrace.Verbose = true
	libtrace.Pattern = "github.com/winfsp/hubfs/*"

	for i := range atinitFn {
		err := atinitFn[i]()
		if nil != err {
			fmt.Printf("error: during init: %v\n", err)
			os.Exit(1)
		}
	}

	ec := m.Run()

	for i := range atexitFn {
		j := len(atexitFn) - 1 - i
		atexitFn[j]()
	}

	os.Exit(ec)
}
