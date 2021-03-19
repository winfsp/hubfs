/*
 * cache.go
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
	"time"

	"github.com/billziss-gh/golib/cache"
)

type cachedItem struct {
	cache.MapItem
	lastUsedTime time.Time
	inUse        int64
}

func (citem *cachedItem) touchCachedItem(ttl time.Duration, delta int) {
	citem.lastUsedTime = time.Now().Add(ttl)
	citem.inUse += int64(delta)
}

func (citem *cachedItem) expireCachedItem(list *cache.MapItem, ttl time.Duration, currentTime time.Time,
	fn func()) bool {
	if citem.lastUsedTime.After(currentTime) {
		return false
	}
	citem.lastUsedTime = currentTime.Add(ttl)
	citem.Remove()
	citem.InsertTail(list)
	if 0 >= citem.inUse {
		fn()
	}
	return true
}
