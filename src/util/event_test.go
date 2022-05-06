/*
 * event_test.go
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

package util

import (
	"fmt"
	"testing"
)

var testEventTotal1 int

func testEventHandler1(event interface{}) {
	testEventTotal1 += event.(int)
}

var testEventTotal2 int

func testEventHandler2(event interface{}) {
	testEventTotal2 -= event.(int)
}

func testEventPanic(event interface{}) {
	// should panic because event is of type int
	fmt.Println(event.(string))
}

func TestEvent(t *testing.T) {
	RegisterEventHandler("test1", testEventHandler1)
	RegisterEventHandler("test1", testEventPanic)
	RegisterEventHandler("test1", testEventHandler1)

	RegisterEventHandler("test2", testEventHandler2)
	RegisterEventHandler("test2", testEventPanic)
	RegisterEventHandler("test2", testEventHandler2)

	testEventTotal1 = 0
	InvokeEvent("test1", 21)
	if testEventTotal1 != 42 {
		t.Error()
	}

	testEventTotal2 = 0
	InvokeEvent("test2", 21)
	if testEventTotal2 != -42 {
		t.Error()
	}
}
