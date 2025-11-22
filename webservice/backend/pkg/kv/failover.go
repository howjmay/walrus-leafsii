package kv

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// LogFunc is a function type for structured logging
type LogFunc func(msg string, fields ...any)

// FailoverStore wraps a primary and fallback store, automatically failing over
// when the primary becomes unavailable and recovering when it becomes healthy again
type FailoverStore struct {
	primary      Store         // Primary store (usually Redis)
	fallback     Store         // Fallback store (usually in-memory)
	active       atomic.Value  // Currently active store (Store)
	probeInterval time.Duration
	logger       LogFunc
	
	// State management
	mu           sync.Mutex
	probing      bool          // Whether background probing is active
	closed       chan struct{} // Signal to stop background processes
	probeStop    chan struct{} // Signal to stop current probe goroutine
	probeDone    chan struct{} // Signal that probe goroutine has stopped
	promote      chan struct{} // Signal to promote to primary
}

// NewFailoverStore creates a new failover store that prefers the primary but falls back to fallback
func NewFailoverStore(primary, fallback Store, probeInterval time.Duration, logger LogFunc) *FailoverStore {
	if logger == nil {
		logger = func(msg string, fields ...any) {} // No-op logger
	}
	
	fs := &FailoverStore{
		primary:       primary,
		fallback:      fallback,
		probeInterval: probeInterval,
		logger:        logger,
		closed:        make(chan struct{}),
		promote:       make(chan struct{}, 1), // Buffered channel
	}
	
	// Start with primary as active
	fs.active.Store(primary)
	
	// Start promotion handler
	go fs.handlePromotions()
	
	return fs
}

// NewFailoverStoreWithFallbackActive creates a failover store that starts with fallback active
// and probes primary for recovery (used when primary fails at startup)
func NewFailoverStoreWithFallbackActive(primary, fallback Store, probeInterval time.Duration, logger LogFunc) *FailoverStore {
	fs := NewFailoverStore(primary, fallback, probeInterval, logger)
	
	// Start with fallback as active and begin probing primary
	fs.active.Store(fallback)
	fs.startProbing()
	
	// Start promotion handler
	go fs.handlePromotions()
	
	return fs
}

// getActiveStore returns the currently active store
func (fs *FailoverStore) getActiveStore() Store {
	return fs.active.Load().(Store)
}

// demoteToFallback switches to the fallback store and starts background probing for recovery
func (fs *FailoverStore) demoteToFallback() {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	
	// Check if we're already using fallback
	if fs.getActiveStore() == fs.fallback {
		return
	}
	
	// Switch to fallback
	fs.active.Store(fs.fallback)
	fs.logger("Failing over to in-memory store", "reason", "primary_unavailable")
	
	// Start probing for recovery
	fs.startProbingUnsafe()
}

// handlePromotions handles promotion signals in a separate goroutine
func (fs *FailoverStore) handlePromotions() {
	for {
		select {
		case <-fs.closed:
			return
		case <-fs.promote:
			// Check if we're already using primary
			if fs.getActiveStore() == fs.primary {
				continue
			}
			
			// Switch to primary
			fs.active.Store(fs.primary)
			fs.logger("Recovered to primary store", "reason", "primary_healthy")
			
			// Stop probing
			fs.stopProbing()
		}
	}
}

// signalPromotion signals that primary should be promoted (non-blocking)
func (fs *FailoverStore) signalPromotion() {
	select {
	case fs.promote <- struct{}{}:
		// Signal sent
	default:
		// Channel full, promotion already pending
	}
}

// startProbing starts background probing if not already active (must hold mutex)
func (fs *FailoverStore) startProbingUnsafe() {
	if fs.probing {
		return
	}
	
	fs.probing = true
	fs.probeStop = make(chan struct{})
	fs.probeDone = make(chan struct{})
	
	go fs.probeLoop()
}

// startProbing starts background probing (external interface)
func (fs *FailoverStore) startProbing() {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.startProbingUnsafe()
}

// stopProbing stops background probing (external interface)
func (fs *FailoverStore) stopProbing() {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.stopProbingUnsafe()
}

// stopProbingUnsafe stops background probing (must hold mutex)
func (fs *FailoverStore) stopProbingUnsafe() {
	if !fs.probing {
		return
	}
	
	close(fs.probeStop)
	<-fs.probeDone
	fs.probing = false
}

// probeLoop runs the background health probing
func (fs *FailoverStore) probeLoop() {
	defer close(fs.probeDone)
	
	ticker := time.NewTicker(fs.probeInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-fs.closed:
			return
		case <-fs.probeStop:
			return
		case <-ticker.C:
			// Only probe if we have a primary
			if fs.primary == nil {
				continue
			}
			
			// Probe primary health
			ctx, cancel := context.WithTimeout(context.Background(), fs.probeInterval/2)
			err := fs.primary.Ping(ctx)
			cancel()
			
			if err == nil {
				// Primary is healthy, signal promotion
				fs.signalPromotion()
				return // Stop probing until next demotion
			}
		}
	}
}

// executeWithFailover executes a function on the active store and handles failover
func (fs *FailoverStore) executeWithFailover(fn func(Store) error) error {
	store := fs.getActiveStore()
	err := fn(store)
	
	// If primary store failed with a connection error, try failover
	if fs.primary != nil && store == fs.primary && errors.Is(err, ErrBackendUnavailable) {
		fs.demoteToFallback()
		
		// Retry with fallback store
		fallbackStore := fs.getActiveStore()
		if fallbackStore != store {
			return fn(fallbackStore)
		}
	}
	
	return err
}

// executeWithFailoverAndResult executes a function that returns a value and handles failover
func (fs *FailoverStore) executeWithFailoverAndResult(fn func(Store) (interface{}, error)) (interface{}, error) {
	store := fs.getActiveStore()
	result, err := fn(store)
	
	// If primary store failed with a connection error, try failover
	if fs.primary != nil && store == fs.primary && errors.Is(err, ErrBackendUnavailable) {
		fs.demoteToFallback()
		
		// Retry with fallback store
		fallbackStore := fs.getActiveStore()
		if fallbackStore != store {
			return fn(fallbackStore)
		}
	}
	
	return result, err
}

// String operations

func (fs *FailoverStore) Set(ctx context.Context, key string, value []byte, ttl ...time.Duration) error {
	return fs.executeWithFailover(func(store Store) error {
		return store.Set(ctx, key, value, ttl...)
	})
}

func (fs *FailoverStore) Get(ctx context.Context, key string) ([]byte, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.Get(ctx, key)
	})
	if err != nil {
		return nil, err
	}
	return result.([]byte), nil
}

func (fs *FailoverStore) SetString(ctx context.Context, key string, value string, ttl ...time.Duration) error {
	return fs.executeWithFailover(func(store Store) error {
		return store.SetString(ctx, key, value, ttl...)
	})
}

func (fs *FailoverStore) GetString(ctx context.Context, key string) (string, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.GetString(ctx, key)
	})
	if err != nil {
		return "", err
	}
	return result.(string), nil
}

// Key operations

func (fs *FailoverStore) Del(ctx context.Context, keys ...string) (int64, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.Del(ctx, keys...)
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (fs *FailoverStore) Exists(ctx context.Context, keys ...string) (int64, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.Exists(ctx, keys...)
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (fs *FailoverStore) Expire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.Expire(ctx, key, ttl)
	})
	if err != nil {
		return false, err
	}
	return result.(bool), nil
}

func (fs *FailoverStore) TTL(ctx context.Context, key string) (time.Duration, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.TTL(ctx, key)
	})
	if err != nil {
		return 0, err
	}
	return result.(time.Duration), nil
}

// Counter operations

func (fs *FailoverStore) IncrBy(ctx context.Context, key string, n int64) (int64, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.IncrBy(ctx, key, n)
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (fs *FailoverStore) DecrBy(ctx context.Context, key string, n int64) (int64, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.DecrBy(ctx, key, n)
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

// Hash operations

func (fs *FailoverStore) HSet(ctx context.Context, key string, field string, value []byte) error {
	return fs.executeWithFailover(func(store Store) error {
		return store.HSet(ctx, key, field, value)
	})
}

func (fs *FailoverStore) HGet(ctx context.Context, key string, field string) ([]byte, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.HGet(ctx, key, field)
	})
	if err != nil {
		return nil, err
	}
	return result.([]byte), nil
}

func (fs *FailoverStore) HDel(ctx context.Context, key string, fields ...string) (int64, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.HDel(ctx, key, fields...)
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (fs *FailoverStore) HGetAll(ctx context.Context, key string) (map[string][]byte, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.HGetAll(ctx, key)
	})
	if err != nil {
		return nil, err
	}
	return result.(map[string][]byte), nil
}

// Set operations

func (fs *FailoverStore) SAdd(ctx context.Context, key string, members ...[]byte) (int64, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.SAdd(ctx, key, members...)
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (fs *FailoverStore) SRem(ctx context.Context, key string, members ...[]byte) (int64, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.SRem(ctx, key, members...)
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (fs *FailoverStore) SMembers(ctx context.Context, key string) ([][]byte, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.SMembers(ctx, key)
	})
	if err != nil {
		return nil, err
	}
	return result.([][]byte), nil
}

func (fs *FailoverStore) SIsMember(ctx context.Context, key string, member []byte) (bool, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.SIsMember(ctx, key, member)
	})
	if err != nil {
		return false, err
	}
	return result.(bool), nil
}

// List operations

func (fs *FailoverStore) LPush(ctx context.Context, key string, values ...[]byte) (int64, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.LPush(ctx, key, values...)
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (fs *FailoverStore) RPush(ctx context.Context, key string, values ...[]byte) (int64, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.RPush(ctx, key, values...)
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (fs *FailoverStore) LPop(ctx context.Context, key string) ([]byte, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.LPop(ctx, key)
	})
	if err != nil {
		return nil, err
	}
	return result.([]byte), nil
}

func (fs *FailoverStore) RPop(ctx context.Context, key string) ([]byte, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.RPop(ctx, key)
	})
	if err != nil {
		return nil, err
	}
	return result.([]byte), nil
}

func (fs *FailoverStore) LRange(ctx context.Context, key string, start, stop int64) ([][]byte, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.LRange(ctx, key, start, stop)
	})
	if err != nil {
		return nil, err
	}
	return result.([][]byte), nil
}

// Multi operations

func (fs *FailoverStore) MGet(ctx context.Context, keys ...string) ([][]byte, error) {
	result, err := fs.executeWithFailoverAndResult(func(store Store) (interface{}, error) {
		return store.MGet(ctx, keys...)
	})
	if err != nil {
		return nil, err
	}
	return result.([][]byte), nil
}

func (fs *FailoverStore) MSet(ctx context.Context, kv map[string][]byte, ttl ...time.Duration) error {
	return fs.executeWithFailover(func(store Store) error {
		return store.MSet(ctx, kv, ttl...)
	})
}

// Health check

func (fs *FailoverStore) Ping(ctx context.Context) error {
	// Ping the active store
	return fs.getActiveStore().Ping(ctx)
}

// GetActiveBackend returns information about which backend is currently active
func (fs *FailoverStore) GetActiveBackend() string {
	if fs.primary != nil && fs.getActiveStore() == fs.primary {
		return "primary"
	}
	return "fallback"
}

// Close shuts down the failover store and stops all background processes
func (fs *FailoverStore) Close() error {
	// Signal shutdown
	close(fs.closed)
	
	// Stop probing if active
	fs.mu.Lock()
	if fs.probing {
		fs.stopProbingUnsafe()
	}
	fs.mu.Unlock()
	
	// Close underlying stores
	var errs []error
	
	if fs.primary != nil {
		if err := fs.primary.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	
	if err := fs.fallback.Close(); err != nil {
		errs = append(errs, err)
	}
	
	// Return first error if any
	if len(errs) > 0 {
		return errs[0]
	}
	
	return nil
}