# Redis Failover Integration Guide

This guide shows how to replace your existing Redis cache setup with the resilient failover system.

## Quick Start

### 1. Replace your existing cache setup

**Before** (crashy):
```go
func setupCache() (*store.Cache, error) {
    cache, err := store.NewCache("127.0.0.1:6379", logger, metrics)
    if err != nil {
        return nil, fmt.Errorf("failed to setup cache: %w", err) // App crashes here!
    }
    return cache, nil
}
```

**After** (resilient):
```go
import (
    "github.com/leafsii/leafsii-backend/pkg/kv"
    // Import backends
    _ "github.com/leafsii/leafsii-backend/pkg/kv/memory"
    _ "github.com/leafsii/leafsii-backend/pkg/kv/redis"
)

func setupCache() (kv.Store, error) {
    cfg := kv.Config{
        Backend:              kv.BackendRedis,
        RedisURL:            "redis://127.0.0.1:6379/0",
        FailoverEnabled:     true,                // Enable automatic failover
        ProbeInterval:       5 * time.Second,    // Check Redis health every 5s
        StartupProbeTimeout: 1 * time.Second,    // Wait 1s for Redis at startup
        Logger:              yourLogger,         // Optional: log failover events
    }
    
    store, err := kv.NewStoreFromConfig(cfg)
    if err != nil {
        return nil, err // This should never fail now
    }
    
    return store, nil
}
```

### 2. Use the store exactly like before

```go
func (s *Service) CacheUserData(ctx context.Context, userID string, data *UserData) error {
    key := fmt.Sprintf("user:%s", userID)
    value, _ := json.Marshal(data)
    
    // This works whether Redis is available or not
    return s.store.Set(ctx, key, value, 10*time.Minute)
}

func (s *Service) GetCachedUserData(ctx context.Context, userID string) (*UserData, error) {
    key := fmt.Sprintf("user:%s", userID)
    value, err := s.store.Get(ctx, key)
    if errors.Is(err, kv.ErrNotFound) {
        return nil, nil // Cache miss
    }
    if err != nil {
        return nil, err
    }
    
    var data UserData
    if err := json.Unmarshal(value, &data); err != nil {
        return nil, err
    }
    
    return &data, nil
}
```

## Configuration Options

```go
type Config struct {
    Backend              Backend           // "memory" or "redis"
    RedisURL            string            // Redis connection string
    FailoverEnabled     bool              // Enable automatic failover (default: false)
    ProbeInterval       time.Duration     // Health check interval (default: 5s)
    StartupProbeTimeout time.Duration     // Startup health check timeout (default: 1s)
    JanitorInterval     time.Duration     // Memory cleanup interval (default: 30s)
    Logger              LogFunc           // Optional logging function
}
```

## Behavior

### Startup Behavior
- **Redis available**: Uses Redis with in-memory failover ready
- **Redis unavailable**: Logs warning, uses in-memory store immediately
- **No crash**: Application always starts successfully

### Runtime Behavior
- **Redis fails**: Automatically switches to in-memory store, logs once
- **Redis recovers**: Automatically switches back to Redis, logs once
- **Transparent**: Application code sees no difference
- **No data sync**: Memory and Redis data may diverge (accept for v1)

### Logging Output
```
[12:34:56] Redis healthy at startup; using Redis with in-memory failover
[12:45:23] Failing over to in-memory store (reason: primary_unavailable)
[12:47:15] Recovered to primary store (reason: primary_healthy)
```

## Migration Checklist

1. ✅ Add imports for kv package and backends
2. ✅ Replace `store.NewCache()` with `kv.NewStoreFromConfig()`
3. ✅ Update method calls (`cache.Get()` → `store.Get()`)
4. ✅ Handle `kv.ErrNotFound` instead of `store.ErrCacheMiss`
5. ✅ Update dependency injection to pass `kv.Store`
6. ✅ Set `FailoverEnabled: true` in production
7. ✅ Add optional logger for observability
8. ✅ Test that app starts when Redis is down

## Advanced Usage

### Custom Logger
```go
logger := func(msg string, fields ...any) {
    // Extract structured fields
    fieldsMap := make(map[string]any)
    for i := 0; i < len(fields); i += 2 {
        if i+1 < len(fields) {
            fieldsMap[fields[i].(string)] = fields[i+1]
        }
    }
    
    // Log with your preferred structured logger
    log.Info(msg, zap.Any("kv", fieldsMap))
}

cfg.Logger = logger
```

### Health Monitoring
```go
// Check which backend is currently active
if fs, ok := store.(interface{ GetActiveBackend() string }); ok {
    backend := fs.GetActiveBackend() // "primary" or "fallback"
    metrics.RecordActiveBackend(backend)
}

// Test store health
ctx, cancel := context.WithTimeout(context.Background(), time.Second)
defer cancel()
if err := store.Ping(ctx); err != nil {
    log.Warn("Store health check failed", zap.Error(err))
}
```

### Environment-based Configuration
```go
func configFromEnv() kv.Config {
    return kv.Config{
        Backend:              kv.BackendRedis,
        RedisURL:            os.Getenv("REDIS_URL"),
        FailoverEnabled:     os.Getenv("ENV") == "production",
        ProbeInterval:       parseEnvDuration("KV_PROBE_INTERVAL", 5*time.Second),
        StartupProbeTimeout: parseEnvDuration("KV_STARTUP_TIMEOUT", 1*time.Second),
        Logger:              setupLogger(),
    }
}
```

The key benefit: **Your application will never crash due to Redis being unavailable**. It gracefully degrades to in-memory storage and automatically recovers when Redis comes back online.