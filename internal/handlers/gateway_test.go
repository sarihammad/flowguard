package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"rate-limiting-gateway/internal/config"
	"rate-limiting-gateway/internal/limiter"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockRateLimiter is a mock implementation of the rate limiter
type MockRateLimiter struct {
	mock.Mock
}

func (m *MockRateLimiter) CheckRateLimit(ctx context.Context, apiKey string) (*limiter.RateLimitResult, error) {
	args := m.Called(ctx, apiKey)
	return args.Get(0).(*limiter.RateLimitResult), args.Error(1)
}

func (m *MockRateLimiter) IncrementRateLimit(ctx context.Context, apiKey string) error {
	args := m.Called(ctx, apiKey)
	return args.Error(0)
}

func (m *MockRateLimiter) GetRateLimitInfo(ctx context.Context, apiKey string) (map[string]interface{}, error) {
	args := m.Called(ctx, apiKey)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

func TestGatewayHandler_HealthCheck(t *testing.T) {
	// Setup
	logger, _ := zap.NewDevelopment()
	cfg := &config.Config{}
	rateLimiter := &MockRateLimiter{}
	
	handler := NewGatewayHandler(cfg, rateLimiter, logger)
	
	// Create Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/health", handler.HealthCheck)
	
	// Create request
	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	
	// Execute request
	router.ServeHTTP(w, req)
	
	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)
	
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	
	assert.Equal(t, "healthy", response["status"])
	assert.Equal(t, "rate-limiting-gateway", response["service"])
	assert.Equal(t, "1.0.0", response["version"])
	assert.NotEmpty(t, response["timestamp"])
}

func TestGatewayHandler_GetRateLimitInfo(t *testing.T) {
	// Setup
	logger, _ := zap.NewDevelopment()
	cfg := &config.Config{}
	rateLimiter := &MockRateLimiter{}
	
	handler := NewGatewayHandler(cfg, rateLimiter, logger)
	
	// Create Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/rate-limit-info", func(c *gin.Context) {
		// Set API key in context for testing
		c.Set("api_key", "test-api-key")
		handler.GetRateLimitInfo(c)
	})
	
	// Mock rate limiter response
	expectedInfo := map[string]interface{}{
		"monthly_quota": map[string]interface{}{
			"used":  float64(50),
			"limit": float64(100000),
		},
		"rate_limits": map[string]interface{}{
			"minute": map[string]interface{}{
				"current":    float64(10),
				"limit":      float64(60),
				"remaining":  float64(50),
				"reset_time": time.Now().Add(time.Minute).Format(time.RFC3339),
			},
		},
	}
	
	rateLimiter.On("GetRateLimitInfo", mock.Anything, "test-api-key").Return(expectedInfo, nil)
	
	// Create request
	req, _ := http.NewRequest("GET", "/rate-limit-info", nil)
	w := httptest.NewRecorder()
	
	// Execute request
	router.ServeHTTP(w, req)
	
	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)
	
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	
	assert.Equal(t, "test...-key", response["api_key"])
	assert.Equal(t, expectedInfo, response["info"])
	
	rateLimiter.AssertExpectations(t)
}

func TestGatewayHandler_GetRateLimitInfo_NoAPIKey(t *testing.T) {
	// Setup
	logger, _ := zap.NewDevelopment()
	cfg := &config.Config{}
	rateLimiter := &MockRateLimiter{}
	
	handler := NewGatewayHandler(cfg, rateLimiter, logger)
	
	// Create Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/rate-limit-info", handler.GetRateLimitInfo)
	
	// Create request
	req, _ := http.NewRequest("GET", "/rate-limit-info", nil)
	w := httptest.NewRecorder()
	
	// Execute request
	router.ServeHTTP(w, req)
	
	// Assertions
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	
	assert.Equal(t, "API key not found in context", response["error"])
	assert.Equal(t, "INTERNAL_ERROR", response["code"])
}

func TestGatewayHandler_GetRateLimitInfo_Error(t *testing.T) {
	// Setup
	logger, _ := zap.NewDevelopment()
	cfg := &config.Config{}
	rateLimiter := &MockRateLimiter{}
	
	handler := NewGatewayHandler(cfg, rateLimiter, logger)
	
	// Create Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/rate-limit-info", func(c *gin.Context) {
		// Set API key in context for testing
		c.Set("api_key", "test-api-key")
		handler.GetRateLimitInfo(c)
	})
	
	// Mock rate limiter error
	rateLimiter.On("GetRateLimitInfo", mock.Anything, "test-api-key").Return(nil, assert.AnError)
	
	// Create request
	req, _ := http.NewRequest("GET", "/rate-limit-info", nil)
	w := httptest.NewRecorder()
	
	// Execute request
	router.ServeHTTP(w, req)
	
	// Assertions
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	
	assert.Equal(t, "Failed to get rate limit information", response["error"])
	assert.Equal(t, "RATE_LIMIT_ERROR", response["code"])
	
	rateLimiter.AssertExpectations(t)
}

func TestMaskAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		apiKey   string
		expected string
	}{
		{
			name:     "short key",
			apiKey:   "123",
			expected: "***",
		},
		{
			name:     "medium key",
			apiKey:   "12345678",
			expected: "***",
		},
		{
			name:     "long key",
			apiKey:   "12345678901234567890",
			expected: "1234...7890",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskAPIKey(tt.apiKey)
			assert.Equal(t, tt.expected, result)
		})
	}
} 