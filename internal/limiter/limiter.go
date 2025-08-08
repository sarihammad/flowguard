package limiter

import (
	"context"
	"fmt"
	"time"

	"rate-limiting-gateway/internal/storage"
	"go.uber.org/zap"
)

// RateLimiter handles rate limiting logic
type RateLimiter struct {
	redis  *storage.RedisClient
	logger *zap.Logger
	config RateLimitConfig
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	RequestsPerMinute int
	RequestsPerHour   int
	RequestsPerDay    int
	MonthlyQuota      int
	WindowSize        time.Duration
}

// RateLimitResult represents the result of a rate limit check
type RateLimitResult struct {
	Allowed    bool
	Remaining  int
	ResetTime  time.Time
	Limit      int
	Window     string
	QuotaUsed  int
	QuotaLimit int
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(redis *storage.RedisClient, config RateLimitConfig, logger *zap.Logger) *RateLimiter {
	return &RateLimiter{
		redis:  redis,
		logger: logger,
		config: config,
	}
}

// CheckRateLimit checks if a request is allowed based on rate limits
func (r *RateLimiter) CheckRateLimit(ctx context.Context, apiKey string) (*RateLimitResult, error) {
	now := time.Now()
	
	// Check monthly quota first
	quotaUsed, err := r.redis.GetMonthlyQuota(ctx, apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get monthly quota: %w", err)
	}

	if quotaUsed >= r.config.MonthlyQuota {
		r.logger.Warn("Monthly quota exceeded",
			zap.String("api_key", apiKey),
			zap.Int("quota_used", quotaUsed),
			zap.Int("quota_limit", r.config.MonthlyQuota),
		)
		return &RateLimitResult{
			Allowed:    false,
			Remaining:  0,
			ResetTime:  time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location()),
			Limit:      0,
			Window:     "monthly",
			QuotaUsed:  quotaUsed,
			QuotaLimit: r.config.MonthlyQuota,
		}, nil
	}

	// Check different time windows
	windows := []struct {
		name   string
		limit  int
		window time.Duration
	}{
		{"minute", r.config.RequestsPerMinute, time.Minute},
		{"hour", r.config.RequestsPerHour, time.Hour},
		{"day", r.config.RequestsPerDay, 24 * time.Hour},
	}

	for _, w := range windows {
		windowKey := r.getWindowKey(now, w.window)
		current, err := r.redis.GetRateLimit(ctx, apiKey, windowKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get rate limit for %s window: %w", w.name, err)
		}

		if current >= w.limit {
			resetTime := r.getWindowResetTime(now, w.window)
			r.logger.Warn("Rate limit exceeded",
				zap.String("api_key", apiKey),
				zap.String("window", w.name),
				zap.Int("current", current),
				zap.Int("limit", w.limit),
				zap.Time("reset_time", resetTime),
			)
			return &RateLimitResult{
				Allowed:    false,
				Remaining:  0,
				ResetTime:  resetTime,
				Limit:      w.limit,
				Window:     w.name,
				QuotaUsed:  quotaUsed,
				QuotaLimit: r.config.MonthlyQuota,
			}, nil
		}
	}

	// All checks passed
	return &RateLimitResult{
		Allowed:    true,
		Remaining:  r.config.RequestsPerMinute, // We'll update this after incrementing
		ResetTime:  r.getWindowResetTime(now, time.Minute),
		Limit:      r.config.RequestsPerMinute,
		Window:     "minute",
		QuotaUsed:  quotaUsed,
		QuotaLimit: r.config.MonthlyQuota,
	}, nil
}

// IncrementRateLimit increments the rate limit counters for all windows
func (r *RateLimiter) IncrementRateLimit(ctx context.Context, apiKey string) error {
	now := time.Now()
	
	// Increment monthly quota
	_, err := r.redis.IncrementMonthlyQuota(ctx, apiKey, r.config.MonthlyQuota)
	if err != nil {
		return fmt.Errorf("failed to increment monthly quota: %w", err)
	}

	// Increment rate limit for all windows
	windows := []struct {
		name   string
		limit  int
		window time.Duration
	}{
		{"minute", r.config.RequestsPerMinute, time.Minute},
		{"hour", r.config.RequestsPerHour, time.Hour},
		{"day", r.config.RequestsPerDay, 24 * time.Hour},
	}

	for _, w := range windows {
		windowKey := r.getWindowKey(now, w.window)
		current, err := r.redis.IncrementRateLimit(ctx, apiKey, windowKey, w.limit)
		if err != nil {
			return fmt.Errorf("failed to increment rate limit for %s window: %w", w.name, err)
		}

		r.logger.Debug("Rate limit incremented",
			zap.String("api_key", apiKey),
			zap.String("window", w.name),
			zap.Int("current", current),
			zap.Int("limit", w.limit),
		)
	}

	return nil
}

// getWindowKey generates a window key based on the current time and window duration
func (r *RateLimiter) getWindowKey(now time.Time, window time.Duration) string {
	switch window {
	case time.Minute:
		return fmt.Sprintf("%d-%02d-%02d-%02d-%02d", now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute())
	case time.Hour:
		return fmt.Sprintf("%d-%02d-%02d-%02d", now.Year(), now.Month(), now.Day(), now.Hour())
	case 24 * time.Hour:
		return fmt.Sprintf("%d-%02d-%02d", now.Year(), now.Month(), now.Day())
	default:
		return fmt.Sprintf("%d", now.Unix()/int64(window.Seconds()))
	}
}

// getWindowResetTime calculates when the current window will reset
func (r *RateLimiter) getWindowResetTime(now time.Time, window time.Duration) time.Time {
	switch window {
	case time.Minute:
		return time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute()+1, 0, 0, now.Location())
	case time.Hour:
		return time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, 0, 0, 0, now.Location())
	case 24 * time.Hour:
		return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	default:
		return now.Add(window)
	}
}

// GetRateLimitInfo gets current rate limit information for an API key
func (r *RateLimiter) GetRateLimitInfo(ctx context.Context, apiKey string) (map[string]interface{}, error) {
	now := time.Now()
	
	// Get monthly quota
	quotaUsed, err := r.redis.GetMonthlyQuota(ctx, apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get monthly quota: %w", err)
	}

	// Get rate limits for all windows
	windows := []struct {
		name   string
		limit  int
		window time.Duration
	}{
		{"minute", r.config.RequestsPerMinute, time.Minute},
		{"hour", r.config.RequestsPerHour, time.Hour},
		{"day", r.config.RequestsPerDay, 24 * time.Hour},
	}

	result := map[string]interface{}{
		"monthly_quota": map[string]interface{}{
			"used":  quotaUsed,
			"limit": r.config.MonthlyQuota,
		},
		"rate_limits": make(map[string]interface{}),
	}

	for _, w := range windows {
		windowKey := r.getWindowKey(now, w.window)
		current, err := r.redis.GetRateLimit(ctx, apiKey, windowKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get rate limit for %s window: %w", w.name, err)
		}

		result["rate_limits"].(map[string]interface{})[w.name] = map[string]interface{}{
			"current":    current,
			"limit":      w.limit,
			"remaining":  w.limit - current,
			"reset_time": r.getWindowResetTime(now, w.window),
		}
	}

	return result, nil
} 