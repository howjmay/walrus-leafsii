// Package kvtest provides conformance tests for kv.Store implementations
package kvtest

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/leafsii/leafsii-backend/pkg/kv"
)

// StoreFactory creates a fresh Store instance for testing
type StoreFactory func(t *testing.T) kv.Store

// RunConformanceTests runs all conformance tests against a Store implementation
func RunConformanceTests(t *testing.T, factory StoreFactory) {
	t.Run("StringOperations", func(t *testing.T) {
		testStringOperations(t, factory)
	})
	t.Run("KeyOperations", func(t *testing.T) {
		testKeyOperations(t, factory)
	})
	t.Run("TTLOperations", func(t *testing.T) {
		testTTLOperations(t, factory)
	})
	t.Run("CounterOperations", func(t *testing.T) {
		testCounterOperations(t, factory)
	})
	t.Run("HashOperations", func(t *testing.T) {
		testHashOperations(t, factory)
	})
	t.Run("SetOperations", func(t *testing.T) {
		testSetOperations(t, factory)
	})
	t.Run("ListOperations", func(t *testing.T) {
		testListOperations(t, factory)
	})
	t.Run("MultiOperations", func(t *testing.T) {
		testMultiOperations(t, factory)
	})
	t.Run("HealthCheck", func(t *testing.T) {
		testHealthCheck(t, factory)
	})
}

func testStringOperations(t *testing.T, factory StoreFactory) {
	tests := []struct {
		name string
		test func(t *testing.T, store kv.Store)
	}{
		{"SetGet", testSetGet},
		{"GetNonExistent", testGetNonExistent},
		{"SetString", testSetString},
		{"GetString", testGetString},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := factory(t)
			defer store.Close()
			tt.test(t, store)
		})
	}
}

func testSetGet(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key := "test:string"
	value := []byte("hello world")

	// Set value
	err := store.Set(ctx, key, value)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get value
	result, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if !reflect.DeepEqual(result, value) {
		t.Fatalf("Expected %v, got %v", value, result)
	}
}

func testGetNonExistent(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key := "test:nonexistent"

	_, err := store.Get(ctx, key)
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("Expected ErrNotFound, got %v", err)
	}
}

func testSetString(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key := "test:setstring"
	value := "hello string"

	err := store.SetString(ctx, key, value)
	if err != nil {
		t.Fatalf("SetString failed: %v", err)
	}

	result, err := store.GetString(ctx, key)
	if err != nil {
		t.Fatalf("GetString failed: %v", err)
	}

	if result != value {
		t.Fatalf("Expected %q, got %q", value, result)
	}
}

func testGetString(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key := "test:getstring"

	_, err := store.GetString(ctx, key)
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("Expected ErrNotFound, got %v", err)
	}
}

func testKeyOperations(t *testing.T, factory StoreFactory) {
	tests := []struct {
		name string
		test func(t *testing.T, store kv.Store)
	}{
		{"Del", testDel},
		{"Exists", testExists},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := factory(t)
			defer store.Close()
			tt.test(t, store)
		})
	}
}

func testDel(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key1, key2 := "test:del1", "test:del2"
	value := []byte("test")

	// Set two keys
	store.Set(ctx, key1, value)
	store.Set(ctx, key2, value)

	// Delete one key
	deleted, err := store.Del(ctx, key1)
	if err != nil {
		t.Fatalf("Del failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("Expected 1 deleted, got %d", deleted)
	}

	// Verify key1 is gone, key2 remains
	_, err = store.Get(ctx, key1)
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("Expected ErrNotFound for deleted key, got %v", err)
	}

	_, err = store.Get(ctx, key2)
	if err != nil {
		t.Fatalf("Expected key2 to still exist, got %v", err)
	}
}

func testExists(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key := "test:exists"
	value := []byte("test")

	// Key doesn't exist initially
	count, err := store.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("Expected 0 for non-existent key, got %d", count)
	}

	// Set key
	store.Set(ctx, key, value)

	// Key exists now
	count, err = store.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("Expected 1 for existing key, got %d", count)
	}
}

func testTTLOperations(t *testing.T, factory StoreFactory) {
	tests := []struct {
		name string
		test func(t *testing.T, store kv.Store)
	}{
		{"SetWithTTL", testSetWithTTL},
		{"Expire", testExpire},
		{"TTL", testTTL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := factory(t)
			defer store.Close()
			tt.test(t, store)
		})
	}
}

func testSetWithTTL(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key := "test:ttl"
	value := []byte("expires")
	ttl := 100 * time.Millisecond

	// Set with TTL
	err := store.Set(ctx, key, value, ttl)
	if err != nil {
		t.Fatalf("Set with TTL failed: %v", err)
	}

	// Key should exist initially
	_, err = store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Expected key to exist initially, got %v", err)
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Key should be expired
	_, err = store.Get(ctx, key)
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("Expected key to be expired, got %v", err)
	}
}

func testExpire(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key := "test:expire"
	value := []byte("test")

	// Set key without TTL
	store.Set(ctx, key, value)

	// Set expiration
	expired, err := store.Expire(ctx, key, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Expire failed: %v", err)
	}
	if !expired {
		t.Fatalf("Expected Expire to return true for existing key")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Key should be expired
	_, err = store.Get(ctx, key)
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("Expected key to be expired, got %v", err)
	}
}

func testTTL(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key := "test:ttl-check"
	value := []byte("test")

	// Non-existent key
	_, err := store.TTL(ctx, key)
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("Expected ErrNotFound for non-existent key, got %v", err)
	}

	// Key without TTL
	store.Set(ctx, key, value)
	ttl, err := store.TTL(ctx, key)
	if err != nil {
		t.Fatalf("TTL failed: %v", err)
	}
	if ttl != -1 {
		t.Fatalf("Expected -1 for key without TTL, got %v", ttl)
	}

	// Key with TTL
	store.Set(ctx, key, value, 500*time.Millisecond)
	ttl, err = store.TTL(ctx, key)
	if err != nil {
		t.Fatalf("TTL failed: %v", err)
	}
	if ttl <= 0 || ttl > 500*time.Millisecond {
		t.Fatalf("Expected TTL between 0 and 500ms, got %v", ttl)
	}
}

func testCounterOperations(t *testing.T, factory StoreFactory) {
	tests := []struct {
		name string
		test func(t *testing.T, store kv.Store)
	}{
		{"IncrBy", testIncrBy},
		{"DecrBy", testDecrBy},
		{"IncrByInvalidValue", testIncrByInvalidValue},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := factory(t)
			defer store.Close()
			tt.test(t, store)
		})
	}
}

func testIncrBy(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key := "test:counter"

	// Increment non-existent key
	result, err := store.IncrBy(ctx, key, 5)
	if err != nil {
		t.Fatalf("IncrBy failed: %v", err)
	}
	if result != 5 {
		t.Fatalf("Expected 5, got %d", result)
	}

	// Increment existing key
	result, err = store.IncrBy(ctx, key, 3)
	if err != nil {
		t.Fatalf("IncrBy failed: %v", err)
	}
	if result != 8 {
		t.Fatalf("Expected 8, got %d", result)
	}
}

func testDecrBy(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key := "test:decr"

	// Start with a value
	store.IncrBy(ctx, key, 10)

	// Decrement
	result, err := store.DecrBy(ctx, key, 3)
	if err != nil {
		t.Fatalf("DecrBy failed: %v", err)
	}
	if result != 7 {
		t.Fatalf("Expected 7, got %d", result)
	}
}

func testIncrByInvalidValue(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key := "test:invalid"

	// Set non-numeric value
	store.Set(ctx, key, []byte("not-a-number"))

	// Try to increment
	_, err := store.IncrBy(ctx, key, 1)
	if err == nil {
		t.Fatalf("Expected error for non-numeric value")
	}
}

func testHashOperations(t *testing.T, factory StoreFactory) {
	tests := []struct {
		name string
		test func(t *testing.T, store kv.Store)
	}{
		{"HSetGet", testHSetGet},
		{"HGetAll", testHGetAll},
		{"HDel", testHDel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := factory(t)
			defer store.Close()
			tt.test(t, store)
		})
	}
}

func testHSetGet(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key := "test:hash"
	field := "field1"
	value := []byte("value1")

	// Set hash field
	err := store.HSet(ctx, key, field, value)
	if err != nil {
		t.Fatalf("HSet failed: %v", err)
	}

	// Get hash field
	result, err := store.HGet(ctx, key, field)
	if err != nil {
		t.Fatalf("HGet failed: %v", err)
	}

	if !reflect.DeepEqual(result, value) {
		t.Fatalf("Expected %v, got %v", value, result)
	}

	// Get non-existent field
	_, err = store.HGet(ctx, key, "nonexistent")
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("Expected ErrNotFound for non-existent field, got %v", err)
	}
}

func testHGetAll(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key := "test:hash-all"

	// Set multiple fields
	store.HSet(ctx, key, "field1", []byte("value1"))
	store.HSet(ctx, key, "field2", []byte("value2"))

	// Get all fields
	result, err := store.HGetAll(ctx, key)
	if err != nil {
		t.Fatalf("HGetAll failed: %v", err)
	}

	expected := map[string][]byte{
		"field1": []byte("value1"),
		"field2": []byte("value2"),
	}

	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("Expected %v, got %v", expected, result)
	}

	// Get all for non-existent key
	_, err = store.HGetAll(ctx, "nonexistent")
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("Expected ErrNotFound for non-existent key, got %v", err)
	}
}

func testHDel(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key := "test:hash-del"

	// Set fields
	store.HSet(ctx, key, "field1", []byte("value1"))
	store.HSet(ctx, key, "field2", []byte("value2"))

	// Delete one field
	deleted, err := store.HDel(ctx, key, "field1")
	if err != nil {
		t.Fatalf("HDel failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("Expected 1 deleted, got %d", deleted)
	}

	// Verify field1 is gone, field2 remains
	_, err = store.HGet(ctx, key, "field1")
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("Expected field1 to be deleted")
	}

	_, err = store.HGet(ctx, key, "field2")
	if err != nil {
		t.Fatalf("Expected field2 to remain: %v", err)
	}
}

func testSetOperations(t *testing.T, factory StoreFactory) {
	tests := []struct {
		name string
		test func(t *testing.T, store kv.Store)
	}{
		{"SAddMembers", testSAddMembers},
		{"SRem", testSRem},
		{"SIsMember", testSIsMember},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := factory(t)
			defer store.Close()
			tt.test(t, store)
		})
	}
}

func testSAddMembers(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key := "test:set"

	// Add members
	added, err := store.SAdd(ctx, key, []byte("member1"), []byte("member2"))
	if err != nil {
		t.Fatalf("SAdd failed: %v", err)
	}
	if added != 2 {
		t.Fatalf("Expected 2 added, got %d", added)
	}

	// Get members
	members, err := store.SMembers(ctx, key)
	if err != nil {
		t.Fatalf("SMembers failed: %v", err)
	}

	if len(members) != 2 {
		t.Fatalf("Expected 2 members, got %d", len(members))
	}

	// Add duplicate member
	added, err = store.SAdd(ctx, key, []byte("member1"))
	if err != nil {
		t.Fatalf("SAdd failed: %v", err)
	}
	if added != 0 {
		t.Fatalf("Expected 0 added for duplicate, got %d", added)
	}
}

func testSRem(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key := "test:set-rem"

	// Add members
	store.SAdd(ctx, key, []byte("member1"), []byte("member2"))

	// Remove member
	removed, err := store.SRem(ctx, key, []byte("member1"))
	if err != nil {
		t.Fatalf("SRem failed: %v", err)
	}
	if removed != 1 {
		t.Fatalf("Expected 1 removed, got %d", removed)
	}

	// Verify member is gone
	isMember, err := store.SIsMember(ctx, key, []byte("member1"))
	if err != nil {
		t.Fatalf("SIsMember failed: %v", err)
	}
	if isMember {
		t.Fatalf("Expected member1 to be removed")
	}
}

func testSIsMember(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key := "test:set-member"

	// Add member
	store.SAdd(ctx, key, []byte("member1"))

	// Check membership
	isMember, err := store.SIsMember(ctx, key, []byte("member1"))
	if err != nil {
		t.Fatalf("SIsMember failed: %v", err)
	}
	if !isMember {
		t.Fatalf("Expected member1 to be a member")
	}

	// Check non-member
	isMember, err = store.SIsMember(ctx, key, []byte("nonmember"))
	if err != nil {
		t.Fatalf("SIsMember failed: %v", err)
	}
	if isMember {
		t.Fatalf("Expected nonmember to not be a member")
	}
}

func testListOperations(t *testing.T, factory StoreFactory) {
	tests := []struct {
		name string
		test func(t *testing.T, store kv.Store)
	}{
		{"LPushPop", testLPushPop},
		{"RPushPop", testRPushPop},
		{"LRange", testLRange},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := factory(t)
			defer store.Close()
			tt.test(t, store)
		})
	}
}

func testLPushPop(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key := "test:list-left"

	// Push values
	length, err := store.LPush(ctx, key, []byte("value1"), []byte("value2"))
	if err != nil {
		t.Fatalf("LPush failed: %v", err)
	}
	if length != 2 {
		t.Fatalf("Expected length 2, got %d", length)
	}

	// Pop value (should be value2 due to LIFO)
	value, err := store.LPop(ctx, key)
	if err != nil {
		t.Fatalf("LPop failed: %v", err)
	}
	if !reflect.DeepEqual(value, []byte("value2")) {
		t.Fatalf("Expected value2, got %v", value)
	}

	// Pop remaining value
	value, err = store.LPop(ctx, key)
	if err != nil {
		t.Fatalf("LPop failed: %v", err)
	}
	if !reflect.DeepEqual(value, []byte("value1")) {
		t.Fatalf("Expected value1, got %v", value)
	}

	// Pop from empty list
	_, err = store.LPop(ctx, key)
	if !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("Expected ErrNotFound for empty list, got %v", err)
	}
}

func testRPushPop(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key := "test:list-right"

	// Push values
	length, err := store.RPush(ctx, key, []byte("value1"), []byte("value2"))
	if err != nil {
		t.Fatalf("RPush failed: %v", err)
	}
	if length != 2 {
		t.Fatalf("Expected length 2, got %d", length)
	}

	// Pop value (should be value2 due to LIFO from right)
	value, err := store.RPop(ctx, key)
	if err != nil {
		t.Fatalf("RPop failed: %v", err)
	}
	if !reflect.DeepEqual(value, []byte("value2")) {
		t.Fatalf("Expected value2, got %v", value)
	}
}

func testLRange(t *testing.T, store kv.Store) {
	ctx := context.Background()
	key := "test:list-range"

	// Push values
	store.RPush(ctx, key, []byte("value1"), []byte("value2"), []byte("value3"))

	// Get range
	values, err := store.LRange(ctx, key, 0, 1)
	if err != nil {
		t.Fatalf("LRange failed: %v", err)
	}

	expected := [][]byte{[]byte("value1"), []byte("value2")}
	if !reflect.DeepEqual(values, expected) {
		t.Fatalf("Expected %v, got %v", expected, values)
	}

	// Get all values
	values, err = store.LRange(ctx, key, 0, -1)
	if err != nil {
		t.Fatalf("LRange failed: %v", err)
	}
	if len(values) != 3 {
		t.Fatalf("Expected 3 values, got %d", len(values))
	}
}

func testMultiOperations(t *testing.T, factory StoreFactory) {
	tests := []struct {
		name string
		test func(t *testing.T, store kv.Store)
	}{
		{"MSetGet", testMSetGet},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := factory(t)
			defer store.Close()
			tt.test(t, store)
		})
	}
}

func testMSetGet(t *testing.T, store kv.Store) {
	ctx := context.Background()

	// Set multiple keys
	kvPairs := map[string][]byte{
		"test:multi1": []byte("value1"),
		"test:multi2": []byte("value2"),
		"test:multi3": []byte("value3"),
	}

	err := store.MSet(ctx, kvPairs)
	if err != nil {
		t.Fatalf("MSet failed: %v", err)
	}

	// Get multiple keys
	keys := []string{"test:multi1", "test:multi2", "test:nonexistent"}
	values, err := store.MGet(ctx, keys...)
	if err != nil {
		t.Fatalf("MGet failed: %v", err)
	}

	if len(values) != 3 {
		t.Fatalf("Expected 3 values, got %d", len(values))
	}

	if !reflect.DeepEqual(values[0], []byte("value1")) {
		t.Fatalf("Expected value1, got %v", values[0])
	}

	if !reflect.DeepEqual(values[1], []byte("value2")) {
		t.Fatalf("Expected value2, got %v", values[1])
	}

	if values[2] != nil {
		t.Fatalf("Expected nil for non-existent key, got %v", values[2])
	}
}

func testHealthCheck(t *testing.T, factory StoreFactory) {
	store := factory(t)
	defer store.Close()
	
	ctx := context.Background()
	
	// Ping should not error for healthy store
	err := store.Ping(ctx)
	if err != nil {
		t.Fatalf("Ping failed for healthy store: %v", err)
	}
}