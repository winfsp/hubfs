/*
 * hubfs_test.go
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

package hubfs

import (
	"reflect"
	"testing"
	"unsafe"
)

// See https://stackoverflow.com/q/42664837/568557
func testGetUnexportedField(field reflect.Value) reflect.Value {
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
}

func TestNewOverlay(t *testing.T) {
	P := []string{"", "/1", "/1/2", "/1/2/3"}
	Q := []string{"/", "/a", "/a/b", "/a/b/c", "/a/b/c/d"}
	E := []struct{ prefix, remain string }{
		{"", "/"},
		{"", "/a"},
		{"", "/a/b"},
		{"/a/b/c", "/"},
		{"/a/b/c", "/d"},
		{"", "/"},
		{"", "/a"},
		{"/a/b", "/"},
		{"/a/b", "/c"},
		{"/a/b", "/c/d"},
		{"", "/"},
		{"/a", "/"},
		{"/a", "/b"},
		{"/a", "/b/c"},
		{"/a", "/b/c/d"},
		{"/", "/"},
		{"/", "/a"},
		{"/", "/a/b"},
		{"/", "/a/b/c"},
		{"/", "/a/b/c/d"},
	}
	i := 0
	for _, p := range P {
		fs := newOverlay(Config{Prefix: p})
		split := testGetUnexportedField(reflect.ValueOf(fs).Elem().FieldByName("split"))
		for _, q := range Q {
			a := make([]reflect.Value, 1)
			a[0] = reflect.ValueOf(q)
			r := split.Call(a)
			prefix, remain := r[0].String(), r[1].String()
			if prefix != E[i].prefix || remain != E[i].remain {
				t.Error()
			}
			i++
		}
	}
}
