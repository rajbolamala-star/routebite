package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache stores JSON-serializable values behind stable keys. Implementations are
// best-effort: callers should continue to the real service when cache fails.
type Cache interface {
	Get(ctx context.Context, key string, dest any) (bool, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
}

// NoopCache keeps external caching optional for local development.
type NoopCache struct{}

func (NoopCache) Get(context.Context, string, any) (bool, error) {
	return false, nil
}

func (NoopCache) Set(context.Context, string, any, time.Duration) error {
	return nil
}

// RedisCache stores JSON values in Redis.
type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(client *redis.Client) *RedisCache {
	return &RedisCache{client: client}
}

func (c *RedisCache) Get(ctx context.Context, key string, dest any) (bool, error) {
	raw, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return false, err
	}
	return true, nil
}

func (c *RedisCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, raw, ttl).Err()
}
