package handlers

import (
	"io"
	"net/http"
	"time"

	"rate-limiting-gateway/internal/config"
	"rate-limiting-gateway/internal/limiter"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GatewayHandler handles proxy requests to upstream services
type GatewayHandler struct {
	config     *config.Config
	rateLimiter limiter.RateLimiterInterface
	logger     *zap.Logger
	httpClient *http.Client
}

// NewGatewayHandler creates a new gateway handler
func NewGatewayHandler(config *config.Config, rateLimiter limiter.RateLimiterInterface, logger *zap.Logger) *GatewayHandler {
	return &GatewayHandler{
		config:      config,
		rateLimiter: rateLimiter,
		logger:      logger,
		httpClient: &http.Client{
			Timeout: config.Target.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// Proxy forwards requests to the upstream service
func (g *GatewayHandler) Proxy(c *gin.Context) {
	start := time.Now()
	
	// Get API key from context
	apiKey := c.GetString("api_key")
	if apiKey == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "API key not found in context",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	// Create the target URL
	targetURL := g.config.Target.URL + c.Request.URL.Path
	if c.Request.URL.RawQuery != "" {
		targetURL += "?" + c.Request.URL.RawQuery
	}

	// Create the request to the upstream service
	req, err := http.NewRequestWithContext(
		c.Request.Context(),
		c.Request.Method,
		targetURL,
		c.Request.Body,
	)
	if err != nil {
		g.logger.Error("Failed to create upstream request",
			zap.String("api_key", maskAPIKey(apiKey)),
			zap.String("target_url", targetURL),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create upstream request",
			"code":  "UPSTREAM_ERROR",
		})
		return
	}

	// Copy headers from the original request
	for key, values := range c.Request.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Add gateway-specific headers
	req.Header.Set("X-Gateway-API-Key", maskAPIKey(apiKey))
	req.Header.Set("X-Gateway-Request-ID", c.GetString("request_id"))
	req.Header.Set("X-Gateway-Timestamp", time.Now().Format(time.RFC3339))

	// Make the request to the upstream service
	resp, err := g.httpClient.Do(req)
	if err != nil {
		g.logger.Error("Failed to make upstream request",
			zap.String("api_key", maskAPIKey(apiKey)),
			zap.String("target_url", targetURL),
			zap.String("method", c.Request.Method),
			zap.Error(err),
		)
		c.JSON(http.StatusBadGateway, gin.H{
			"error": "Upstream service unavailable",
			"code":  "UPSTREAM_UNAVAILABLE",
		})
		return
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		g.logger.Error("Failed to read upstream response",
			zap.String("api_key", maskAPIKey(apiKey)),
			zap.String("target_url", targetURL),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to read upstream response",
			"code":  "UPSTREAM_ERROR",
		})
		return
	}

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}

	// Add gateway-specific response headers
	duration := time.Since(start)
	c.Header("X-Gateway-Response-Time", duration.String())
	c.Header("X-Gateway-Upstream-Status", resp.Status)

	// Set the response status and body
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)

	g.logger.Info("Request proxied successfully",
		zap.String("api_key", maskAPIKey(apiKey)),
		zap.String("target_url", targetURL),
		zap.String("method", c.Request.Method),
		zap.Int("upstream_status", resp.StatusCode),
		zap.Duration("duration", duration),
		zap.Int("response_size", len(body)),
	)
}

// HealthCheck handles health check requests
func (g *GatewayHandler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"service":   "rate-limiting-gateway",
		"version":   "1.0.0",
	})
}

// GetRateLimitInfo returns rate limit information for the current API key
func (g *GatewayHandler) GetRateLimitInfo(c *gin.Context) {
	apiKey := c.GetString("api_key")
	if apiKey == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "API key not found in context",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	info, err := g.rateLimiter.GetRateLimitInfo(c.Request.Context(), apiKey)
	if err != nil {
		g.logger.Error("Failed to get rate limit info",
			zap.String("api_key", maskAPIKey(apiKey)),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get rate limit information",
			"code":  "RATE_LIMIT_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"api_key": maskAPIKey(apiKey),
		"info":    info,
	})
}

// maskAPIKey masks the API key for logging (shows only first 4 and last 4 characters)
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "***"
	}
	return apiKey[:4] + "..." + apiKey[len(apiKey)-4:]
} 