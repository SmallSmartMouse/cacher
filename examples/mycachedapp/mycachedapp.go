package main

import (
	"fmt"
	"time"

	"github.com/SmallSmartMouse/cacher"
)

// Keys & values in cacher can be of arbitrary types, e.g. a struct.
type myStruct struct {
	text     string
	moreData []byte
}

func main() {
	// Accessing a new cache table for the first time will create it.
	cache := cacher.New("myCache", 5*time.Second)

	// We will put a new item in the cache. It will expire after
	// not being accessed via Get(key) for more than 5 seconds.
	val := myStruct{"This is a test!", []byte{}}
	cache.Set("someKey", 5*time.Second, &val)

	// Let's retrieve the item from the cache.
	res, err := cache.Get("someKey")
	if err == nil {
		fmt.Println("Found value in cache:", res.Data().(*myStruct).text)
	} else {
		fmt.Println("Error retrieving value from cache:", err)
	}

	// Wait for the item to expire in cache.
	time.Sleep(6 * time.Second)
	res, err = cache.Get("someKey")
	if err != nil {
		fmt.Println("Item is not cached (anymore).")
	}

	// Set another item that never expires.
	cache.Set("someKey", 0, &val)

	// cacher supports a few handy callbacks and loading mechanisms.
	cache.SetAboutToDeleteItemCallback(func(e *cacher.CacheItem) {
		fmt.Println("Deleting:", e.Key(), e.Data().(*myStruct).text, e.CreatedOn())
	})

	// Remove the item from the cache.
	cache.Delete("someKey")

	// And wipe the entire cache table.
	cache.Flush()
}
