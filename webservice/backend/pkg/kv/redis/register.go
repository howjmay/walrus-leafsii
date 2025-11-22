package redis

import (
	"fmt"

	"github.com/leafsii/leafsii-backend/pkg/kv"
)

func init() {
	kv.RegisterBackend(kv.BackendRedis, func(cfg kv.Config) (kv.Store, error) {
		if cfg.RedisURL == "" {
			return nil, fmt.Errorf("redis URL is required when backend is 'redis'")
		}
		return New(cfg.RedisURL)
	})
}

// NewStore creates a new Redis-backed store
func NewStore(redisURL string) (kv.Store, error) {
	return New(redisURL)
}