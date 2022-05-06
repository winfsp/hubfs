/*
 * event.go
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

import "sync"

var eventmux sync.RWMutex
var eventmap = make(map[string][]func(interface{}))

func RegisterEventHandler(name string, handler func(interface{})) {
	eventmux.Lock()
	defer eventmux.Unlock()
	eventmap[name] = append(eventmap[name], handler)
}

func InvokeEvent(name string, event interface{}) {
	eventmux.RLock()
	defer eventmux.RUnlock()
	for l, i := eventmap[name], 0; len(l) > i; {
		invokeEvent(l, &i, event)
	}
}

func invokeEvent(l []func(interface{}), i *int, event interface{}) {
	defer func() {
		recover()
	}()
	for len(l) > *i {
		j := *i
		(*i)++
		l[j](event)
	}
}
