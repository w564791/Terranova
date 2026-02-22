package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type rateLimitEntry struct {
	count    int
	windowStart time.Time
}

// RateLimiter 基于滑动窗口的内存限速器
type RateLimiter struct {
	maxAttempts int
	window      time.Duration
	mu          sync.Mutex
	entries     map[string]*rateLimitEntry
}

// NewRateLimiter 创建限速器
// maxAttempts: 窗口内最大请求数
// window: 时间窗口
func NewRateLimiter(maxAttempts int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		maxAttempts: maxAttempts,
		window:      window,
		entries:     make(map[string]*rateLimitEntry),
	}
}

// Allow 判断 key 是否允许通过，同时递增计数
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	entry, exists := rl.entries[key]

	if !exists || now.Sub(entry.windowStart) >= rl.window {
		// 新窗口
		rl.entries[key] = &rateLimitEntry{count: 1, windowStart: now}
		return true
	}

	if entry.count >= rl.maxAttempts {
		return false
	}

	entry.count++
	return true
}

// RetryAfter 返回 key 还需等待多久才能重试，0 表示未被限速
func (rl *RateLimiter) RetryAfter(key string) time.Duration {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	entry, exists := rl.entries[key]
	if !exists {
		return 0
	}

	remaining := rl.window - time.Since(entry.windowStart)
	if remaining <= 0 {
		return 0
	}

	if entry.count >= rl.maxAttempts {
		return remaining
	}

	return 0
}

// Cleanup 清除过期条目，释放内存
func (rl *RateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for key, entry := range rl.entries {
		if now.Sub(entry.windowStart) >= rl.window {
			delete(rl.entries, key)
		}
	}
}

// RateLimit 返回 Gin 限速中间件
func RateLimit(rl *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP()

		if !rl.Allow(key) {
			retryAfter := rl.RetryAfter(key)
			c.Header("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())+1))
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":      429,
				"message":   "Too many requests, please try again later",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
