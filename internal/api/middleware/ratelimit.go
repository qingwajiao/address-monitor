package middleware

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type rateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

func (r *rateLimiter) allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-r.window)

	// 清理过期请求
	times := r.requests[key]
	valid := times[:0]
	for _, t := range times {
		if t.After(windowStart) {
			valid = append(valid, t)
		}
	}
	r.requests[key] = valid

	if len(valid) >= r.limit {
		return false
	}

	r.requests[key] = append(r.requests[key], now)
	return true
}

// RateLimit 限流中间件，每个 API Key 每分钟最多 600 次请求
func RateLimit() gin.HandlerFunc {
	limiter := newRateLimiter(600, time.Minute)
	return func(c *gin.Context) {
		key := c.GetHeader("X-API-Key")
		if key == "" {
			key = c.ClientIP()
		}
		if !limiter.allow(key) {
			c.AbortWithStatusJSON(429, gin.H{"error": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}
