package main

import (
	"fmt"
	"github.com/SmallSmartMouse/cacher"
	"strconv"
	"time"
)

func main() {
	cache := cacher.New("myCache", 5*time.Second)

	// The data loader gets called automatically whenever something
	// tries to retrieve a non-existing key from the cache.
	cache.SetDataLoader(func(key interface{}) (interface{}, time.Duration, error) {
		// Apply some clever loading logic here, e.g. read values for
		// this key from database, network or file.
		val := "This is a test with key " + key.(string)

		return val, 0, nil
	})

	// Let's retrieve a few auto-generated items from the cache.
	for i := 0; i < 10; i++ {
		res, err := cache.Get("someKey_" + strconv.Itoa(i))
		if err == nil {
			fmt.Println("Found value in cache:", res.Data())
		} else {
			fmt.Println("Error retrieving value from cache:", err)
		}
	}
}
