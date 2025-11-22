package kv_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/leafsii/leafsii-backend/pkg/kv"

	// Import backends to register them
	_ "github.com/leafsii/leafsii-backend/pkg/kv/memory"
	_ "github.com/leafsii/leafsii-backend/pkg/kv/redis"
)

func ExampleNewStoreFromConfig_gracefulFailover() {
	// Create a logger to see failover events
	logger := func(msg string, fields ...any) {
		fmt.Printf("KV Store: %s\n", msg)
	}

	cfg := kv.Config{
		Backend:             kv.BackendRedis,
		RedisURL:            "redis://localhost:6379/0", // This will likely fail
		FailoverEnabled:     true,
		ProbeInterval:       2 * time.Second,
		StartupProbeTimeout: 500 * time.Millisecond,
		Logger:              logger,
	}

	// This should gracefully fall back to memory store if Redis is unavailable
	store, err := kv.NewStoreFromConfig(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()

	// Use the store normally - it will work regardless of Redis availability
	err = store.Set(ctx, "user:123", []byte("john"))
	if err != nil {
		log.Fatal(err)
	}

	value, err := store.Get(ctx, "user:123")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Retrieved value: %s\n", string(value))

	// Check which backend is active
	if fs, ok := store.(interface{ GetActiveBackend() string }); ok {
		fmt.Printf("Active backend: %s\n", fs.GetActiveBackend())
	}

	// Output will vary based on Redis availability:
	// If Redis is down: "KV Store: Redis unavailable at startup; using in-memory store"
	// Retrieved value: john
	// Active backend: fallback
}

func ExampleFailoverStore_transparentFailover() {
	// This example shows how failover works transparently
	// Note: This would typically be used internally by the factory

	// For demonstration, we'll create a config that fails gracefully
	cfg := kv.Config{
		Backend:         kv.BackendRedis,
		RedisURL:        "redis://nonexistent:6379/0", // Intentionally wrong
		FailoverEnabled: true,
		Logger: func(msg string, fields ...any) {
			fmt.Printf("[%s] %s\n", time.Now().Format("15:04:05"), msg)
		},
	}

	store, err := kv.NewStoreFromConfig(cfg)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer store.Close()

	ctx := context.Background()

	// Store operations continue to work normally
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("test:%d", i)
		value := fmt.Sprintf("value-%d", i)

		err := store.Set(ctx, key, []byte(value))
		if err != nil {
			fmt.Printf("Set failed: %v\n", err)
			continue
		}

		result, err := store.Get(ctx, key)
		if err != nil {
			fmt.Printf("Get failed: %v\n", err)
			continue
		}

		fmt.Printf("Stored and retrieved: %s = %s\n", key, string(result))
	}

	// This will show that the application continues to work
	// even when the primary backend (Redis) is unavailable
}
