/*
 * Simple caching library with expiration capabilities
 *
 *   For license see LICENSE.txt
 */

package cacher

import (
	"github.com/SmallSmartMouse/cacher/singleflight"
	"log"
	"sort"
	"sync"
	"time"
)

// CacheTable is a table within the cache
type CacheTable struct {
	sync.RWMutex
	singleSetCache singleflight.Group
	// The table's name.
	name string
	// All cached items.
	items map[interface{}]*CacheItem

	// Current timer duration.
	cleanupInterval time.Duration
	// default expire duration.
	defaultExpiration time.Duration
	// The logger used for this table.
	logger *log.Logger
	// true cache empty data
	enableNullData bool
	enableAutoLoad bool
	janitor        *janitor
	// Callback method triggered when trying to load a non-existing key.
	loadData func(k interface{}) (interface{}, time.Duration, error)
	// Callback method triggered when adding a new item to the cache.
	addedItem []func(item *CacheItem)
	// Callback method triggered before deleting an item from the cache.
	aboutToDeleteItem []func(item *CacheItem)
}

// Count returns how many items are currently stored in the cache.
func (table *CacheTable) Count() int {
	table.RLock()
	defer table.RUnlock()
	return len(table.items)
}

// Foreach all items
func (table *CacheTable) Foreach(trans func(key interface{}, item *CacheItem)) {
	table.RLock()
	defer table.RUnlock()

	for k, v := range table.items {
		trans(k, v)
	}
}

func (table *CacheTable) EnableNullData(b bool) {
	table.Lock()
	defer table.Unlock()
	table.enableNullData = b
}

// SetDataLoader configures a data-loader callback, which will be called when
// trying to access a non-existing key. The key and 0...n additional arguments
// are passed to the callback function.
func (table *CacheTable) SetDataLoader(f func(k interface{}) (interface{}, time.Duration, error)) {
	table.Lock()
	defer table.Unlock()
	table.enableAutoLoad = true
	table.loadData = f
}

// SetAddedItemCallback configures a callback, which will be called every time
// a new item is added to the cache.
func (table *CacheTable) SetAddedItemCallback(f func(*CacheItem)) {
	if len(table.addedItem) > 0 {
		table.RemoveAddedItemCallbacks()
	}
	table.Lock()
	defer table.Unlock()
	table.addedItem = append(table.addedItem, f)
}

//AddAddedItemCallback appends a new callback to the addedItem queue
func (table *CacheTable) AddAddedItemCallback(f func(*CacheItem)) {
	table.Lock()
	defer table.Unlock()
	table.addedItem = append(table.addedItem, f)
}

// RemoveAddedItemCallbacks empties the added item callback queue
func (table *CacheTable) RemoveAddedItemCallbacks() {
	table.Lock()
	defer table.Unlock()
	table.addedItem = nil
}

// SetAboutToDeleteItemCallback configures a callback, which will be called
// every time an item is about to be removed from the cache.
func (table *CacheTable) SetAboutToDeleteItemCallback(f func(*CacheItem)) {
	if len(table.aboutToDeleteItem) > 0 {
		table.RemoveAboutToDeleteItemCallback()
	}
	table.Lock()
	defer table.Unlock()
	table.aboutToDeleteItem = append(table.aboutToDeleteItem, f)
}

// AddAboutToDeleteItemCallback appends a new callback to the AboutToDeleteItem queue
func (table *CacheTable) AddAboutToDeleteItemCallback(f func(*CacheItem)) {
	table.Lock()
	defer table.Unlock()
	table.aboutToDeleteItem = append(table.aboutToDeleteItem, f)
}

// RemoveAboutToDeleteItemCallback empties the about to delete item callback queue
func (table *CacheTable) RemoveAboutToDeleteItemCallback() {
	table.Lock()
	defer table.Unlock()
	table.aboutToDeleteItem = nil
}

// SetLogger sets the logger to be used by this cache table.
func (table *CacheTable) SetLogger(logger *log.Logger) {
	table.Lock()
	defer table.Unlock()
	table.logger = logger
}

// ExpirationCheck check loop
func (table *CacheTable) ExpirationCheck() {
	table.Lock()

	if table.cleanupInterval > 0 {
		table.log("Expiration check triggered after", table.cleanupInterval, "for table", table.name)
	} else {
		table.log("Expiration check installed for table", table.name)
	}

	// To be more accurate with timers, we would need to update 'now' on every
	// loop iteration. Not sure it's really efficient though.
	now := time.Now()
	for key, item := range table.items {
		// Cache values so we don't keep blocking the mutex.
		item.RLock()
		lifeSpan := item.lifeSpan
		accessedOn := item.accessedOn
		createdOn := item.createdOn
		item.RUnlock()
		// lasting key
		if lifeSpan == 0 {
			continue
		}

		if now.Sub(createdOn) >= lifeSpan {
			if table.enableAutoLoad {
				if now.Sub(accessedOn) <= lifeSpan*2/3 {
					temp, tempLifeSpan, err1 := table.loadData(key)
					if err1 == nil {
						table.addInternal(NewCacheItem(key, tempLifeSpan, temp))
						continue
					}
				}
			}
			table.deleteInternal(key)
		}
	}
	table.Unlock()
}

func (table *CacheTable) addInternal(item *CacheItem) {
	// Careful: do not run this method unless the table-mutex is locked!
	// It will unlock it for the caller before running the callbacks and checks
	table.log("Adding item with key", item.key, "and lifespan of", item.lifeSpan, "to table", table.name)
	table.items[item.key] = item

	// Cache values so we don't keep blocking the mutex.
	addedItem := table.addedItem
	// Trigger callback after adding an item to cache.
	if addedItem != nil {
		for _, callback := range addedItem {
			callback(item)
		}
	}
}

// Set adds a key/value pair to the cache.
// Parameter key is the item's cache-key.
// Parameter lifeSpan determines after which time period without an access the item
// will get removed from the cache.
// Parameter data is the item's value.
func (table *CacheTable) Set(key interface{}, lifeSpan time.Duration, data interface{}) *CacheItem {
	item := NewCacheItem(key, lifeSpan, data)

	// Set item to cache.
	table.Lock()
	table.addInternal(item)
	table.Unlock()

	return item
}

func (table *CacheTable) deleteInternal(key interface{}) (*CacheItem, error) {
	r, ok := table.items[key]
	if !ok {
		return nil, ErrKeyNotFound
	}

	// Cache value so we don't keep blocking the mutex.
	aboutToDeleteItem := table.aboutToDeleteItem
	table.Unlock()

	// Trigger callbacks before deleting an item from cache.
	if aboutToDeleteItem != nil {
		for _, callback := range aboutToDeleteItem {
			callback(r)
		}
	}

	r.RLock()
	defer r.RUnlock()
	if r.aboutToExpire != nil {
		for _, callback := range r.aboutToExpire {
			callback(key)
		}
	}

	table.Lock()
	table.log("Deleting item with key", key, "created on", r.createdOn, "and hit", r.accessCount, "times from table", table.name)
	delete(table.items, key)

	return r, nil
}

// Delete an item from the cache.
func (table *CacheTable) Delete(key interface{}) (*CacheItem, error) {
	table.Lock()
	defer table.Unlock()

	return table.deleteInternal(key)
}

// Exists returns whether an item exists in the cache. Unlike the Get method
// Exists neither tries to fetch data via the loadData callback nor does it
// keep the item alive in the cache.
func (table *CacheTable) Exists(key interface{}) bool {
	table.RLock()
	defer table.RUnlock()
	_, ok := table.items[key]

	return ok
}

// Add checks whether an item is not yet cached. Unlike the Exists
// method this also adds data if the key could not be found.
func (table *CacheTable) Add(key interface{}, lifeSpan time.Duration, data interface{}) bool {
	table.Lock()
	if _, ok := table.items[key]; ok {
		table.Unlock()
		return false
	}

	item := NewCacheItem(key, lifeSpan, data)
	table.addInternal(item)
	table.Unlock()
	return true
}

// Get returns an item from the cache and marks it to be kept alive. You can
// pass additional arguments to your DataLoader callback function.
func (table *CacheTable) Get(key interface{}) (*CacheItem, error) {
	table.RLock()
	r, ok := table.items[key]
	loadData := table.loadData
	table.RUnlock()

	if ok {
		// Update access counter and timestamp.
		r.KeepAlive()
		return r, nil
	}

	// Item doesn't exist in cache. Try and fetch it with a data-loader.
	if loadData != nil {
		data, err, _ := table.singleSetCache.Do(key, func() (interface{}, error) {
			temp, tempLifeSpan, err1 := loadData(key)
			if err1 != nil && !table.enableNullData {
				return nil, err1
			}

			item := NewCacheItem(key, tempLifeSpan, temp)
			table.Lock()
			table.addInternal(item)
			table.Unlock()
			return item, nil
		})

		if err != nil {
			return nil, err
		}
		return data.(*CacheItem), nil
	}
	return nil, ErrKeyNotFound
}

// Flush deletes all items from this cache table.
func (table *CacheTable) Flush() {
	table.Lock()
	defer table.Unlock()

	table.log("Flushing table", table.name)

	table.items = make(map[interface{}]*CacheItem)
	table.cleanupInterval = 0
}

// CacheItemPair maps key to access counter
type CacheItemPair struct {
	Key         interface{}
	AccessCount int64
}

// CacheItemPairList is a slice of CacheItemPairs that implements sort.
// Interface to sort by AccessCount.
type CacheItemPairList []CacheItemPair

func (p CacheItemPairList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p CacheItemPairList) Len() int           { return len(p) }
func (p CacheItemPairList) Less(i, j int) bool { return p[i].AccessCount > p[j].AccessCount }

// MostAccessed returns the most accessed items in this cache table
func (table *CacheTable) MostAccessed(count int64) []*CacheItem {
	table.RLock()
	defer table.RUnlock()

	p := make(CacheItemPairList, len(table.items))
	i := 0
	for k, v := range table.items {
		p[i] = CacheItemPair{k, v.accessCount}
		i++
	}
	sort.Sort(p)

	var r []*CacheItem
	c := int64(0)
	for _, v := range p {
		if c >= count {
			break
		}

		item, ok := table.items[v.Key]
		if ok {
			r = append(r, item)
		}
		c++
	}

	return r
}

// Internal logging method for convenience.
func (table *CacheTable) log(v ...interface{}) {
	if table.logger == nil {
		return
	}

	table.logger.Println(v...)
}

type janitor struct {
	Interval time.Duration
	stop     chan bool
}

func (j *janitor) Run(c *CacheTable) {
	ticker := time.NewTicker(j.Interval)
	for {
		select {
		case <-ticker.C:
			c.ExpirationCheck()
		case <-j.stop:
			ticker.Stop()
			return
		}
	}
}

func stopJanitor(c *CacheTable) {
	c.janitor.stop <- true
}

func runJanitor(c *CacheTable, ci time.Duration) {
	j := &janitor{
		Interval: ci,
		stop:     make(chan bool),
	}
	c.janitor = j
	go j.Run(c)
}
