package storage

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

// RedisClient wraps the Redis client with additional methods
type RedisClient struct {
	client *redis.Client
	logger *zap.Logger
}

// NewRedisClient creates a new Redis client
func NewRedisClient(addr, password string, db, poolSize int, logger *zap.Logger) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
		PoolSize: poolSize,
	})

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisClient{
		client: client,
		logger: logger,
	}, nil
}

// Close closes the Redis connection
func (r *RedisClient) Close() error {
	return r.client.Close()
}

// IncrementRateLimit increments the rate limit counter for a given API key and window
func (r *RedisClient) IncrementRateLimit(ctx context.Context, apiKey, window string, limit int) (int, error) {
	key := fmt.Sprintf("rate:%s:%s", apiKey, window)
	
	// Use Redis pipeline for atomic operations
	pipe := r.client.Pipeline()
	
	// Increment the counter
	incr := pipe.Incr(ctx, key)
	
	// Set expiration if key doesn't exist
	pipe.Expire(ctx, key, time.Hour)
	
	// Execute pipeline
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to increment rate limit: %w", err)
	}

	current := int(incr.Val())
	
	r.logger.Debug("Rate limit incremented",
		zap.String("api_key", apiKey),
		zap.String("window", window),
		zap.Int("current", current),
		zap.Int("limit", limit),
	)

	return current, nil
}

// GetRateLimit gets the current rate limit count for a given API key and window
func (r *RedisClient) GetRateLimit(ctx context.Context, apiKey, window string) (int, error) {
	key := fmt.Sprintf("rate:%s:%s", apiKey, window)
	
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get rate limit: %w", err)
	}

	count, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("failed to parse rate limit count: %w", err)
	}

	return count, nil
}

// IncrementMonthlyQuota increments the monthly quota counter for a given API key
func (r *RedisClient) IncrementMonthlyQuota(ctx context.Context, apiKey string, quota int) (int, error) {
	now := time.Now()
	monthKey := fmt.Sprintf("%d-%02d", now.Year(), now.Month())
	key := fmt.Sprintf("quota:%s:%s", apiKey, monthKey)
	
	// Use Redis pipeline for atomic operations
	pipe := r.client.Pipeline()
	
	// Increment the counter
	incr := pipe.Incr(ctx, key)
	
	// Set expiration to end of month
	expiration := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location()).Sub(now)
	pipe.Expire(ctx, key, expiration)
	
	// Execute pipeline
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to increment monthly quota: %w", err)
	}

	current := int(incr.Val())
	
	r.logger.Debug("Monthly quota incremented",
		zap.String("api_key", apiKey),
		zap.String("month", monthKey),
		zap.Int("current", current),
		zap.Int("quota", quota),
	)

	return current, nil
}

// GetMonthlyQuota gets the current monthly quota count for a given API key
func (r *RedisClient) GetMonthlyQuota(ctx context.Context, apiKey string) (int, error) {
	now := time.Now()
	monthKey := fmt.Sprintf("%d-%02d", now.Year(), now.Month())
	key := fmt.Sprintf("quota:%s:%s", apiKey, monthKey)
	
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get monthly quota: %w", err)
	}

	count, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("failed to parse monthly quota count: %w", err)
	}

	return count, nil
}

// ValidateAPIKey checks if an API key is valid
func (r *RedisClient) ValidateAPIKey(ctx context.Context, apiKey string) (bool, error) {
	// For now, we'll use a simple approach where any non-empty API key is valid
	// In production, you might want to check against a database or Redis set
	if apiKey == "" {
		return false, nil
	}

	// Optional: Check against a Redis set of valid API keys
	// key := fmt.Sprintf("valid_keys")
	// exists, err := r.client.SIsMember(ctx, key, apiKey).Result()
	// if err != nil {
	//     return false, fmt.Errorf("failed to validate API key: %w", err)
	// }
	// return exists, nil

	return true, nil
}

// AddValidAPIKey adds an API key to the valid keys set
func (r *RedisClient) AddValidAPIKey(ctx context.Context, apiKey string) error {
	key := "valid_keys"
	return r.client.SAdd(ctx, key, apiKey).Err()
}

// RemoveValidAPIKey removes an API key from the valid keys set
func (r *RedisClient) RemoveValidAPIKey(ctx context.Context, apiKey string) error {
	key := "valid_keys"
	return r.client.SRem(ctx, key, apiKey).Err()
}

// GetValidAPIKeys gets all valid API keys
func (r *RedisClient) GetValidAPIKeys(ctx context.Context) ([]string, error) {
	key := "valid_keys"
	return r.client.SMembers(ctx, key).Result()
} 