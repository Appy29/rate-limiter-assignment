package services

import (
	"context"
	"fmt"
	"hash/crc32"
	"time"

	"github.com/go-redis/redis/v8"
)

// RedisManager manages multiple Redis clients with simple hashing
type RedisManager struct {
	clients []redis.Client // slice of Redis clients
}

// NewRedisManager creates a new Redis manager
func NewRedisManager(instances []string, password string, db int) *RedisManager {
	rm := &RedisManager{
		clients: make([]redis.Client, len(instances)),
	}

	// Create Redis clients for each instance
	for i, instance := range instances {
		rm.clients[i] = *redis.NewClient(&redis.Options{
			Addr:     instance,
			Password: password,
			DB:       db,
		})
	}

	return rm
}

// GetClient returns the Redis client for the given user ID
func (rm *RedisManager) GetClient(userID string) *redis.Client {
	fmt.Printf("DEBUG: GetClient called for userID='%s'\n", userID)
	fmt.Printf("DEBUG: Number of clients: %d\n", len(rm.clients))

	// Simple hash: use CRC32 to get a number, then mod by number of clients
	hash := crc32.ChecksumIEEE([]byte(userID))
	index := int(hash) % len(rm.clients)

	fmt.Printf("DEBUG: Hash=%d, Index=%d\n", hash, index)
	fmt.Printf("DEBUG: Returning client at index %d\n", index)

	return &rm.clients[index]
}

// GetClientIndex returns which Redis instance (0 or 1) for the user
func (rm *RedisManager) GetClientIndex(userID string) int {
	hash := crc32.ChecksumIEEE([]byte(userID))
	return int(hash) % len(rm.clients)
}

// GetHealthStatus returns health status of all clients
func (rm *RedisManager) GetHealthStatus() map[string]bool {
	status := make(map[string]bool)

	for i, client := range rm.clients {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := client.Ping(ctx).Result()
		cancel()

		status[fmt.Sprintf("redis-%d", i+1)] = err == nil
	}

	return status
}

// GetDistributionCount returns count of users per Redis instance (for load testing)
func (rm *RedisManager) GetDistributionCount(userIDs []string) map[string]int {
	counts := make(map[string]int)

	for _, userID := range userIDs {
		index := rm.GetClientIndex(userID)
		redisName := fmt.Sprintf("redis-%d", index+1)
		counts[redisName]++
	}

	return counts
}

// Close closes all Redis connections
func (rm *RedisManager) Close() error {
	for _, client := range rm.clients {
		if err := client.Close(); err != nil {
			return err
		}
	}
	return nil
}
