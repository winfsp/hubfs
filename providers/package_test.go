/*
 * test.go
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
	libtrace.Pattern = "github.com/billziss-gh/hubfs/*"

	for i := range atinitFn {
		err := atinitFn[i]()
		if nil != err {
			fmt.Printf("error: during init: %v\n", err)
		}
	}

	ec := m.Run()

	for i := range atexitFn {
		j := len(atexitFn) - 1 - i
		atexitFn[j]()
	}

	os.Exit(ec)
}
