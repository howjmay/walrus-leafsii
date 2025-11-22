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

func ExampleNewStoreFromConfig_memory() {
	cfg := kv.Config{
		Backend:         kv.BackendMemory,
		JanitorInterval: 30 * time.Second,
	}
	
	store, err := kv.NewStoreFromConfig(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	
	ctx := context.Background()
	
	// Basic string operations
	err = store.Set(ctx, "user:123", []byte("john"))
	if err != nil {
		log.Fatal(err)
	}
	
	value, err := store.Get(ctx, "user:123")
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Println(string(value))
	// Output: john
}

func ExampleNewStoreFromConfig_redis() {
	cfg := kv.Config{
		Backend:  kv.BackendRedis,
		RedisURL: "redis://localhost:6379/0",
	}
	
	store, err := kv.NewStoreFromConfig(cfg)
	if err != nil {
		// Handle error (Redis might not be available)
		fmt.Println("Redis not available, using memory store instead")
		
		cfg.Backend = kv.BackendMemory
		store, err = kv.NewStoreFromConfig(cfg)
		if err != nil {
			log.Fatal(err)
		}
	}
	defer store.Close()
	
	ctx := context.Background()
	
	// Set with TTL
	err = store.Set(ctx, "session:abc", []byte("active"), 5*time.Minute)
	if err != nil {
		log.Fatal(err)
	}
	
	// Check if key exists
	exists, err := store.Exists(ctx, "session:abc")
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("Session exists: %t\n", exists > 0)
}

func ExampleStore_hash() {
	cfg := kv.Config{
		Backend: kv.BackendMemory,
	}
	
	store, err := kv.NewStoreFromConfig(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	
	ctx := context.Background()
	
	// Hash operations
	userKey := "user:profile:123"
	
	// Set hash fields
	store.HSet(ctx, userKey, "name", []byte("John Doe"))
	store.HSet(ctx, userKey, "email", []byte("john@example.com"))
	store.HSet(ctx, userKey, "age", []byte("30"))
	
	// Get single field
	name, err := store.HGet(ctx, userKey, "name")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Name:", string(name))
	
	// Get all fields
	profile, err := store.HGetAll(ctx, userKey)
	if err != nil {
		log.Fatal(err)
	}
	
	for field, value := range profile {
		fmt.Printf("%s: %s\n", field, string(value))
	}
}

func ExampleStore_counter() {
	cfg := kv.Config{
		Backend: kv.BackendMemory,
	}
	
	store, err := kv.NewStoreFromConfig(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	
	ctx := context.Background()
	
	// Counter operations
	counterKey := "page:views"
	
	// Increment page views
	views, err := store.IncrBy(ctx, counterKey, 1)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Page views: %d\n", views)
	
	// Increment by 5
	views, err = store.IncrBy(ctx, counterKey, 5)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Page views after +5: %d\n", views)
}

func ExampleStore_set() {
	cfg := kv.Config{
		Backend: kv.BackendMemory,
	}
	
	store, err := kv.NewStoreFromConfig(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	
	ctx := context.Background()
	
	// Set operations
	setKey := "tags:article:123"
	
	// Add tags
	added, err := store.SAdd(ctx, setKey, []byte("go"), []byte("redis"), []byte("tutorial"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Added %d tags\n", added)
	
	// Check if tag exists
	isMember, err := store.SIsMember(ctx, setKey, []byte("go"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("'go' tag exists: %t\n", isMember)
	
	// Get all tags
	tags, err := store.SMembers(ctx, setKey)
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Print("All tags: ")
	for i, tag := range tags {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Print(string(tag))
	}
	fmt.Println()
}

func ExampleStore_list() {
	cfg := kv.Config{
		Backend: kv.BackendMemory,
	}
	
	store, err := kv.NewStoreFromConfig(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	
	ctx := context.Background()
	
	// List operations (queue/stack)
	queueKey := "jobs:queue"
	
	// Add jobs to queue (right push)
	length, err := store.RPush(ctx, queueKey, []byte("job1"), []byte("job2"), []byte("job3"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Queue length: %d\n", length)
	
	// Process jobs from queue (left pop - FIFO)
	for i := 0; i < 2; i++ {
		job, err := store.LPop(ctx, queueKey)
		if err != nil {
			if err == kv.ErrNotFound {
				fmt.Println("Queue is empty")
				break
			}
			log.Fatal(err)
		}
		fmt.Printf("Processing job: %s\n", string(job))
	}
	
	// Check remaining jobs
	remaining, err := store.LRange(ctx, queueKey, 0, -1)
	if err != nil && err != kv.ErrNotFound {
		log.Fatal(err)
	}
	
	fmt.Printf("Remaining jobs: %d\n", len(remaining))
}