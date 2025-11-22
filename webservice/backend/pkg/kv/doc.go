// Package kv provides a Redis-like key-value store abstraction with in-memory
// and Redis-backed implementations.
//
// The package defines a Store interface that supports common Redis operations
// including strings, hashes, sets, lists, and counters with TTL support.
//
// Example usage:
//
//	cfg := Config{
//		Backend: "memory",
//		JanitorInterval: 30 * time.Second,
//	}
//	store, err := NewStoreFromConfig(cfg)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer store.Close()
//
//	ctx := context.Background()
//	err = store.Set(ctx, "key", []byte("value"), 10*time.Second)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	value, err := store.Get(ctx, "key")
//	if err != nil {
//		if errors.Is(err, ErrNotFound) {
//			log.Println("Key not found")
//		} else {
//			log.Fatal(err)
//		}
//	}
//
// The in-memory implementation provides a first-class development and testing
// experience with full TTL support and background expiration. The Redis adapter
// wraps go-redis/v8 for production use while maintaining the same interface.
package kv