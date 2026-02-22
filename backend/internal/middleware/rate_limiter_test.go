package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ========== RateLimiter Core Logic Tests ==========

func TestRateLimiter_AllowWithinLimit(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)

	for i := 0; i < 5; i++ {
		if !rl.Allow("192.168.1.1") {
			t.Errorf("request %d should be allowed within limit", i+1)
		}
	}
}

func TestRateLimiter_BlockAfterLimit(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)

	for i := 0; i < 3; i++ {
		rl.Allow("10.0.0.1")
	}

	if rl.Allow("10.0.0.1") {
		t.Error("4th request should be blocked after limit of 3")
	}
}

func TestRateLimiter_DifferentKeysIndependent(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)

	rl.Allow("ip-a")
	rl.Allow("ip-a")

	// ip-a exhausted, but ip-b should still work
	if !rl.Allow("ip-b") {
		t.Error("different key should have its own counter")
	}
}

func TestRateLimiter_ResetAfterWindow(t *testing.T) {
	rl := NewRateLimiter(2, 50*time.Millisecond)

	rl.Allow("key1")
	rl.Allow("key1")

	if rl.Allow("key1") {
		t.Error("should be blocked after limit")
	}

	time.Sleep(60 * time.Millisecond)

	if !rl.Allow("key1") {
		t.Error("should be allowed after window expires")
	}
}

func TestRateLimiter_RetryAfterReturnsPositive(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)

	rl.Allow("x")
	rl.Allow("x") // blocked

	retryAfter := rl.RetryAfter("x")
	if retryAfter <= 0 {
		t.Errorf("RetryAfter should be positive when blocked, got %v", retryAfter)
	}
	if retryAfter > time.Minute {
		t.Errorf("RetryAfter should not exceed window, got %v", retryAfter)
	}
}

func TestRateLimiter_RetryAfterZeroWhenNotBlocked(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)

	retryAfter := rl.RetryAfter("unknown-key")
	if retryAfter != 0 {
		t.Errorf("RetryAfter should be 0 for unknown key, got %v", retryAfter)
	}
}

func TestRateLimiter_Cleanup(t *testing.T) {
	rl := NewRateLimiter(1, 50*time.Millisecond)

	rl.Allow("a")
	rl.Allow("b")

	time.Sleep(60 * time.Millisecond)

	rl.Cleanup()

	// After cleanup, expired entries should be removed
	// New requests should succeed
	if !rl.Allow("a") {
		t.Error("should allow after cleanup removed expired entry")
	}
}

// ========== Gin Middleware Integration Tests ==========

func setupTestRouter(rl *RateLimiter) *gin.Engine {
	r := gin.New()
	r.POST("/login", RateLimit(rl), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	return r
}

func TestRateLimitMiddleware_AllowsNormalRequests(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)
	router := setupTestRouter(rl)

	req := httptest.NewRequest("POST", "/login", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRateLimitMiddleware_Returns429WhenExceeded(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	router := setupTestRouter(rl)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/login", nil)
		req.RemoteAddr = "1.2.3.4:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	// 3rd request should be 429
	req := httptest.NewRequest("POST", "/login", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
}

func TestRateLimitMiddleware_SetsRetryAfterHeader(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	router := setupTestRouter(rl)

	// exhaust
	req := httptest.NewRequest("POST", "/login", nil)
	req.RemoteAddr = "5.6.7.8:1111"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// blocked
	req = httptest.NewRequest("POST", "/login", nil)
	req.RemoteAddr = "5.6.7.8:1111"
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
	retryAfter := w.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Error("expected Retry-After header to be set")
	}
}

func TestRateLimitMiddleware_UsesClientIPNotRemoteAddr(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	router := setupTestRouter(rl)

	// Two requests from different RemoteAddr but same X-Forwarded-For
	// should count as same client
	req1 := httptest.NewRequest("POST", "/login", nil)
	req1.RemoteAddr = "10.0.0.1:1111"
	req1.Header.Set("X-Forwarded-For", "203.0.113.50")
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)

	req2 := httptest.NewRequest("POST", "/login", nil)
	req2.RemoteAddr = "10.0.0.2:2222" // different proxy
	req2.Header.Set("X-Forwarded-For", "203.0.113.50") // same real IP
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 for same X-Forwarded-For IP, got %d", w2.Code)
	}
}

func TestRateLimitMiddleware_ResponseBody(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	router := setupTestRouter(rl)

	req := httptest.NewRequest("POST", "/login", nil)
	req.RemoteAddr = "9.9.9.9:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// trigger 429
	req = httptest.NewRequest("POST", "/login", nil)
	req.RemoteAddr = "9.9.9.9:1234"
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body := w.Body.String()
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
	if body == "" {
		t.Error("expected non-empty response body")
	}
}

// ========== Concurrent Access Test ==========

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := NewRateLimiter(100, time.Minute)

	done := make(chan bool, 200)
	for i := 0; i < 200; i++ {
		go func() {
			rl.Allow("concurrent-key")
			done <- true
		}()
	}

	for i := 0; i < 200; i++ {
		<-done
	}

	// Should have blocked ~100 of the 200 requests
	// Just verify no panic/race — exact count depends on scheduling
	if rl.Allow("concurrent-key") {
		t.Error("should be blocked after 200 attempts with limit 100")
	}
}
