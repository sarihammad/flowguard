package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// Metrics holds all Prometheus metrics
type Metrics struct {
	requestCounter   *prometheus.CounterVec
	requestDuration  *prometheus.HistogramVec
	requestSize      *prometheus.HistogramVec
	responseSize     *prometheus.HistogramVec
	rateLimitCounter *prometheus.CounterVec
	upstreamErrors   *prometheus.CounterVec
	logger           *zap.Logger
}

// NewMetrics creates a new metrics instance
func NewMetrics(logger *zap.Logger) *Metrics {
	requestCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_requests_total",
			Help: "Total number of requests processed",
		},
		[]string{"method", "path", "status_code", "api_key"},
	)

	requestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_request_duration_seconds",
			Help:    "Request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status_code"},
	)

	requestSize := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_request_size_bytes",
			Help:    "Request size in bytes",
			Buckets: prometheus.ExponentialBuckets(100, 10, 8),
		},
		[]string{"method", "path"},
	)

	responseSize := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_response_size_bytes",
			Help:    "Response size in bytes",
			Buckets: prometheus.ExponentialBuckets(100, 10, 8),
		},
		[]string{"method", "path", "status_code"},
	)

	rateLimitCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_rate_limit_exceeded_total",
			Help: "Total number of rate limit violations",
		},
		[]string{"api_key", "window"},
	)

	upstreamErrors := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_upstream_errors_total",
			Help: "Total number of upstream service errors",
		},
		[]string{"api_key", "error_type"},
	)

	// Register metrics
	prometheus.MustRegister(requestCounter)
	prometheus.MustRegister(requestDuration)
	prometheus.MustRegister(requestSize)
	prometheus.MustRegister(responseSize)
	prometheus.MustRegister(rateLimitCounter)
	prometheus.MustRegister(upstreamErrors)

	return &Metrics{
		requestCounter:   requestCounter,
		requestDuration:  requestDuration,
		requestSize:      requestSize,
		responseSize:     responseSize,
		rateLimitCounter: rateLimitCounter,
		upstreamErrors:   upstreamErrors,
		logger:           logger,
	}
}

// RecordRequest records a request metric
func (m *Metrics) RecordRequest(method, path, apiKey string, statusCode int, duration time.Duration, requestSize, responseSize int) {
	statusCodeStr := strconv.Itoa(statusCode)
	maskedAPIKey := maskAPIKey(apiKey)

	m.requestCounter.WithLabelValues(method, path, statusCodeStr, maskedAPIKey).Inc()
	m.requestDuration.WithLabelValues(method, path, statusCodeStr).Observe(duration.Seconds())
	
	if requestSize > 0 {
		m.requestSize.WithLabelValues(method, path).Observe(float64(requestSize))
	}
	
	if responseSize > 0 {
		m.responseSize.WithLabelValues(method, path, statusCodeStr).Observe(float64(responseSize))
	}

	m.logger.Debug("Request metric recorded",
		zap.String("method", method),
		zap.String("path", path),
		zap.String("api_key", maskedAPIKey),
		zap.Int("status_code", statusCode),
		zap.Duration("duration", duration),
		zap.Int("request_size", requestSize),
		zap.Int("response_size", responseSize),
	)
}

// RecordRateLimitExceeded records a rate limit violation
func (m *Metrics) RecordRateLimitExceeded(apiKey, window string) {
	maskedAPIKey := maskAPIKey(apiKey)
	m.rateLimitCounter.WithLabelValues(maskedAPIKey, window).Inc()

	m.logger.Debug("Rate limit metric recorded",
		zap.String("api_key", maskedAPIKey),
		zap.String("window", window),
	)
}

// RecordUpstreamError records an upstream service error
func (m *Metrics) RecordUpstreamError(apiKey, errorType string) {
	maskedAPIKey := maskAPIKey(apiKey)
	m.upstreamErrors.WithLabelValues(maskedAPIKey, errorType).Inc()

	m.logger.Debug("Upstream error metric recorded",
		zap.String("api_key", maskedAPIKey),
		zap.String("error_type", errorType),
	)
}

// MetricsHandler returns the Prometheus metrics handler
func (m *Metrics) MetricsHandler() gin.HandlerFunc {
	return gin.WrapH(promhttp.Handler())
}

// maskAPIKey masks the API key for metrics (shows only first 4 and last 4 characters)
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "***"
	}
	return apiKey[:4] + "..." + apiKey[len(apiKey)-4:]
} 