package middleware

import (
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type rateLimiter struct {
	mu       sync.Mutex
	counters map[string]*counter
	limit    int
	window   time.Duration
}

type counter struct {
	count    int
	resetAt  time.Time
}

// RateLimitPerUser returns middleware that limits requests per user per minute.
func RateLimitPerUser() gin.HandlerFunc {
	limit := 10
	if v := os.Getenv("RATE_LIMIT_EXCHANGE_PER_MINUTE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	rl := &rateLimiter{
		counters: make(map[string]*counter),
		limit:    limit,
		window:   time.Minute,
	}

	return func(c *gin.Context) {
		claims, ok := GetSessionClaims(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		if !rl.allow(claims.Sub) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}

		c.Next()
	}
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	c, exists := rl.counters[key]
	if !exists || now.After(c.resetAt) {
		rl.counters[key] = &counter{count: 1, resetAt: now.Add(rl.window)}
		return true
	}

	if c.count >= rl.limit {
		return false
	}

	c.count++
	return true
}
