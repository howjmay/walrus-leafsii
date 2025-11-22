package redis

import (
	"context"
	"os"
	"testing"

	"github.com/leafsii/leafsii-backend/pkg/kv"
	"github.com/leafsii/leafsii-backend/pkg/kv/kvtest"
)

func TestRedisStore(t *testing.T) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		t.Skip("REDIS_URL not set, skipping Redis tests")
	}

	factory := func(t *testing.T) kv.Store {
		store, err := New(redisURL)
		if err != nil {
			t.Fatalf("Failed to create Redis store: %v", err)
		}

		// Clean up any existing test keys
		store.Del(context.Background(), "test:*")

		return store
	}

	kvtest.RunConformanceTests(t, factory)
}
