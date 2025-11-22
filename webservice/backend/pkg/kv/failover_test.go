package kv

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// MockStore implements Store for testing
type MockStore struct {
	name              string
	shouldFail        atomic.Bool
	callCount         atomic.Int64
	failAfterCalls    int64
	connectionError   bool
	pingFailCount     atomic.Int64
	pingFailThreshold int64
	closed            atomic.Bool
}

func NewMockStore(name string) *MockStore {
	return &MockStore{name: name}
}

func (m *MockStore) SetFailAfter(calls int64, connectionError bool) {
	m.failAfterCalls = calls
	m.connectionError = connectionError
}

func (m *MockStore) SetPingFailThreshold(threshold int64) {
	m.pingFailThreshold = threshold
}

func (m *MockStore) GetCallCount() int64 {
	return m.callCount.Load()
}

func (m *MockStore) checkFailure() error {
	if m.closed.Load() {
		return errors.New("store is closed")
	}
	
	calls := m.callCount.Add(1)
	if m.failAfterCalls > 0 && calls > m.failAfterCalls {
		if m.connectionError {
			return ErrBackendUnavailable
		}
		return errors.New("mock failure")
	}
	return nil
}

// Store interface implementation
func (m *MockStore) Set(ctx context.Context, key string, value []byte, ttl ...time.Duration) error {
	return m.checkFailure()
}

func (m *MockStore) Get(ctx context.Context, key string) ([]byte, error) {
	if err := m.checkFailure(); err != nil {
		return nil, err
	}
	return []byte("mock-value"), nil
}

func (m *MockStore) SetString(ctx context.Context, key string, value string, ttl ...time.Duration) error {
	return m.checkFailure()
}

func (m *MockStore) GetString(ctx context.Context, key string) (string, error) {
	if err := m.checkFailure(); err != nil {
		return "", err
	}
	return "mock-value", nil
}

func (m *MockStore) Del(ctx context.Context, keys ...string) (int64, error) {
	if err := m.checkFailure(); err != nil {
		return 0, err
	}
	return int64(len(keys)), nil
}

func (m *MockStore) Exists(ctx context.Context, keys ...string) (int64, error) {
	if err := m.checkFailure(); err != nil {
		return 0, err
	}
	return 1, nil
}

func (m *MockStore) Expire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if err := m.checkFailure(); err != nil {
		return false, err
	}
	return true, nil
}

func (m *MockStore) TTL(ctx context.Context, key string) (time.Duration, error) {
	if err := m.checkFailure(); err != nil {
		return 0, err
	}
	return time.Minute, nil
}

func (m *MockStore) IncrBy(ctx context.Context, key string, n int64) (int64, error) {
	if err := m.checkFailure(); err != nil {
		return 0, err
	}
	return n, nil
}

func (m *MockStore) DecrBy(ctx context.Context, key string, n int64) (int64, error) {
	if err := m.checkFailure(); err != nil {
		return 0, err
	}
	return -n, nil
}

func (m *MockStore) HSet(ctx context.Context, key string, field string, value []byte) error {
	return m.checkFailure()
}

func (m *MockStore) HGet(ctx context.Context, key string, field string) ([]byte, error) {
	if err := m.checkFailure(); err != nil {
		return nil, err
	}
	return []byte("mock-hash-value"), nil
}

func (m *MockStore) HDel(ctx context.Context, key string, fields ...string) (int64, error) {
	if err := m.checkFailure(); err != nil {
		return 0, err
	}
	return int64(len(fields)), nil
}

func (m *MockStore) HGetAll(ctx context.Context, key string) (map[string][]byte, error) {
	if err := m.checkFailure(); err != nil {
		return nil, err
	}
	return map[string][]byte{"field": []byte("value")}, nil
}

func (m *MockStore) SAdd(ctx context.Context, key string, members ...[]byte) (int64, error) {
	if err := m.checkFailure(); err != nil {
		return 0, err
	}
	return int64(len(members)), nil
}

func (m *MockStore) SRem(ctx context.Context, key string, members ...[]byte) (int64, error) {
	if err := m.checkFailure(); err != nil {
		return 0, err
	}
	return int64(len(members)), nil
}

func (m *MockStore) SMembers(ctx context.Context, key string) ([][]byte, error) {
	if err := m.checkFailure(); err != nil {
		return nil, err
	}
	return [][]byte{[]byte("member1"), []byte("member2")}, nil
}

func (m *MockStore) SIsMember(ctx context.Context, key string, member []byte) (bool, error) {
	if err := m.checkFailure(); err != nil {
		return false, err
	}
	return true, nil
}

func (m *MockStore) LPush(ctx context.Context, key string, values ...[]byte) (int64, error) {
	if err := m.checkFailure(); err != nil {
		return 0, err
	}
	return int64(len(values)), nil
}

func (m *MockStore) RPush(ctx context.Context, key string, values ...[]byte) (int64, error) {
	if err := m.checkFailure(); err != nil {
		return 0, err
	}
	return int64(len(values)), nil
}

func (m *MockStore) LPop(ctx context.Context, key string) ([]byte, error) {
	if err := m.checkFailure(); err != nil {
		return nil, err
	}
	return []byte("popped-value"), nil
}

func (m *MockStore) RPop(ctx context.Context, key string) ([]byte, error) {
	if err := m.checkFailure(); err != nil {
		return nil, err
	}
	return []byte("popped-value"), nil
}

func (m *MockStore) LRange(ctx context.Context, key string, start, stop int64) ([][]byte, error) {
	if err := m.checkFailure(); err != nil {
		return nil, err
	}
	return [][]byte{[]byte("item1"), []byte("item2")}, nil
}

func (m *MockStore) MGet(ctx context.Context, keys ...string) ([][]byte, error) {
	if err := m.checkFailure(); err != nil {
		return nil, err
	}
	result := make([][]byte, len(keys))
	for i := range result {
		result[i] = []byte("mock-value")
	}
	return result, nil
}

func (m *MockStore) MSet(ctx context.Context, kv map[string][]byte, ttl ...time.Duration) error {
	return m.checkFailure()
}

func (m *MockStore) Ping(ctx context.Context) error {
	if m.closed.Load() {
		return errors.New("store is closed")
	}
	
	// Special ping failure logic
	if m.pingFailThreshold > 0 {
		count := m.pingFailCount.Add(1)
		if count <= m.pingFailThreshold {
			if m.connectionError {
				return ErrBackendUnavailable
			}
			return errors.New("ping failed")
		}
	}
	
	return m.checkFailure()
}

func (m *MockStore) Close() error {
	m.closed.Store(true)
	return nil
}

func TestFailoverStore_BasicFailover(t *testing.T) {
	primary := NewMockStore("primary")
	fallback := NewMockStore("fallback")
	
	var logMsgs []string
	var logMu sync.Mutex
	logger := func(msg string, fields ...any) {
		logMu.Lock()
		defer logMu.Unlock()
		logMsgs = append(logMsgs, msg)
	}
	
	fs := NewFailoverStore(primary, fallback, 10*time.Millisecond, logger)
	defer fs.Close()
	
	ctx := context.Background()
	
	// Initially should use primary
	if fs.GetActiveBackend() != "primary" {
		t.Errorf("Expected primary backend initially, got %s", fs.GetActiveBackend())
	}
	
	// First call should succeed on primary
	err := fs.Set(ctx, "key1", []byte("value1"))
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}
	
	if primary.GetCallCount() != 1 {
		t.Errorf("Expected 1 call to primary, got %d", primary.GetCallCount())
	}
	
	// Make primary fail with connection error after 1 call
	primary.SetFailAfter(1, true)
	
	// Next call should trigger failover
	err = fs.Set(ctx, "key2", []byte("value2"))
	if err != nil {
		t.Errorf("Expected success after failover, got error: %v", err)
	}
	
	// Should now be using fallback
	if fs.GetActiveBackend() != "fallback" {
		t.Errorf("Expected fallback backend after failover, got %s", fs.GetActiveBackend())
	}
	
	// Check that failover was logged
	time.Sleep(50 * time.Millisecond) // Give time for logging
	logMu.Lock()
	found := false
	for _, msg := range logMsgs {
		if msg == "Failing over to in-memory store" {
			found = true
			break
		}
	}
	logMu.Unlock()
	
	if !found {
		t.Errorf("Expected failover log message, got: %v", logMsgs)
	}
}

func TestFailoverStore_Recovery(t *testing.T) {
	primary := NewMockStore("primary")
	fallback := NewMockStore("fallback")
	
	var logMsgs []string
	var logMu sync.Mutex
	logger := func(msg string, fields ...any) {
		logMu.Lock()
		defer logMu.Unlock()
		logMsgs = append(logMsgs, msg)
	}
	
	// Start with fallback active (simulating startup failure)
	fs := NewFailoverStoreWithFallbackActive(primary, fallback, 20*time.Millisecond, logger)
	defer fs.Close()
	
	// Should start with fallback
	if fs.GetActiveBackend() != "fallback" {
		t.Errorf("Expected fallback backend initially, got %s", fs.GetActiveBackend())
	}
	
	// Make primary fail for first few pings, then succeed
	primary.SetPingFailThreshold(2) // Fail first 2 pings, then succeed
	primary.connectionError = true
	
	// Wait for recovery (should take 2-3 probe intervals)
	time.Sleep(80 * time.Millisecond)
	
	// Should recover to primary
	if fs.GetActiveBackend() != "primary" {
		t.Errorf("Expected primary backend after recovery, got %s", fs.GetActiveBackend())
	}
	
	// Check that recovery was logged
	logMu.Lock()
	found := false
	for _, msg := range logMsgs {
		if msg == "Recovered to primary store" {
			found = true
			break
		}
	}
	logMu.Unlock()
	
	if !found {
		t.Errorf("Expected recovery log message, got: %v", logMsgs)
	}
}

func TestFailoverStore_NoFailoverOnBusinessError(t *testing.T) {
	primary := NewMockStore("primary")
	fallback := NewMockStore("fallback")
	
	fs := NewFailoverStore(primary, fallback, 10*time.Millisecond, nil)
	defer fs.Close()
	
	ctx := context.Background()
	
	// Make primary fail with non-connection error
	primary.SetFailAfter(1, false) // false = not connection error
	
	// First call succeeds
	err := fs.Set(ctx, "key1", []byte("value1"))
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}
	
	// Second call should fail with business error, no failover
	err = fs.Set(ctx, "key2", []byte("value2"))
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
	
	// Should still be using primary (no failover)
	if fs.GetActiveBackend() != "primary" {
		t.Errorf("Expected primary backend (no failover), got %s", fs.GetActiveBackend())
	}
	
	if fallback.GetCallCount() > 0 {
		t.Errorf("Expected no calls to fallback, got %d", fallback.GetCallCount())
	}
}

func TestFailoverStore_ErrNotFoundHandling(t *testing.T) {
	// Create a custom mock that returns ErrNotFound
	primary := &MockStoreWithNotFound{MockStore: NewMockStore("primary")}
	fallback := NewMockStore("fallback")
	
	fs := NewFailoverStore(primary, fallback, 10*time.Millisecond, nil)
	defer fs.Close()
	
	ctx := context.Background()
	
	// Get should return ErrNotFound without triggering failover
	_, err := fs.Get(ctx, "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
	
	if primary.GetCallCount() == 0 {
		t.Errorf("Expected primary to be called")
	}
	
	if fs.GetActiveBackend() != "primary" {
		t.Errorf("Expected primary backend (ErrNotFound should not trigger failover), got %s", fs.GetActiveBackend())
	}
}

// MockStoreWithNotFound is a mock that can return ErrNotFound
type MockStoreWithNotFound struct {
	*MockStore
}

func (m *MockStoreWithNotFound) Get(ctx context.Context, key string) ([]byte, error) {
	if err := m.checkFailure(); err != nil {
		return nil, err
	}
	// Simulate key not found
	return nil, ErrNotFound
}

func TestFailoverStore_ConcurrentAccess(t *testing.T) {
	primary := NewMockStore("primary")
	fallback := NewMockStore("fallback")
	
	fs := NewFailoverStore(primary, fallback, 10*time.Millisecond, nil)
	defer fs.Close()
	
	ctx := context.Background()
	
	// Make primary fail after 10 calls
	primary.SetFailAfter(10, true)
	
	// Launch multiple goroutines
	const numGoroutines = 50
	const callsPerGoroutine = 10
	
	var wg sync.WaitGroup
	var errorCount atomic.Int64
	
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < callsPerGoroutine; j++ {
				err := fs.Set(ctx, "key", []byte("value"))
				if err != nil {
					errorCount.Add(1)
				}
				time.Sleep(time.Millisecond)
			}
		}()
	}
	
	wg.Wait()
	
	// Should have completed without errors (either primary or fallback should handle calls)
	if errorCount.Load() > 0 {
		t.Errorf("Expected no errors in concurrent access, got %d", errorCount.Load())
	}
	
	// Should have eventually failed over to fallback
	if fs.GetActiveBackend() != "fallback" {
		t.Errorf("Expected failover to fallback under concurrent load, got %s", fs.GetActiveBackend())
	}
}

func TestFailoverStore_CloseStopsProbing(t *testing.T) {
	primary := NewMockStore("primary")
	fallback := NewMockStore("fallback")
	
	fs := NewFailoverStoreWithFallbackActive(primary, fallback, 10*time.Millisecond, nil)
	
	// Give it time to start probing
	time.Sleep(20 * time.Millisecond)
	
	// Close should stop probing
	err := fs.Close()
	if err != nil {
		t.Errorf("Expected no error on close, got %v", err)
	}
	
	// Give it time to stop
	time.Sleep(30 * time.Millisecond)
	
	// Verify stores were closed
	if !primary.closed.Load() {
		t.Errorf("Expected primary to be closed")
	}
	
	if !fallback.closed.Load() {
		t.Errorf("Expected fallback to be closed")
	}
}