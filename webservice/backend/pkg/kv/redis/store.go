package redis

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/leafsii/leafsii-backend/pkg/kv"
)

// Store is a Redis-backed implementation of the kv.Store interface
type Store struct {
	client *redis.Client
}

// IsConnectionError checks if an error is a connection-related error that should trigger failover
func IsConnectionError(err error) bool {
	if err == nil {
		return false
	}
	
	// Don't treat redis.Nil as a connection error (it means "key not found")
	if err == redis.Nil {
		return false
	}
	
	// Context cancellation by caller should not trigger failover
	if errors.Is(err, context.Canceled) {
		return false
	}
	
	// Check for various network/connection errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	
	// Check for syscall connection errors
	var sysErr syscall.Errno
	if errors.As(err, &sysErr) {
		switch sysErr {
		case syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ECONNABORTED, syscall.ETIMEDOUT:
			return true
		}
	}
	
	// Check error message for common connection issues
	errStr := err.Error()
	connectionErrors := []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"no such host",
		"network is unreachable",
		"timeout",
		"connection closed",
		"EOF",
	}
	
	for _, connErr := range connectionErrors {
		if contains(errStr, connErr) {
			return true
		}
	}
	
	return false
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (s == substr || 
		    (len(s) > len(substr) && 
			 findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// wrapConnectionError wraps connection errors with ErrBackendUnavailable
func (s *Store) wrapConnectionError(err error) error {
	if err == nil {
		return nil
	}
	if IsConnectionError(err) {
		return fmt.Errorf("%w: %v", kv.ErrBackendUnavailable, err)
	}
	return err
}

// New creates a new Redis-backed store
func New(redisURL string) (*Store, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		// Fallback for simple address format
		u, parseErr := url.Parse("redis://" + redisURL)
		if parseErr != nil {
			return nil, err // Return original error
		}
		
		db := 0
		if u.Path != "" && u.Path != "/" {
			if dbNum, dbErr := strconv.Atoi(u.Path[1:]); dbErr == nil {
				db = dbNum
			}
		}
		
		opt = &redis.Options{
			Addr:     u.Host,
			Password: "",
			DB:       db,
		}
		
		if u.User != nil {
			if password, hasPassword := u.User.Password(); hasPassword {
				opt.Password = password
			}
		}
	}
	
	client := redis.NewClient(opt)
	
	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, err
	}
	
	return &Store{client: client}, nil
}

// String operations

func (s *Store) Set(ctx context.Context, key string, value []byte, ttl ...time.Duration) error {
	var expiration time.Duration
	if len(ttl) > 0 {
		expiration = ttl[0]
	}
	return s.wrapConnectionError(s.client.Set(ctx, key, value, expiration).Err())
}

func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	result, err := s.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, kv.ErrNotFound
		}
		return nil, s.wrapConnectionError(err)
	}
	return []byte(result), nil
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
	return s.client.Del(ctx, keys...).Result()
}

func (s *Store) Exists(ctx context.Context, keys ...string) (int64, error) {
	return s.client.Exists(ctx, keys...).Result()
}

func (s *Store) Expire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return s.client.Expire(ctx, key, ttl).Result()
}

func (s *Store) TTL(ctx context.Context, key string) (time.Duration, error) {
	ttl, err := s.client.TTL(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	
	// Redis returns -2 for non-existent keys
	if ttl == -2*time.Second {
		return 0, kv.ErrNotFound
	}
	
	return ttl, nil
}

// Counter operations

func (s *Store) IncrBy(ctx context.Context, key string, n int64) (int64, error) {
	return s.client.IncrBy(ctx, key, n).Result()
}

func (s *Store) DecrBy(ctx context.Context, key string, n int64) (int64, error) {
	return s.client.DecrBy(ctx, key, n).Result()
}

// Hash operations

func (s *Store) HSet(ctx context.Context, key string, field string, value []byte) error {
	return s.client.HSet(ctx, key, field, value).Err()
}

func (s *Store) HGet(ctx context.Context, key string, field string) ([]byte, error) {
	result, err := s.client.HGet(ctx, key, field).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, kv.ErrNotFound
		}
		return nil, err
	}
	return []byte(result), nil
}

func (s *Store) HDel(ctx context.Context, key string, fields ...string) (int64, error) {
	return s.client.HDel(ctx, key, fields...).Result()
}

func (s *Store) HGetAll(ctx context.Context, key string) (map[string][]byte, error) {
	result, err := s.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	
	if len(result) == 0 {
		// Check if key exists to distinguish between empty hash and non-existent key
		exists, err := s.client.Exists(ctx, key).Result()
		if err != nil {
			return nil, err
		}
		if exists == 0 {
			return nil, kv.ErrNotFound
		}
	}
	
	byteMap := make(map[string][]byte, len(result))
	for field, value := range result {
		byteMap[field] = []byte(value)
	}
	
	return byteMap, nil
}

// Set operations

func (s *Store) SAdd(ctx context.Context, key string, members ...[]byte) (int64, error) {
	interfaces := make([]interface{}, len(members))
	for i, member := range members {
		interfaces[i] = member
	}
	return s.client.SAdd(ctx, key, interfaces...).Result()
}

func (s *Store) SRem(ctx context.Context, key string, members ...[]byte) (int64, error) {
	interfaces := make([]interface{}, len(members))
	for i, member := range members {
		interfaces[i] = member
	}
	return s.client.SRem(ctx, key, interfaces...).Result()
}

func (s *Store) SMembers(ctx context.Context, key string) ([][]byte, error) {
	result, err := s.client.SMembers(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	
	if len(result) == 0 {
		// Check if key exists to distinguish between empty set and non-existent key
		exists, err := s.client.Exists(ctx, key).Result()
		if err != nil {
			return nil, err
		}
		if exists == 0 {
			return nil, kv.ErrNotFound
		}
	}
	
	members := make([][]byte, len(result))
	for i, member := range result {
		members[i] = []byte(member)
	}
	
	return members, nil
}

func (s *Store) SIsMember(ctx context.Context, key string, member []byte) (bool, error) {
	return s.client.SIsMember(ctx, key, member).Result()
}

// List operations

func (s *Store) LPush(ctx context.Context, key string, values ...[]byte) (int64, error) {
	interfaces := make([]interface{}, len(values))
	for i, value := range values {
		interfaces[i] = value
	}
	return s.client.LPush(ctx, key, interfaces...).Result()
}

func (s *Store) RPush(ctx context.Context, key string, values ...[]byte) (int64, error) {
	interfaces := make([]interface{}, len(values))
	for i, value := range values {
		interfaces[i] = value
	}
	return s.client.RPush(ctx, key, interfaces...).Result()
}

func (s *Store) LPop(ctx context.Context, key string) ([]byte, error) {
	result, err := s.client.LPop(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, kv.ErrNotFound
		}
		return nil, err
	}
	return []byte(result), nil
}

func (s *Store) RPop(ctx context.Context, key string) ([]byte, error) {
	result, err := s.client.RPop(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, kv.ErrNotFound
		}
		return nil, err
	}
	return []byte(result), nil
}

func (s *Store) LRange(ctx context.Context, key string, start, stop int64) ([][]byte, error) {
	result, err := s.client.LRange(ctx, key, start, stop).Result()
	if err != nil {
		return nil, err
	}
	
	if len(result) == 0 {
		// Check if key exists to distinguish between empty range and non-existent key
		exists, err := s.client.Exists(ctx, key).Result()
		if err != nil {
			return nil, err
		}
		if exists == 0 {
			return nil, kv.ErrNotFound
		}
	}
	
	values := make([][]byte, len(result))
	for i, value := range result {
		values[i] = []byte(value)
	}
	
	return values, nil
}

// Multi operations

func (s *Store) MGet(ctx context.Context, keys ...string) ([][]byte, error) {
	result, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	
	values := make([][]byte, len(result))
	for i, value := range result {
		if value != nil {
			if str, ok := value.(string); ok {
				values[i] = []byte(str)
			}
		}
		// nil values remain nil (representing missing keys)
	}
	
	return values, nil
}

func (s *Store) MSet(ctx context.Context, kv map[string][]byte, ttl ...time.Duration) error {
	// For MSet with TTL, we need to use a pipeline since Redis MSET doesn't support TTL
	if len(ttl) > 0 && ttl[0] > 0 {
		pipe := s.client.Pipeline()
		
		for key, value := range kv {
			pipe.Set(context.Background(), key, value, ttl[0])
		}
		
		_, err := pipe.Exec(ctx)
		return err
	}
	
	// Convert to interface map for Redis client
	values := make([]interface{}, 0, len(kv)*2)
	for key, value := range kv {
		values = append(values, key, value)
	}
	
	return s.client.MSet(ctx, values...).Err()
}

// Ping checks if Redis is reachable
func (s *Store) Ping(ctx context.Context) error {
	return s.wrapConnectionError(s.client.Ping(ctx).Err())
}

// Close closes the Redis connection
func (s *Store) Close() error {
	return s.client.Close()
}