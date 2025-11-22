package memory

import (
	"time"

	"github.com/leafsii/leafsii-backend/pkg/kv"
)

func init() {
	kv.RegisterBackend(kv.BackendMemory, func(cfg kv.Config) (kv.Store, error) {
		interval := cfg.JanitorInterval
		if interval == 0 {
			interval = 30 * time.Second // Default interval
		}
		return New(interval), nil
	})
}

// NewStore creates a new in-memory store with default janitor interval
func NewStore() kv.Store {
	return New(30 * time.Second)
}

// NewStoreWithInterval creates a new in-memory store with custom janitor interval
func NewStoreWithInterval(interval time.Duration) kv.Store {
	return New(interval)
}