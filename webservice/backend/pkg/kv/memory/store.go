package memory

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/leafsii/leafsii-backend/pkg/kv"
)

// Store is an in-memory implementation of the kv.Store interface
type Store struct {
	mu          sync.RWMutex
	strings     map[string][]byte
	hashes      map[string]map[string][]byte
	sets        map[string]map[string]struct{}
	lists       map[string][][]byte
	expirations map[string]time.Time
	
	janitorInterval time.Duration
	janitorStop     chan struct{}
	janitorDone     chan struct{}
}

// New creates a new in-memory store with optional janitor for TTL cleanup
func New(janitorInterval time.Duration) *Store {
	s := &Store{
		strings:         make(map[string][]byte),
		hashes:          make(map[string]map[string][]byte),
		sets:            make(map[string]map[string]struct{}),
		lists:           make(map[string][][]byte),
		expirations:     make(map[string]time.Time),
		janitorInterval: janitorInterval,
		janitorStop:     make(chan struct{}),
		janitorDone:     make(chan struct{}),
	}
	
	if janitorInterval > 0 {
		go s.janitor()
	} else {
		close(s.janitorDone)
	}
	
	return s
}

// janitor runs background expiration cleanup
func (s *Store) janitor() {
	defer close(s.janitorDone)
	ticker := time.NewTicker(s.janitorInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			s.evictExpired()
		case <-s.janitorStop:
			return
		}
	}
}

// evictExpired removes all expired keys
func (s *Store) evictExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	now := time.Now()
	for key, expiry := range s.expirations {
		if now.After(expiry) {
			s.deleteKeyUnsafe(key)
			delete(s.expirations, key)
		}
	}
}

// isExpired checks if a key has expired (must hold read lock)
func (s *Store) isExpired(key string) bool {
	if expiry, exists := s.expirations[key]; exists {
		return time.Now().After(expiry)
	}
	return false
}

// setExpiration sets TTL for a key (must hold write lock)
func (s *Store) setExpiration(key string, ttl time.Duration) {
	if ttl > 0 {
		s.expirations[key] = time.Now().Add(ttl)
	} else {
		delete(s.expirations, key)
	}
}

// deleteKeyUnsafe removes a key from all data structures (must hold write lock)
func (s *Store) deleteKeyUnsafe(key string) {
	delete(s.strings, key)
	delete(s.hashes, key)
	delete(s.sets, key)
	delete(s.lists, key)
}

// String operations

func (s *Store) Set(ctx context.Context, key string, value []byte, ttl ...time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.deleteKeyUnsafe(key)
	s.strings[key] = value
	
	if len(ttl) > 0 && ttl[0] > 0 {
		s.setExpiration(key, ttl[0])
	}
	
	return nil
}

func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if s.isExpired(key) {
		s.mu.RUnlock()
		s.mu.Lock()
		s.deleteKeyUnsafe(key)
		delete(s.expirations, key)
		s.mu.Unlock()
		s.mu.RLock()
		return nil, kv.ErrNotFound
	}
	
	value, exists := s.strings[key]
	if !exists {
		return nil, kv.ErrNotFound
	}
	
	return value, nil
}

func (s *Store) SetString(ctx context.Context, key string, value string, ttl ...time.Duration) error {
	return s.Set(ctx, key, []byte(value), ttl...)
}

func (s *Store) GetString(ctx context.Context, key string) (string, error) {
	data, err := s.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Key operations

func (s *Store) Del(ctx context.Context, keys ...string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	var deleted int64
	for _, key := range keys {
		if _, exists := s.strings[key]; exists {
			deleted++
		} else if _, exists := s.hashes[key]; exists {
			deleted++
		} else if _, exists := s.sets[key]; exists {
			deleted++
		} else if _, exists := s.lists[key]; exists {
			deleted++
		}
		
		s.deleteKeyUnsafe(key)
		delete(s.expirations, key)
	}
	
	return deleted, nil
}

func (s *Store) Exists(ctx context.Context, keys ...string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	var exists int64
	for _, key := range keys {
		if s.isExpired(key) {
			continue
		}
		
		if _, found := s.strings[key]; found {
			exists++
		} else if _, found := s.hashes[key]; found {
			exists++
		} else if _, found := s.sets[key]; found {
			exists++
		} else if _, found := s.lists[key]; found {
			exists++
		}
	}
	
	return exists, nil
}

func (s *Store) Expire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.isExpired(key) {
		s.deleteKeyUnsafe(key)
		delete(s.expirations, key)
		return false, nil
	}
	
	keyExists := false
	if _, exists := s.strings[key]; exists {
		keyExists = true
	} else if _, exists := s.hashes[key]; exists {
		keyExists = true
	} else if _, exists := s.sets[key]; exists {
		keyExists = true
	} else if _, exists := s.lists[key]; exists {
		keyExists = true
	}
	
	if !keyExists {
		return false, nil
	}
	
	s.setExpiration(key, ttl)
	return true, nil
}

func (s *Store) TTL(ctx context.Context, key string) (time.Duration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	expiry, hasExpiry := s.expirations[key]
	if !hasExpiry {
		keyExists := false
		if _, exists := s.strings[key]; exists {
			keyExists = true
		} else if _, exists := s.hashes[key]; exists {
			keyExists = true
		} else if _, exists := s.sets[key]; exists {
			keyExists = true
		} else if _, exists := s.lists[key]; exists {
			keyExists = true
		}
		
		if !keyExists {
			return 0, kv.ErrNotFound
		}
		return -1, nil // Key exists but has no expiration
	}
	
	remaining := time.Until(expiry)
	if remaining <= 0 {
		return 0, nil // Key has expired
	}
	
	return remaining, nil
}

// Counter operations

func (s *Store) IncrBy(ctx context.Context, key string, n int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.isExpired(key) {
		s.deleteKeyUnsafe(key)
		delete(s.expirations, key)
	}
	
	var current int64
	if value, exists := s.strings[key]; exists {
		parsed, err := strconv.ParseInt(string(value), 10, 64)
		if err != nil {
			return 0, err
		}
		current = parsed
	}
	
	newValue := current + n
	s.strings[key] = []byte(strconv.FormatInt(newValue, 10))
	
	return newValue, nil
}

func (s *Store) DecrBy(ctx context.Context, key string, n int64) (int64, error) {
	return s.IncrBy(ctx, key, -n)
}

// Hash operations

func (s *Store) HSet(ctx context.Context, key string, field string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.isExpired(key) {
		s.deleteKeyUnsafe(key)
		delete(s.expirations, key)
	}
	
	if s.hashes[key] == nil {
		s.deleteKeyUnsafe(key) // Clear other data types
		s.hashes[key] = make(map[string][]byte)
	}
	
	s.hashes[key][field] = value
	return nil
}

func (s *Store) HGet(ctx context.Context, key string, field string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if s.isExpired(key) {
		s.mu.RUnlock()
		s.mu.Lock()
		s.deleteKeyUnsafe(key)
		delete(s.expirations, key)
		s.mu.Unlock()
		s.mu.RLock()
		return nil, kv.ErrNotFound
	}
	
	hash, exists := s.hashes[key]
	if !exists {
		return nil, kv.ErrNotFound
	}
	
	value, fieldExists := hash[field]
	if !fieldExists {
		return nil, kv.ErrNotFound
	}
	
	return value, nil
}

func (s *Store) HDel(ctx context.Context, key string, fields ...string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.isExpired(key) {
		s.deleteKeyUnsafe(key)
		delete(s.expirations, key)
		return 0, nil
	}
	
	hash, exists := s.hashes[key]
	if !exists {
		return 0, nil
	}
	
	var deleted int64
	for _, field := range fields {
		if _, fieldExists := hash[field]; fieldExists {
			delete(hash, field)
			deleted++
		}
	}
	
	// Remove key if hash is empty
	if len(hash) == 0 {
		delete(s.hashes, key)
	}
	
	return deleted, nil
}

func (s *Store) HGetAll(ctx context.Context, key string) (map[string][]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if s.isExpired(key) {
		s.mu.RUnlock()
		s.mu.Lock()
		s.deleteKeyUnsafe(key)
		delete(s.expirations, key)
		s.mu.Unlock()
		s.mu.RLock()
		return nil, kv.ErrNotFound
	}
	
	hash, exists := s.hashes[key]
	if !exists {
		return nil, kv.ErrNotFound
	}
	
	result := make(map[string][]byte, len(hash))
	for field, value := range hash {
		result[field] = value
	}
	
	return result, nil
}

// Set operations

func (s *Store) SAdd(ctx context.Context, key string, members ...[]byte) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.isExpired(key) {
		s.deleteKeyUnsafe(key)
		delete(s.expirations, key)
	}
	
	if s.sets[key] == nil {
		s.deleteKeyUnsafe(key) // Clear other data types
		s.sets[key] = make(map[string]struct{})
	}
	
	var added int64
	for _, member := range members {
		memberStr := string(member)
		if _, exists := s.sets[key][memberStr]; !exists {
			s.sets[key][memberStr] = struct{}{}
			added++
		}
	}
	
	return added, nil
}

func (s *Store) SRem(ctx context.Context, key string, members ...[]byte) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.isExpired(key) {
		s.deleteKeyUnsafe(key)
		delete(s.expirations, key)
		return 0, nil
	}
	
	set, exists := s.sets[key]
	if !exists {
		return 0, nil
	}
	
	var removed int64
	for _, member := range members {
		memberStr := string(member)
		if _, memberExists := set[memberStr]; memberExists {
			delete(set, memberStr)
			removed++
		}
	}
	
	// Remove key if set is empty
	if len(set) == 0 {
		delete(s.sets, key)
	}
	
	return removed, nil
}

func (s *Store) SMembers(ctx context.Context, key string) ([][]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if s.isExpired(key) {
		s.mu.RUnlock()
		s.mu.Lock()
		s.deleteKeyUnsafe(key)
		delete(s.expirations, key)
		s.mu.Unlock()
		s.mu.RLock()
		return nil, kv.ErrNotFound
	}
	
	set, exists := s.sets[key]
	if !exists {
		return nil, kv.ErrNotFound
	}
	
	members := make([][]byte, 0, len(set))
	for member := range set {
		members = append(members, []byte(member))
	}
	
	return members, nil
}

func (s *Store) SIsMember(ctx context.Context, key string, member []byte) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if s.isExpired(key) {
		s.mu.RUnlock()
		s.mu.Lock()
		s.deleteKeyUnsafe(key)
		delete(s.expirations, key)
		s.mu.Unlock()
		s.mu.RLock()
		return false, nil
	}
	
	set, exists := s.sets[key]
	if !exists {
		return false, nil
	}
	
	_, isMember := set[string(member)]
	return isMember, nil
}

// List operations

func (s *Store) LPush(ctx context.Context, key string, values ...[]byte) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.isExpired(key) {
		s.deleteKeyUnsafe(key)
		delete(s.expirations, key)
	}
	
	if s.lists[key] == nil {
		s.deleteKeyUnsafe(key) // Clear other data types
		s.lists[key] = make([][]byte, 0)
	}
	
	// Prepend values in order (each value becomes the new head)
	for _, value := range values {
		s.lists[key] = append([][]byte{value}, s.lists[key]...)
	}
	
	return int64(len(s.lists[key])), nil
}

func (s *Store) RPush(ctx context.Context, key string, values ...[]byte) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.isExpired(key) {
		s.deleteKeyUnsafe(key)
		delete(s.expirations, key)
	}
	
	if s.lists[key] == nil {
		s.deleteKeyUnsafe(key) // Clear other data types
		s.lists[key] = make([][]byte, 0)
	}
	
	s.lists[key] = append(s.lists[key], values...)
	return int64(len(s.lists[key])), nil
}

func (s *Store) LPop(ctx context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.isExpired(key) {
		s.deleteKeyUnsafe(key)
		delete(s.expirations, key)
		return nil, kv.ErrNotFound
	}
	
	list, exists := s.lists[key]
	if !exists || len(list) == 0 {
		return nil, kv.ErrNotFound
	}
	
	value := list[0]
	s.lists[key] = list[1:]
	
	// Remove key if list is empty
	if len(s.lists[key]) == 0 {
		delete(s.lists, key)
	}
	
	return value, nil
}

func (s *Store) RPop(ctx context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.isExpired(key) {
		s.deleteKeyUnsafe(key)
		delete(s.expirations, key)
		return nil, kv.ErrNotFound
	}
	
	list, exists := s.lists[key]
	if !exists || len(list) == 0 {
		return nil, kv.ErrNotFound
	}
	
	lastIndex := len(list) - 1
	value := list[lastIndex]
	s.lists[key] = list[:lastIndex]
	
	// Remove key if list is empty
	if len(s.lists[key]) == 0 {
		delete(s.lists, key)
	}
	
	return value, nil
}

func (s *Store) LRange(ctx context.Context, key string, start, stop int64) ([][]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if s.isExpired(key) {
		s.mu.RUnlock()
		s.mu.Lock()
		s.deleteKeyUnsafe(key)
		delete(s.expirations, key)
		s.mu.Unlock()
		s.mu.RLock()
		return nil, kv.ErrNotFound
	}
	
	list, exists := s.lists[key]
	if !exists {
		return nil, kv.ErrNotFound
	}
	
	listLen := int64(len(list))
	if listLen == 0 {
		return [][]byte{}, nil
	}
	
	// Handle negative indices
	if start < 0 {
		start = listLen + start
	}
	if stop < 0 {
		stop = listLen + stop
	}
	
	// Clamp to bounds
	if start < 0 {
		start = 0
	}
	if stop >= listLen {
		stop = listLen - 1
	}
	
	// Check if range is valid
	if start > stop || start >= listLen {
		return [][]byte{}, nil
	}
	
	result := make([][]byte, stop-start+1)
	for i := start; i <= stop; i++ {
		result[i-start] = list[i]
	}
	
	return result, nil
}

// Multi operations

func (s *Store) MGet(ctx context.Context, keys ...string) ([][]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	result := make([][]byte, len(keys))
	for i, key := range keys {
		if s.isExpired(key) {
			result[i] = nil
			continue
		}
		
		if value, exists := s.strings[key]; exists {
			result[i] = value
		} else {
			result[i] = nil
		}
	}
	
	return result, nil
}

func (s *Store) MSet(ctx context.Context, kv map[string][]byte, ttl ...time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	var expiration time.Duration
	if len(ttl) > 0 && ttl[0] > 0 {
		expiration = ttl[0]
	}
	
	for key, value := range kv {
		s.deleteKeyUnsafe(key)
		s.strings[key] = value
		
		if expiration > 0 {
			s.setExpiration(key, expiration)
		}
	}
	
	return nil
}

// Ping always returns nil for the in-memory store (always available)
func (s *Store) Ping(ctx context.Context) error {
	return nil
}

// Close stops the background janitor and cleans up resources
func (s *Store) Close() error {
	if s.janitorInterval > 0 {
		close(s.janitorStop)
		<-s.janitorDone
	}
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Clear all data
	s.strings = make(map[string][]byte)
	s.hashes = make(map[string]map[string][]byte)
	s.sets = make(map[string]map[string]struct{})
	s.lists = make(map[string][][]byte)
	s.expirations = make(map[string]time.Time)
	
	return nil
}