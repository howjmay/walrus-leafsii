package kv

import (
	"context"
	"fmt"
	"time"
)

// Backend represents the storage backend type
type Backend string

const (
	// BackendMemory uses the in-memory store
	BackendMemory Backend = "memory"
	// BackendRedis uses Redis as the backend
	BackendRedis Backend = "redis"
)

// Config holds configuration for creating a Store instance
type Config struct {
	// Backend specifies which storage backend to use
	Backend Backend
	
	// RedisURL is the connection string for Redis (required when Backend is "redis")
	// Format: redis://localhost:6379/0 or redis://:password@localhost:6379/1
	RedisURL string
	
	// JanitorInterval controls how often the in-memory store cleans up expired keys
	// Set to 0 to disable background cleanup (not recommended for production)
	// Default: 30 seconds
	JanitorInterval time.Duration
	
	// FailoverEnabled controls whether automatic failover to in-memory store is enabled
	// when Redis becomes unavailable. Default: true
	FailoverEnabled bool
	
	// ProbeInterval controls how often to probe Redis for recovery after failover
	// Default: 5 seconds
	ProbeInterval time.Duration
	
	// StartupProbeTimeout controls how long to wait for Redis at startup
	// Default: 1 second
	StartupProbeTimeout time.Duration
	
	// Logger is used for logging failover events. If nil, no logging occurs.
	Logger LogFunc
}

// StoreFactory defines a function that creates a Store instance
type StoreFactory func(cfg Config) (Store, error)

// factories holds registered store factories
var factories = make(map[Backend]StoreFactory)

// RegisterBackend registers a store factory for a given backend
func RegisterBackend(backend Backend, factory StoreFactory) {
	factories[backend] = factory
}

// NewStoreFromConfig creates a new Store instance based on the provided configuration
func NewStoreFromConfig(cfg Config) (Store, error) {
	// Set defaults
	if cfg.JanitorInterval == 0 {
		cfg.JanitorInterval = 30 * time.Second
	}
	if cfg.ProbeInterval == 0 {
		cfg.ProbeInterval = 5 * time.Second
	}
	if cfg.StartupProbeTimeout == 0 {
		cfg.StartupProbeTimeout = 1 * time.Second
	}
	
	switch cfg.Backend {
	case BackendMemory:
		// Always create memory store directly
		factory, exists := factories[BackendMemory]
		if !exists {
			return nil, fmt.Errorf("memory backend not registered")
		}
		return factory(cfg)
		
	case BackendRedis:
		return createRedisStoreWithFailover(cfg)
		
	default:
		return nil, fmt.Errorf("unsupported backend: %s (supported: %s, %s)", 
			cfg.Backend, BackendMemory, BackendRedis)
	}
}

// createRedisStoreWithFailover creates a Redis store with optional failover
func createRedisStoreWithFailover(cfg Config) (Store, error) {
	if cfg.RedisURL == "" {
		return nil, fmt.Errorf("redis URL is required when backend is 'redis'")
	}
	
	// Create memory store for failover
	memoryFactory, exists := factories[BackendMemory]
	if !exists {
		return nil, fmt.Errorf("memory backend not registered")
	}
	
	memoryStore, err := memoryFactory(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create memory store for failover: %w", err)
	}
	
	// If failover is disabled, try to create Redis directly or return memory with warning
	if !cfg.FailoverEnabled {
		redisFactory, exists := factories[BackendRedis]
		if !exists {
			return nil, fmt.Errorf("redis backend not registered")
		}
		
		redisStore, err := redisFactory(cfg)
		if err != nil {
			if cfg.Logger != nil {
				cfg.Logger("Failed to connect to Redis, using in-memory store", "error", err.Error())
			}
			return memoryStore, nil
		}
		
		// Test Redis connection
		ctx, cancel := context.WithTimeout(context.Background(), cfg.StartupProbeTimeout)
		defer cancel()
		
		if err := redisStore.Ping(ctx); err != nil {
			redisStore.Close() // Clean up failed store
			if cfg.Logger != nil {
				cfg.Logger("Redis health check failed at startup, using in-memory store", "error", err.Error())
			}
			return memoryStore, nil
		}
		
		// Redis is healthy, close memory store and return Redis
		memoryStore.Close()
		return redisStore, nil
	}
	
	// Failover is enabled - create Redis store and test it
	redisFactory, exists := factories[BackendRedis]
	if !exists {
		return nil, fmt.Errorf("redis backend not registered")
	}
	
	redisStore, err := redisFactory(cfg)
	if err != nil {
		// Redis creation failed, just use memory store without failover
		if cfg.Logger != nil {
			cfg.Logger("Redis unavailable at startup; using in-memory store", 
				"error", err.Error())
		}
		return memoryStore, nil
	}
	
	// Test Redis health at startup
	ctx, cancel := context.WithTimeout(context.Background(), cfg.StartupProbeTimeout)
	defer cancel()
	
	if err := redisStore.Ping(ctx); err != nil {
		// Redis is unhealthy at startup, start with memory and probe Redis for recovery
		if cfg.Logger != nil {
			cfg.Logger("Redis unhealthy at startup; using in-memory store (will retry in background)", 
				"error", err.Error())
		}
		return NewFailoverStoreWithFallbackActive(redisStore, memoryStore, cfg.ProbeInterval, cfg.Logger), nil
	}
	
	// Redis is healthy at startup, use it as primary with memory as fallback
	if cfg.Logger != nil {
		cfg.Logger("Redis healthy at startup; using Redis with in-memory failover")
	}
	return NewFailoverStore(redisStore, memoryStore, cfg.ProbeInterval, cfg.Logger), nil
}