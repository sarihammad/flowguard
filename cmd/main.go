package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rate-limiting-gateway/internal/config"
	"rate-limiting-gateway/internal/handlers"
	"rate-limiting-gateway/internal/limiter"
	"rate-limiting-gateway/internal/metrics"
	"rate-limiting-gateway/internal/middleware"
	"rate-limiting-gateway/internal/storage"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	logger, err := initLogger()
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Starting rate limiting gateway...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	if err := cfg.Validate(); err != nil {
		logger.Fatal("Invalid configuration", zap.Error(err))
	}

	// Initialize Redis client
	redisClient, err := storage.NewRedisClient(
		cfg.Redis.Addr,
		cfg.Redis.Password,
		cfg.Redis.DB,
		cfg.Redis.PoolSize,
		logger,
	)
	if err != nil {
		logger.Fatal("Failed to connect to Redis", zap.Error(err))
	}
	defer redisClient.Close()

	// Initialize rate limiter
	rateLimiter := limiter.NewRateLimiter(redisClient, limiter.RateLimitConfig{
		RequestsPerMinute: cfg.RateLimit.RequestsPerMinute,
		RequestsPerHour:   cfg.RateLimit.RequestsPerHour,
		RequestsPerDay:    cfg.RateLimit.RequestsPerDay,
		MonthlyQuota:      cfg.RateLimit.MonthlyQuota,
		WindowSize:        cfg.RateLimit.WindowSize,
	}, logger)

	// Initialize metrics
	metricsInstance := metrics.NewMetrics(logger)

	// Initialize middleware
	authMiddleware := middleware.NewAuthMiddleware(redisClient, logger)
	rateLimitMiddleware := middleware.NewRateLimitMiddleware(rateLimiter, logger)
	loggingMiddleware := middleware.NewLoggingMiddleware(logger)

	// Initialize handlers
	gatewayHandler := handlers.NewGatewayHandler(cfg, rateLimiter, logger)

	// Setup Gin router
	router := setupRouter(
		authMiddleware,
		rateLimitMiddleware,
		loggingMiddleware,
		gatewayHandler,
		metricsInstance,
	)

	// Create HTTP server
	server := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start server in a goroutine
	go func() {
		logger.Info("Starting HTTP server", zap.String("port", cfg.Server.Port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Create a deadline for server shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited")
}

// setupRouter configures the Gin router with all middleware and routes
func setupRouter(
	authMiddleware *middleware.AuthMiddleware,
	rateLimitMiddleware *middleware.RateLimitMiddleware,
	loggingMiddleware *middleware.LoggingMiddleware,
	gatewayHandler *handlers.GatewayHandler,
	metricsInstance *metrics.Metrics,
) *gin.Engine {
	// Set Gin mode
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()

	// Add global middleware
	router.Use(gin.Recovery())
	router.Use(loggingMiddleware.LogRequest())
	router.Use(loggingMiddleware.LogError())

	// Health check endpoint (no auth required)
	router.GET("/health", gatewayHandler.HealthCheck)

	// Metrics endpoint (no auth required)
	router.GET("/metrics", metricsInstance.MetricsHandler())

	// API routes (require authentication)
	api := router.Group("/api")
	{
		// Rate limit info endpoint
		api.GET("/rate-limit-info", 
			authMiddleware.Authenticate(),
			gatewayHandler.GetRateLimitInfo,
		)
	}

	// Proxy endpoint (requires auth and rate limiting)
	proxy := router.Group("/proxy")
	{
		proxy.Use(authMiddleware.Authenticate())
		proxy.Use(rateLimitMiddleware.RateLimit())
		proxy.Use(rateLimitMiddleware.IncrementRateLimit())
		
		// Catch-all route for proxying
		proxy.Any("/*path", gatewayHandler.Proxy)
	}

	return router
}

// initLogger initializes the Zap logger
func initLogger() (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"stdout"}
	config.ErrorOutputPaths = []string{"stderr"}
	
	// Set log level based on environment
	if os.Getenv("LOG_LEVEL") != "" {
		if err := config.Level.UnmarshalText([]byte(os.Getenv("LOG_LEVEL"))); err != nil {
			return nil, fmt.Errorf("invalid log level: %w", err)
		}
	}

	// Set JSON logging based on environment
	if os.Getenv("LOG_JSON") == "false" {
		config.Encoding = "console"
	}

	return config.Build()
} 