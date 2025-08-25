package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Server struct {
		Port string `json:"port"`
		Host string `json:"host"`
	} `json:"server"`

	Redis struct {
		Instances []string `json:"instances"` // Multiple Redis instances
		Password  string   `json:"password"`
		DB        int      `json:"db"`
	} `json:"redis"`

	RateLimit struct {
		DefaultCapacity int64         `json:"default_capacity"`
		DefaultRefill   time.Duration `json:"default_refill"`
		Algorithm       string        `json:"algorithm"` // "token_bucket" or "leaky_bucket"
	} `json:"rate_limit"`

	JWT struct {
		Secret string `json:"secret"`
	} `json:"jwt"`
}

func Load() *Config {
	// Load .env file from config directory
	if err := godotenv.Load("config/.env"); err != nil {
		log.Println("No config/.env file found, using environment variables")
	}

	cfg := &Config{}
	cfg.loadFromEnv()

	return cfg
}

func (c *Config) loadFromEnv() {
	// Server config
	c.Server.Port = getEnv("PORT", "8080")
	c.Server.Host = getEnv("HOST", "localhost")

	// Redis config - support multiple instances
	redisInstances := getEnv("REDIS_INSTANCES", "localhost:6379,localhost:6380")
	c.Redis.Instances = strings.Split(redisInstances, ",")

	for i, instance := range c.Redis.Instances {
		c.Redis.Instances[i] = strings.TrimSpace(instance)
	}
	c.Redis.Password = getEnv("REDIS_PASSWORD", "")
	c.Redis.DB = getEnvInt("REDIS_DB", 0)

	// Rate limiter config
	c.RateLimit.DefaultCapacity = getEnvInt64("DEFAULT_CAPACITY", 100)
	c.RateLimit.DefaultRefill = getEnvDuration("DEFAULT_REFILL_RATE", time.Second)
	c.RateLimit.Algorithm = getEnv("ALGORITHM", "token_bucket")

	// JWT config
	c.JWT.Secret = getEnv("JWT_SECRET", "your-secret-key-change-in-production")
}

func (c *Config) GetServerAddress() string {
	return fmt.Sprintf("%s:%s", c.Server.Host, c.Server.Port)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
