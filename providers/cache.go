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
	"sync"
	"time"

	libcache "github.com/billziss-gh/golib/cache"
)

type cache struct {
	Value   interface{}
	lock    sync.Locker
	lrulist libcache.MapItem
	ttl     time.Duration
	stopC   chan bool
	stopW   *sync.WaitGroup
}

type cacheItem struct {
	libcache.MapItem
	lastUsedTime time.Time
	inUse        int64
}

type expirable interface {
	expire(c *cache, currentTime time.Time) bool
}

func newCache(lock sync.Locker) *cache {
	c := &cache{}
	c.lock = lock
	c.lrulist.Empty()
	return c
}

func (c *cache) newCacheMap() *libcache.Map {
	return libcache.NewMap(&c.lrulist)
}

func (c *cache) touchCacheItem(citem *cacheItem, delta int) {
	citem.lastUsedTime = time.Now().Add(c.ttl)
	citem.inUse += int64(delta)
}

func (c *cache) expireCacheItem(citem *cacheItem, currentTime time.Time, fn func()) bool {
	if citem.lastUsedTime.After(currentTime) {
		return false
	}
	citem.lastUsedTime = currentTime.Add(c.ttl)
	citem.Remove()
	citem.InsertTail(&c.lrulist)
	if 0 >= citem.inUse {
		fn()
	}
	return true
}

func (c *cache) startExpiration(timeToLive time.Duration) {
	c.ttl = timeToLive
	c.stopC = make(chan bool, 1)
	c.stopW = &sync.WaitGroup{}
	c.stopW.Add(1)
	go c._tick()
}

func (c *cache) stopExpiration() {
	c.stopC <- true
	c.stopW.Wait()
	close(c.stopC)
	c.ttl = 0
	c.stopC = nil
	c.stopW = nil
}

func (c *cache) _tick() {
	defer c.stopW.Done()
	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-ticker.C:
			currentTime := time.Now()
			c.lock.Lock()
			c.lrulist.Expire(func(l, item *libcache.MapItem) bool {
				return item.Value.(expirable).expire(c, currentTime)
			})
			c.lock.Unlock()
		case <-c.stopC:
			ticker.Stop()
			return
		}
	}
}
