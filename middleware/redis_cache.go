package middleware

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	// Cache key prefixes.
	icsPrefix  = "ics:"
	dataPrefix = "data:"

	// Cache TTLs.
	icsCacheTTL  = time.Hour        // ICS files expire after 1 hour
	dataCacheTTL = 30 * time.Minute // General data cache expires after 30 minutes
)

// RedisCache wraps the Redis client for caching operations.
type RedisCache struct {
	client *redis.Client
	ctx    context.Context
	logger *zap.Logger
}

// NewRedisCache creates a new Redis cache client.
func NewRedisCache(ctx context.Context, logger *zap.Logger) (*RedisCache, error) {
	// Get Redis address from environment variable, default to redis service in Docker network
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "redis:6379"
	}

	// Get Redis password from environment variable (optional)
	redisPassword := os.Getenv("REDIS_PASSWORD")

	// Get Redis DB number from environment variable, default to 0
	redisDB := 0

	//nolint:exhaustruct // Other Redis options use defaults
	client := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       redisDB,
	})

	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logger.Info("Redis cache initialized",
		zap.String("addr", redisAddr),
		zap.Int("db", redisDB))

	return &RedisCache{
		client: client,
		ctx:    ctx,
		logger: logger,
	}, nil
}

// GetICS retrieves an ICS file from cache.
func (c *RedisCache) GetICS(hash string) (string, bool) {
	key := icsPrefix + hash
	val, err := c.client.Get(c.ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		c.logger.Debug("ICS cache miss", zap.String("hash", hash))
		return "", false
	}
	if err != nil {
		c.logger.Error("Failed to get ICS from cache", zap.Error(err), zap.String("hash", hash))
		return "", false
	}

	c.logger.Debug("ICS cache hit", zap.String("hash", hash))
	return val, true
}

// SetICS stores an ICS file in cache with TTL.
func (c *RedisCache) SetICS(hash string, content string) error {
	key := icsPrefix + hash
	err := c.client.Set(c.ctx, key, content, icsCacheTTL).Err()
	if err != nil {
		c.logger.Error("Failed to set ICS in cache", zap.Error(err), zap.String("hash", hash))
		return err
	}

	c.logger.Debug("ICS cached", zap.String("hash", hash), zap.Duration("ttl", icsCacheTTL))
	return nil
}

// DeleteICS removes a specific ICS file from cache.
func (c *RedisCache) DeleteICS(hash string) error {
	key := icsPrefix + hash
	err := c.client.Del(c.ctx, key).Err()
	if err != nil {
		c.logger.Error("Failed to delete ICS from cache", zap.Error(err), zap.String("hash", hash))
		return err
	}

	c.logger.Info("ICS cache entry deleted", zap.String("hash", hash))
	return nil
}

// GetData retrieves general data from cache (for games, leagues, teams, etc.).
func (c *RedisCache) GetData(key string) (string, bool) {
	fullKey := dataPrefix + key
	val, err := c.client.Get(c.ctx, fullKey).Result()
	if errors.Is(err, redis.Nil) {
		c.logger.Debug("Data cache miss", zap.String("key", key))
		return "", false
	}
	if err != nil {
		c.logger.Error("Failed to get data from cache", zap.Error(err), zap.String("key", key))
		return "", false
	}

	c.logger.Debug("Data cache hit", zap.String("key", key))
	return val, true
}

// SetData stores general data in cache with TTL.
func (c *RedisCache) SetData(key string, value string) error {
	fullKey := dataPrefix + key
	err := c.client.Set(c.ctx, fullKey, value, dataCacheTTL).Err()
	if err != nil {
		c.logger.Error("Failed to set data in cache", zap.Error(err), zap.String("key", key))
		return err
	}

	c.logger.Debug("Data cached", zap.String("key", key), zap.Duration("ttl", dataCacheTTL))
	return nil
}

// GetBytes retrieves binary data from cache.
func (c *RedisCache) GetBytes(key string) ([]byte, bool) {
	fullKey := dataPrefix + key
	val, err := c.client.Get(c.ctx, fullKey).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false
	}
	if err != nil {
		c.logger.Error("Failed to get bytes from cache", zap.Error(err), zap.String("key", key))
		return nil, false
	}
	return val, true
}

// SetBytes stores binary data in cache with TTL.
func (c *RedisCache) SetBytes(key string, value []byte) error {
	fullKey := dataPrefix + key
	err := c.client.Set(c.ctx, fullKey, value, dataCacheTTL).Err()
	if err != nil {
		c.logger.Error("Failed to set bytes in cache", zap.Error(err), zap.String("key", key))
		return err
	}
	return nil
}

// Clear removes all cache entries (for this application).
func (c *RedisCache) Clear() error {
	// Delete all keys with our prefixes
	iter := c.client.Scan(c.ctx, 0, icsPrefix+"*", 0).Iterator()
	for iter.Next(c.ctx) {
		if err := c.client.Del(c.ctx, iter.Val()).Err(); err != nil {
			c.logger.Warn("Failed to delete ICS cache key", zap.Error(err), zap.String("key", iter.Val()))
		}
	}
	if err := iter.Err(); err != nil {
		return err
	}

	iter = c.client.Scan(c.ctx, 0, dataPrefix+"*", 0).Iterator()
	for iter.Next(c.ctx) {
		if err := c.client.Del(c.ctx, iter.Val()).Err(); err != nil {
			c.logger.Warn("Failed to delete data cache key", zap.Error(err), zap.String("key", iter.Val()))
		}
	}
	if err := iter.Err(); err != nil {
		return err
	}

	c.logger.Info("Cache cleared")
	return nil
}

// Close closes the Redis connection.
func (c *RedisCache) Close() error {
	if err := c.client.Close(); err != nil {
		c.logger.Error("Failed to close Redis client", zap.Error(err))
		return err
	}
	c.logger.Info("Redis connection closed")
	return nil
}

// GetCacheStats returns cache statistics.
func (c *RedisCache) GetCacheStats() (map[string]int64, error) {
	stats := make(map[string]int64)

	// Count ICS cache entries
	icsCount := int64(0)
	iter := c.client.Scan(c.ctx, 0, icsPrefix+"*", 0).Iterator()
	for iter.Next(c.ctx) {
		icsCount++
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	stats["ics_entries"] = icsCount

	// Count data cache entries
	dataCount := int64(0)
	iter = c.client.Scan(c.ctx, 0, dataPrefix+"*", 0).Iterator()
	for iter.Next(c.ctx) {
		dataCount++
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	stats["data_entries"] = dataCount

	return stats, nil
}
