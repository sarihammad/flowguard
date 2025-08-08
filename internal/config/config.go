package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	Server   ServerConfig
	Redis    RedisConfig
	RateLimit RateLimitConfig
	Target   TargetConfig
	Logging  LoggingConfig
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
	PoolSize int
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	RequestsPerMinute int
	RequestsPerHour   int
	RequestsPerDay    int
	MonthlyQuota      int
	WindowSize        time.Duration
}

// TargetConfig holds upstream service configuration
type TargetConfig struct {
	URL     string
	Timeout time.Duration
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level string
	JSON  bool
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		// It's okay if .env doesn't exist
	}

	config := &Config{
		Server: ServerConfig{
			Port:         getEnv("SERVER_PORT", "8080"),
			ReadTimeout:  getEnvAsDuration("SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout: getEnvAsDuration("SERVER_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:  getEnvAsDuration("SERVER_IDLE_TIMEOUT", 60*time.Second),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvAsInt("REDIS_DB", 0),
			PoolSize: getEnvAsInt("REDIS_POOL_SIZE", 10),
		},
		RateLimit: RateLimitConfig{
			RequestsPerMinute: getEnvAsInt("RATE_LIMIT_PER_MINUTE", 60),
			RequestsPerHour:   getEnvAsInt("RATE_LIMIT_PER_HOUR", 1000),
			RequestsPerDay:    getEnvAsInt("RATE_LIMIT_PER_DAY", 10000),
			MonthlyQuota:      getEnvAsInt("RATE_LIMIT_MONTHLY_QUOTA", 100000),
			WindowSize:        getEnvAsDuration("RATE_LIMIT_WINDOW_SIZE", time.Minute),
		},
		Target: TargetConfig{
			URL:     getEnv("TARGET_URL", "http://localhost:3000"),
			Timeout: getEnvAsDuration("TARGET_TIMEOUT", 30*time.Second),
		},
		Logging: LoggingConfig{
			Level: getEnv("LOG_LEVEL", "info"),
			JSON:  getEnvAsBool("LOG_JSON", true),
		},
	}

	return config, nil
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvAsInt gets an environment variable as int or returns a default value
func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvAsBool gets an environment variable as bool or returns a default value
func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

// getEnvAsDuration gets an environment variable as duration or returns a default value
func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.Port == "" {
		return fmt.Errorf("server port is required")
	}
	if c.Redis.Addr == "" {
		return fmt.Errorf("redis address is required")
	}
	if c.Target.URL == "" {
		return fmt.Errorf("target URL is required")
	}
	return nil
} 