package kv

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a key or field is not found
var ErrNotFound = errors.New("not found")

// ErrBackendUnavailable is returned when the backend storage is unavailable
var ErrBackendUnavailable = errors.New("backend unavailable")

// Store defines the interface for a Redis-like key-value store
type Store interface {
	// String operations
	Set(ctx context.Context, key string, value []byte, ttl ...time.Duration) error
	Get(ctx context.Context, key string) ([]byte, error)
	SetString(ctx context.Context, key string, value string, ttl ...time.Duration) error
	GetString(ctx context.Context, key string) (string, error)
	
	// Key operations
	Del(ctx context.Context, keys ...string) (int64, error)
	Exists(ctx context.Context, keys ...string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) (bool, error)
	TTL(ctx context.Context, key string) (time.Duration, error)
	
	// Counter operations
	IncrBy(ctx context.Context, key string, n int64) (int64, error)
	DecrBy(ctx context.Context, key string, n int64) (int64, error)
	
	// Hash operations
	HSet(ctx context.Context, key string, field string, value []byte) error
	HGet(ctx context.Context, key string, field string) ([]byte, error)
	HDel(ctx context.Context, key string, fields ...string) (int64, error)
	HGetAll(ctx context.Context, key string) (map[string][]byte, error)
	
	// Set operations
	SAdd(ctx context.Context, key string, members ...[]byte) (int64, error)
	SRem(ctx context.Context, key string, members ...[]byte) (int64, error)
	SMembers(ctx context.Context, key string) ([][]byte, error)
	SIsMember(ctx context.Context, key string, member []byte) (bool, error)
	
	// List operations
	LPush(ctx context.Context, key string, values ...[]byte) (int64, error)
	RPush(ctx context.Context, key string, values ...[]byte) (int64, error)
	LPop(ctx context.Context, key string) ([]byte, error)
	RPop(ctx context.Context, key string) ([]byte, error)
	LRange(ctx context.Context, key string, start, stop int64) ([][]byte, error)
	
	// Multi operations
	MGet(ctx context.Context, keys ...string) ([][]byte, error)
	MSet(ctx context.Context, kv map[string][]byte, ttl ...time.Duration) error
	
	// Health check
	Ping(ctx context.Context) error
	
	// Cleanup
	Close() error
}