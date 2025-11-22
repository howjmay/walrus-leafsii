package memory

import (
	"context"
	"testing"
	"time"

	"github.com/leafsii/leafsii-backend/pkg/kv"
	"github.com/leafsii/leafsii-backend/pkg/kv/kvtest"
)

func TestMemoryStore(t *testing.T) {
	factory := func(t *testing.T) kv.Store {
		return New(0) // Disable janitor for deterministic tests
	}
	
	kvtest.RunConformanceTests(t, factory)
}

func TestMemoryStoreWithJanitor(t *testing.T) {
	// Test with a short janitor interval for faster cleanup testing
	store := New(10 * time.Millisecond)
	defer store.Close()
	
	ctx := context.Background()
	key := "test:janitor"
	value := []byte("test")
	
	// Set key with short TTL
	err := store.Set(ctx, key, value, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	
	// Key should exist initially
	_, err = store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Expected key to exist initially: %v", err)
	}
	
	// Wait for janitor to clean up
	time.Sleep(50 * time.Millisecond)
	
	// Key should be cleaned up by janitor
	_, err = store.Get(ctx, key)
	if err != kv.ErrNotFound {
		t.Fatalf("Expected key to be cleaned up by janitor: %v", err)
	}
}