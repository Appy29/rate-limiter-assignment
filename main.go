package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/Appy29/rate-limiter/config"
	"github.com/Appy29/rate-limiter/handlers"
	"github.com/Appy29/rate-limiter/middleware"
	"github.com/Appy29/rate-limiter/services"
)

func main() {
	// Load configuration
	cfg := config.Load()

	fmt.Printf("Starting Rate Limiter Server...\n")
	fmt.Printf("Environment: %s\n", getEnv("ENV", "dev"))
	fmt.Printf("Server will run on: %s\n", cfg.GetServerAddress())
	fmt.Printf("Redis Instances: %v\n", cfg.Redis.Instances)
	fmt.Printf("Default Capacity: %d\n", cfg.RateLimit.DefaultCapacity)
	fmt.Printf("Default Refill Rate: %v\n", cfg.RateLimit.DefaultRefill)
	fmt.Printf("JWT Secret: %s\n", maskSecret(cfg.JWT.Secret))

	// Test Redis connectivity
	fmt.Println("\nTesting Redis connectivity...")
	redisManager := services.NewRedisManager(cfg.Redis.Instances, cfg.Redis.Password, cfg.Redis.DB)
	healthStatus := redisManager.GetHealthStatus()
	for node, healthy := range healthStatus {
		if healthy {
			fmt.Printf("Yes %s: Connected\n", node)
		} else {
			fmt.Printf("No %s: Failed\n", node)
		}
	}

	// Initialize services with Redis backend
	var rateLimiter services.RateLimiterInterface = services.NewRedisRateLimiterService(cfg)

	// Initialize handlers
	h := handlers.NewHandlers(rateLimiter)

	// Setup routes
	setupRoutes(h, cfg)

	// Start server
	fmt.Printf("\nServer starting on %s\n", cfg.GetServerAddress())
	if err := http.ListenAndServe(cfg.GetServerAddress(), nil); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}

func setupRoutes(h *handlers.Handlers, cfg *config.Config) {
	// Health check (no middleware needed)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Token generation endpoint (for testing) - only context middleware
	http.HandleFunc("/generate-token", middleware.ContextMiddleware(h.GenerateTokenHandler(cfg.JWT.Secret)))

	// Protected rate limiter endpoints - context + JWT middleware
	http.HandleFunc("/acquire", middleware.ContextMiddleware(
		middleware.JWTMiddleware(cfg.JWT.Secret)(h.AcquireHandler),
	))

	http.HandleFunc("/status", middleware.ContextMiddleware(
		middleware.JWTMiddleware(cfg.JWT.Secret)(h.StatusHandler),
	))

	// Metrics endpoint - only context middleware (no JWT required for monitoring)
	http.HandleFunc("/metrics", middleware.ContextMiddleware(h.MetricsHandler))

	// Root endpoint (register this LAST as it catches everything)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Rate Limiter Service is running! 🚀"))
	})
}

func getEnv(key, defaultValue string) string {
	return defaultValue
}

// maskSecret masks JWT secret for logging
func maskSecret(secret string) string {
	if len(secret) <= 6 {
		return "****"
	}
	return secret[:3] + "****" + secret[len(secret)-3:]
}
