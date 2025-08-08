package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"rate-limiting-gateway/internal/config"
	"rate-limiting-gateway/internal/handlers"
	"rate-limiting-gateway/internal/limiter"
	"rate-limiting-gateway/internal/middleware"
	"rate-limiting-gateway/internal/metrics"
	"rate-limiting-gateway/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestGatewayIntegration tests the complete gateway functionality
func TestGatewayIntegration(t *testing.T) {
	// Skip if Redis is not available
	if !isRedisAvailable() {
		t.Skip("Redis not available, skipping integration test")
	}

	// Setup
	logger, _ := zap.NewDevelopment()
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: "8080",
		},
		Redis: config.RedisConfig{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
			PoolSize: 10,
		},
		RateLimit: config.RateLimitConfig{
			RequestsPerMinute: 5, // Low limit for testing
			RequestsPerHour:   100,
			RequestsPerDay:    1000,
			MonthlyQuota:      10000,
			WindowSize:        time.Minute,
		},
		Target: config.TargetConfig{
			URL:     "http://localhost:8000",
			Timeout: 30 * time.Second,
		},
		Logging: config.LoggingConfig{
			Level: "debug",
			JSON:  false,
		},
	}

	// Create mock upstream server
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := map[string]interface{}{
			"message": "Hello from upstream",
			"path":    r.URL.Path,
			"method":  r.Method,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer mockUpstream.Close()

	// Update target URL to use mock server
	cfg.Target.URL = mockUpstream.URL

	// Initialize components
	redisClient, err := storage.NewRedisClient(
		cfg.Redis.Addr,
		cfg.Redis.Password,
		cfg.Redis.DB,
		cfg.Redis.PoolSize,
		logger,
	)
	require.NoError(t, err)
	defer redisClient.Close()

	rateLimiter := limiter.NewRateLimiter(redisClient, limiter.RateLimitConfig{
		RequestsPerMinute: cfg.RateLimit.RequestsPerMinute,
		RequestsPerHour:   cfg.RateLimit.RequestsPerHour,
		RequestsPerDay:    cfg.RateLimit.RequestsPerDay,
		MonthlyQuota:      cfg.RateLimit.MonthlyQuota,
		WindowSize:        cfg.RateLimit.WindowSize,
	}, logger)

	metricsInstance := metrics.NewMetrics(logger)

	// Initialize middleware
	authMiddleware := middleware.NewAuthMiddleware(redisClient, logger)
	rateLimitMiddleware := middleware.NewRateLimitMiddleware(rateLimiter, logger)
	loggingMiddleware := middleware.NewLoggingMiddleware(logger)

	// Initialize handlers
	gatewayHandler := handlers.NewGatewayHandler(cfg, rateLimiter, logger)

	// Setup router
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(loggingMiddleware.LogRequest())
	router.Use(loggingMiddleware.LogError())

	// Health check endpoint
	router.GET("/health", gatewayHandler.HealthCheck)

	// Metrics endpoint
	router.GET("/metrics", metricsInstance.MetricsHandler())

	// API routes
	api := router.Group("/api")
	{
		api.GET("/rate-limit-info",
			authMiddleware.Authenticate(),
			gatewayHandler.GetRateLimitInfo,
		)
	}

	// Proxy endpoint
	proxy := router.Group("/proxy")
	{
		proxy.Use(authMiddleware.Authenticate())
		proxy.Use(rateLimitMiddleware.RateLimit())
		proxy.Use(rateLimitMiddleware.IncrementRateLimit())
		proxy.Any("/*path", gatewayHandler.Proxy)
	}

	// Test cases
	t.Run("HealthCheck", func(t *testing.T) {
		testHealthCheck(t, router)
	})

	t.Run("Metrics", func(t *testing.T) {
		testMetrics(t, router)
	})

	t.Run("Authentication", func(t *testing.T) {
		testAuthentication(t, router)
	})

	t.Run("RateLimiting", func(t *testing.T) {
		testRateLimiting(t, router, redisClient)
	})

	t.Run("Proxy", func(t *testing.T) {
		testProxy(t, router)
	})
}

func testHealthCheck(t *testing.T, router *gin.Engine) {
	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "healthy", response["status"])
	assert.Equal(t, "rate-limiting-gateway", response["service"])
}

func testMetrics(t *testing.T, router *gin.Engine) {
	req, _ := http.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "gateway_requests_total")
}

func testAuthentication(t *testing.T, router *gin.Engine) {
	// Test without API key
	req, _ := http.NewRequest("GET", "/api/rate-limit-info", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "API key is required", response["error"])

	// Test with empty API key
	req, _ = http.NewRequest("GET", "/api/rate-limit-info", nil)
	req.Header.Set("X-API-Key", "")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Test with valid API key
	req, _ = http.NewRequest("GET", "/api/rate-limit-info", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func testRateLimiting(t *testing.T, router *gin.Engine, redisClient *storage.RedisClient) {
	apiKey := "test-rate-limit-key"

	// Make requests up to the limit
	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest("GET", "/proxy/test", nil)
		req.Header.Set("X-API-Key", apiKey)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if i < 4 {
			assert.Equal(t, http.StatusOK, w.Code)
		} else {
			// Last request should be rate limited
			assert.Equal(t, http.StatusTooManyRequests, w.Code)
		}
	}

	// Wait for rate limit to reset (in real scenario, this would be 1 minute)
	// For testing, we'll clear the Redis keys
	ctx := context.Background()
	keys, err := redisClient.client.Keys(ctx, "rate:*").Result()
	require.NoError(t, err)
	if len(keys) > 0 {
		redisClient.client.Del(ctx, keys...)
	}

	// Test that requests work again after reset
	req, _ := http.NewRequest("GET", "/proxy/test", nil)
	req.Header.Set("X-API-Key", apiKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func testProxy(t *testing.T, router *gin.Engine) {
	req, _ := http.NewRequest("GET", "/proxy/test", nil)
	req.Header.Set("X-API-Key", "test-proxy-key")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "Hello from upstream", response["message"])
	assert.Equal(t, "/test", response["path"])
	assert.Equal(t, "GET", response["method"])

	// Check for gateway headers
	assert.NotEmpty(t, w.Header().Get("X-Gateway-Response-Time"))
	assert.NotEmpty(t, w.Header().Get("X-Gateway-Upstream-Status"))
}

// isRedisAvailable checks if Redis is available for testing
func isRedisAvailable() bool {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Ping(ctx).Result()
	return err == nil
}

// TestRateLimitHeaders tests that rate limit headers are properly set
func TestRateLimitHeaders(t *testing.T) {
	if !isRedisAvailable() {
		t.Skip("Redis not available, skipping test")
	}

	logger, _ := zap.NewDevelopment()
	cfg := &config.Config{
		RateLimit: config.RateLimitConfig{
			RequestsPerMinute: 10,
			RequestsPerHour:   100,
			RequestsPerDay:    1000,
			MonthlyQuota:      10000,
		},
	}

	redisClient, err := storage.NewRedisClient("localhost:6379", "", 0, 10, logger)
	require.NoError(t, err)
	defer redisClient.Close()

	rateLimiter := limiter.NewRateLimiter(redisClient, limiter.RateLimitConfig{
		RequestsPerMinute: cfg.RateLimit.RequestsPerMinute,
		RequestsPerHour:   cfg.RateLimit.RequestsPerHour,
		RequestsPerDay:    cfg.RateLimit.RequestsPerDay,
		MonthlyQuota:      cfg.RateLimit.MonthlyQuota,
	}, logger)

	rateLimitMiddleware := middleware.NewRateLimitMiddleware(rateLimiter, logger)

	router := gin.New()
	router.Use(rateLimitMiddleware.RateLimit())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Make a request
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "test-headers-key")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Check rate limit headers
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Limit"))
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Reset"))
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Window"))
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-QuotaUsed"))
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-QuotaLimit"))
} 