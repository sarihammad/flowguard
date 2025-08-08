package middleware

import (
	"bytes"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// LoggingMiddleware handles request logging
type LoggingMiddleware struct {
	logger *zap.Logger
}

// NewLoggingMiddleware creates a new logging middleware
func NewLoggingMiddleware(logger *zap.Logger) *LoggingMiddleware {
	return &LoggingMiddleware{
		logger: logger,
	}
}

// LogRequest logs incoming requests
func (l *LoggingMiddleware) LogRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		
		// Capture request body for logging (optional)
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(start)
		
		// Get API key from context
		apiKey := GetAPIKeyFromContext(c)
		
		// Get rate limit info if available
		var rateLimitInfo interface{}
		if result, exists := c.Get("rate_limit_result"); exists {
			rateLimitInfo = result
		}

		// Log the request
		l.logger.Info("HTTP Request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("query", c.Request.URL.RawQuery),
			zap.String("ip", c.ClientIP()),
			zap.String("user_agent", c.GetHeader("User-Agent")),
			zap.String("api_key", maskAPIKey(apiKey)),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("duration", duration),
			zap.Int("response_size", c.Writer.Size()),
			zap.String("referer", c.GetHeader("Referer")),
			zap.String("forwarded_for", c.GetHeader("X-Forwarded-For")),
			zap.String("real_ip", c.GetHeader("X-Real-IP")),
			zap.Any("rate_limit_info", rateLimitInfo),
		)

		// Log request body for debugging (only for non-GET requests and small bodies)
		if len(bodyBytes) > 0 && len(bodyBytes) < 1024 && c.Request.Method != "GET" {
			l.logger.Debug("Request body",
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path),
				zap.String("body", string(bodyBytes)),
			)
		}
	}
}

// LogError logs errors that occur during request processing
func (l *LoggingMiddleware) LogError() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Check if there were any errors
		if len(c.Errors) > 0 {
			apiKey := GetAPIKeyFromContext(c)
			
			for _, err := range c.Errors {
				l.logger.Error("Request error",
					zap.String("method", c.Request.Method),
					zap.String("path", c.Request.URL.Path),
					zap.String("ip", c.ClientIP()),
					zap.String("api_key", maskAPIKey(apiKey)),
					zap.Int("status", c.Writer.Status()),
									zap.Error(err.Err),
				zap.String("error_type", string(err.Type)),
				)
			}
		}
	}
} 