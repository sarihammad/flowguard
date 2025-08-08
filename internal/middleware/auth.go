package middleware

import (
	"net/http"

	"rate-limiting-gateway/internal/storage"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	APIKeyHeader = "X-API-Key"
	APIKeyContextKey = "api_key"
)

// AuthMiddleware validates API keys
type AuthMiddleware struct {
	redis  *storage.RedisClient
	logger *zap.Logger
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(redis *storage.RedisClient, logger *zap.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		redis:  redis,
		logger: logger,
	}
}

// Authenticate validates the API key from the request header
func (a *AuthMiddleware) Authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader(APIKeyHeader)
		
		// Check if API key is provided
		if apiKey == "" {
			a.logger.Warn("Missing API key",
				zap.String("ip", c.ClientIP()),
				zap.String("user_agent", c.GetHeader("User-Agent")),
			)
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "API key is required",
				"code":  "MISSING_API_KEY",
			})
			c.Abort()
			return
		}

		// Validate API key
		valid, err := a.redis.ValidateAPIKey(c.Request.Context(), apiKey)
		if err != nil {
			a.logger.Error("Failed to validate API key",
				zap.String("api_key", apiKey),
				zap.String("ip", c.ClientIP()),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Internal server error",
				"code":  "VALIDATION_ERROR",
			})
			c.Abort()
			return
		}

		if !valid {
			a.logger.Warn("Invalid API key",
				zap.String("api_key", maskAPIKey(apiKey)),
				zap.String("ip", c.ClientIP()),
				zap.String("user_agent", c.GetHeader("User-Agent")),
			)
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid API key",
				"code":  "INVALID_API_KEY",
			})
			c.Abort()
			return
		}

		// Store API key in context for later use
		c.Set(APIKeyContextKey, apiKey)
		
		a.logger.Debug("API key validated successfully",
			zap.String("api_key", maskAPIKey(apiKey)),
			zap.String("ip", c.ClientIP()),
		)

		c.Next()
	}
}

// GetAPIKeyFromContext extracts the API key from the Gin context
func GetAPIKeyFromContext(c *gin.Context) string {
	if apiKey, exists := c.Get(APIKeyContextKey); exists {
		return apiKey.(string)
	}
	return ""
}

// maskAPIKey masks the API key for logging (shows only first 4 and last 4 characters)
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "***"
	}
	return apiKey[:4] + "..." + apiKey[len(apiKey)-4:]
} 