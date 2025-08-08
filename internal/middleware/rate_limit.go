package middleware

import (
	"net/http"
	"strconv"
	"time"

	"rate-limiting-gateway/internal/limiter"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RateLimitMiddleware handles rate limiting
type RateLimitMiddleware struct {
	rateLimiter limiter.RateLimiterInterface
	logger      *zap.Logger
}

// NewRateLimitMiddleware creates a new rate limiting middleware
func NewRateLimitMiddleware(rateLimiter limiter.RateLimiterInterface, logger *zap.Logger) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		rateLimiter: rateLimiter,
		logger:      logger,
	}
}

// RateLimit enforces rate limits for requests
func (r *RateLimitMiddleware) RateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := GetAPIKeyFromContext(c)
		if apiKey == "" {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "API key not found in context",
				"code":  "INTERNAL_ERROR",
			})
			c.Abort()
			return
		}

		// Check rate limits
		result, err := r.rateLimiter.CheckRateLimit(c.Request.Context(), apiKey)
		if err != nil {
			r.logger.Error("Failed to check rate limit",
				zap.String("api_key", maskAPIKey(apiKey)),
				zap.String("ip", c.ClientIP()),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Internal server error",
				"code":  "RATE_LIMIT_ERROR",
			})
			c.Abort()
			return
		}

		if !result.Allowed {
			// Set rate limit headers
			c.Header("X-RateLimit-Limit", strconv.Itoa(result.Limit))
			c.Header("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
			c.Header("X-RateLimit-Reset", result.ResetTime.Format(time.RFC3339))
			c.Header("X-RateLimit-Window", result.Window)
			c.Header("X-RateLimit-QuotaUsed", strconv.Itoa(result.QuotaUsed))
			c.Header("X-RateLimit-QuotaLimit", strconv.Itoa(result.QuotaLimit))

			// Return appropriate error response
			var statusCode int
			var errorCode string
			var errorMessage string

			if result.Window == "monthly" {
				statusCode = http.StatusTooManyRequests
				errorCode = "MONTHLY_QUOTA_EXCEEDED"
				errorMessage = "Monthly quota exceeded"
			} else {
				statusCode = http.StatusTooManyRequests
				errorCode = "RATE_LIMIT_EXCEEDED"
				errorMessage = "Rate limit exceeded"
			}

			r.logger.Warn("Rate limit exceeded",
				zap.String("api_key", maskAPIKey(apiKey)),
				zap.String("ip", c.ClientIP()),
				zap.String("window", result.Window),
				zap.Int("limit", result.Limit),
				zap.Int("quota_used", result.QuotaUsed),
				zap.Int("quota_limit", result.QuotaLimit),
				zap.Time("reset_time", result.ResetTime),
			)

			c.JSON(statusCode, gin.H{
				"error":      errorMessage,
				"code":       errorCode,
				"window":     result.Window,
				"limit":      result.Limit,
				"reset_time": result.ResetTime.Format(time.RFC3339),
				"quota_used": result.QuotaUsed,
				"quota_limit": result.QuotaLimit,
			})
			c.Abort()
			return
		}

		// Store rate limit result in context for later use
		c.Set("rate_limit_result", result)

		// Set rate limit headers for successful requests
		c.Header("X-RateLimit-Limit", strconv.Itoa(result.Limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
		c.Header("X-RateLimit-Reset", result.ResetTime.Format(time.RFC3339))
		c.Header("X-RateLimit-Window", result.Window)
		c.Header("X-RateLimit-QuotaUsed", strconv.Itoa(result.QuotaUsed))
		c.Header("X-RateLimit-QuotaLimit", strconv.Itoa(result.QuotaLimit))

		c.Next()
	}
}

// IncrementRateLimit increments the rate limit counters after a successful request
func (r *RateLimitMiddleware) IncrementRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only increment if the request was successful
		if c.Writer.Status() >= 200 && c.Writer.Status() < 300 {
			apiKey := GetAPIKeyFromContext(c)
			if apiKey != "" {
				if err := r.rateLimiter.IncrementRateLimit(c.Request.Context(), apiKey); err != nil {
					r.logger.Error("Failed to increment rate limit",
						zap.String("api_key", maskAPIKey(apiKey)),
						zap.String("ip", c.ClientIP()),
						zap.Error(err),
					)
					// Don't fail the request if rate limit increment fails
					// Just log the error
				}
			}
		}
	}
} 