package limiter

import "context"

// RateLimiterInterface defines the interface for rate limiting operations
type RateLimiterInterface interface {
	CheckRateLimit(ctx context.Context, apiKey string) (*RateLimitResult, error)
	IncrementRateLimit(ctx context.Context, apiKey string) error
	GetRateLimitInfo(ctx context.Context, apiKey string) (map[string]interface{}, error)
} 