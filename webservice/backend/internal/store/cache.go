package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/leafsii/leafsii-backend/internal/metrics"
	"github.com/leafsii/leafsii-backend/pkg/kv"
	memkv "github.com/leafsii/leafsii-backend/pkg/kv/memory"
	"go.uber.org/zap"
)

type Cache struct {
	// When Redis is available, use client for all operations
	client *redis.Client
	// When Redis is unavailable, fall back to an in-memory kv.Store
	kvStore kv.Store
	// In-memory pubsub hub for when Redis is unavailable
	pubsubHub *PubSubHub

	logger  *zap.SugaredLogger
	metrics *metrics.Metrics
}

func NewCache(addr string, logger *zap.SugaredLogger, metrics *metrics.Metrics) (*Cache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     "",
		DB:           0,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 5,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		// Redis unavailable: fall back to in-memory cache
		if logger != nil {
			logger.Warnw("Redis unavailable; using in-memory cache with mock pubsub", "error", err)
		}
		return &Cache{
			client:    nil,
			kvStore:   memkv.NewStore(),
			pubsubHub: NewPubSubHub(),
			logger:    logger,
			metrics:   metrics,
		}, nil
	}

	return &Cache{
		client:  client,
		logger:  logger,
		metrics: metrics,
	}, nil
}

// Cache key prefixes
const (
	KeyProtocolState = "fx:protocol:state"
	KeySPIndex       = "fx:sp:index"
	KeyOraclePrice   = "fx:oracle:price"
	KeyUserPosition  = "fx:user:position"
	KeyQuoteMint     = "fx:quotes:mint"
	KeyQuoteRedeem   = "fx:quotes:redeem"
	KeyQuoteStake    = "fx:quotes:stake"
)

func (c *Cache) Get(ctx context.Context, key string, dest interface{}) error {
	// Redis mode
	if c.client != nil {
		val, err := c.client.Get(ctx, key).Result()
		if err != nil {
			if err == redis.Nil {
				if c.metrics != nil {
					c.metrics.RecordCacheMiss(ctx, key)
				}
				return ErrCacheMiss
			}
			if c.logger != nil {
				c.logger.Errorw("Cache get error", "key", key, "error", err)
			}
			return fmt.Errorf("cache get error: %w", err)
		}
		if c.metrics != nil {
			c.metrics.RecordCacheHit(ctx, key)
		}
		if err := json.Unmarshal([]byte(val), dest); err != nil {
			return fmt.Errorf("cache unmarshal error: %w", err)
		}
		return nil
	}

	// In-memory mode via kv.Store
	data, err := c.kvStore.Get(ctx, key)
	if err != nil {
		if err == kv.ErrNotFound {
			if c.metrics != nil {
				c.metrics.RecordCacheMiss(ctx, key)
			}
			return ErrCacheMiss
		}
		return fmt.Errorf("cache get error: %w", err)
	}
	if c.metrics != nil {
		c.metrics.RecordCacheHit(ctx, key)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("cache unmarshal error: %w", err)
	}
	return nil
}

func (c *Cache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache marshal error: %w", err)
	}
	if c.client != nil {
		if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
			if c.logger != nil {
				c.logger.Errorw("Cache set error", "key", key, "error", err)
			}
			return fmt.Errorf("cache set error: %w", err)
		}
		return nil
	}
	if err := c.kvStore.Set(ctx, key, data, ttl); err != nil {
		return fmt.Errorf("cache set error: %w", err)
	}
	return nil
}

func (c *Cache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	if c.client != nil {
		if err := c.client.Del(ctx, keys...).Err(); err != nil {
			if c.logger != nil {
				c.logger.Errorw("Cache delete error", "keys", keys, "error", err)
			}
			return fmt.Errorf("cache delete error: %w", err)
		}
		return nil
	}
	if _, err := c.kvStore.Del(ctx, keys...); err != nil {
		return fmt.Errorf("cache delete error: %w", err)
	}
	return nil
}

func (c *Cache) Exists(ctx context.Context, key string) (bool, error) {
	if c.client != nil {
		count, err := c.client.Exists(ctx, key).Result()
		if err != nil {
			return false, fmt.Errorf("cache exists error: %w", err)
		}
		return count > 0, nil
	}
	count, err := c.kvStore.Exists(ctx, key)
	if err != nil {
		return false, fmt.Errorf("cache exists error: %w", err)
	}
	return count > 0, nil
}

// Specialized cache methods
func (c *Cache) GetProtocolState(ctx context.Context, dest interface{}) error {
	return c.Get(ctx, KeyProtocolState, dest)
}

func (c *Cache) SetProtocolState(ctx context.Context, value interface{}) error {
	return c.Set(ctx, KeyProtocolState, value, 3*time.Second)
}

func (c *Cache) GetSPIndex(ctx context.Context, dest interface{}) error {
	return c.Get(ctx, KeySPIndex, dest)
}

func (c *Cache) SetSPIndex(ctx context.Context, value interface{}) error {
	return c.Set(ctx, KeySPIndex, value, 2*time.Second)
}

func (c *Cache) GetUserPosition(ctx context.Context, address string, dest interface{}) error {
	key := fmt.Sprintf("%s:%s", KeyUserPosition, address)
	return c.Get(ctx, key, dest)
}

func (c *Cache) SetUserPosition(ctx context.Context, address string, value interface{}) error {
	key := fmt.Sprintf("%s:%s", KeyUserPosition, address)
	return c.Set(ctx, key, value, 10*time.Second)
}

func (c *Cache) GetOraclePrice(ctx context.Context, symbol string, dest interface{}) error {
	key := fmt.Sprintf("%s:%s", KeyOraclePrice, symbol)
	return c.Get(ctx, key, dest)
}

func (c *Cache) SetOraclePrice(ctx context.Context, symbol string, value interface{}, ttl time.Duration) error {
	key := fmt.Sprintf("%s:%s", KeyOraclePrice, symbol)
	return c.Set(ctx, key, value, ttl)
}

// Quote cache methods with unique keys
func (c *Cache) GetQuote(ctx context.Context, quoteType, quoteID string, dest interface{}) error {
	key := fmt.Sprintf("fx:quotes:%s:%s", quoteType, quoteID)
	return c.Get(ctx, key, dest)
}

func (c *Cache) SetQuote(ctx context.Context, quoteType, quoteID string, value interface{}, ttl time.Duration) error {
	key := fmt.Sprintf("fx:quotes:%s:%s", quoteType, quoteID)
	return c.Set(ctx, key, value, ttl)
}

// Pub/Sub methods for real-time updates
func (c *Cache) Publish(ctx context.Context, channel string, message interface{}) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("pubsub marshal error: %w", err)
	}

	if c.client != nil {
		// Redis mode
		if err := c.client.Publish(ctx, channel, data).Err(); err != nil {
			if c.logger != nil {
				c.logger.Errorw("Publish error", "channel", channel, "error", err)
			}
			return fmt.Errorf("pubsub publish error: %w", err)
		}
		return nil
	}

	// In-memory mode
	if c.pubsubHub != nil {
		c.pubsubHub.Publish(channel, string(data))
		if c.logger != nil {
			c.logger.Debugw("Published to in-memory pubsub", "channel", channel)
		}
	}
	return nil
}

func (c *Cache) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	if c.client != nil {
		// Redis mode
		return c.client.Subscribe(ctx, channels...)
	}

	// In-memory mode - return nil but the system should handle this gracefully
	if c.logger != nil {
		c.logger.Debugw("Redis unavailable; using in-memory cache - PubSub simulation active", "channels", channels)
	}
	return nil
}

// SubscribeInMemory subscribes to channels using the in-memory pubsub hub
// Returns a MockPubSub that can be used similarly to redis.PubSub
func (c *Cache) SubscribeInMemory(ctx context.Context, channels ...string) *MockPubSub {
	if c.pubsubHub != nil {
		return c.pubsubHub.Subscribe(ctx, channels...)
	}
	return nil
}

// IsInMemoryMode returns true if the cache is running in in-memory mode
func (c *Cache) IsInMemoryMode() bool {
	return c.client == nil
}

// Health check
func (c *Cache) Ping(ctx context.Context) error {
	if c.client != nil {
		return c.client.Ping(ctx).Err()
	}
	// In-memory mode considered healthy
	return nil
}

// Close connection
func (c *Cache) Close() error {
	var err error
	if c.client != nil {
		err = c.client.Close()
	}
	if c.kvStore != nil {
		// Close in-memory store if it has cleanup
		if closeErr := c.kvStore.Close(); err == nil {
			err = closeErr
		}
	}
	return err
}

// Error types
var (
	ErrCacheMiss = fmt.Errorf("cache miss")
)
