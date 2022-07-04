/*
 * Simple caching library with expiration capabilities
 *     Copyright (c) 2012, Radu Ioan Fericean
 *
 *   For license see LICENSE.txt
 */

package cacher

import (
	"runtime"
	"sync"
	"time"
)

var (
	cache = make(map[string]*CacheTable)
	mutex sync.RWMutex
)

// New Return a new cache with a given default expiration duration and cleanup
// interval. If the expiration duration is less than one (or NoExpiration),
// the items in the cache never expire (by default), and must be deleted
func New(table string, cleanupInterval time.Duration) *CacheTable {
	mutex.RLock()
	t, ok := cache[table]
	mutex.RUnlock()

	if !ok {
		mutex.Lock()
		t, ok = cache[table]
		// Double check whether the table exists or not.
		if !ok {
			t = &CacheTable{
				name:              table,
				cleanupInterval:   cleanupInterval,
				defaultExpiration: time.Millisecond,
				//defaultExpiration:defaultExpiration, TODO
				items: make(map[interface{}]*CacheItem),
			}
			runJanitor(t, cleanupInterval)
			runtime.SetFinalizer(t, stopJanitor)

			cache[table] = t
		}
		mutex.Unlock()
	}
	return t

}
