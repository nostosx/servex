package cache_test

import (
	"context"
	"fmt"
	"time"

	"github.com/nostosx/servex/storage/cache"
	"github.com/nostosx/servex/storage/cache/memory"
)

func ExampleNewLoader() {
	owner, stop := context.WithCancel(context.Background())
	defer stop()
	loader, err := cache.NewLoader[string](owner, memory.New(), 5*time.Second, cache.WithTTL(time.Minute))
	if err != nil {
		panic(err)
	}
	defer loader.Close()

	value, err := loader.Get(owner, "tenant-a:user:42", func(ctx context.Context) (string, error) {
		if err := context.Cause(ctx); err != nil {
			return "", err
		}
		return "alice", nil
	})
	fmt.Println(value, err)
	// Output: alice <nil>
}
