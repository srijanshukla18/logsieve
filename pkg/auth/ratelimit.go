package auth

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	RequestsPerMin int    `mapstructure:"requests_per_minute"`
	BurstSize      int    `mapstructure:"burst_size"`
	KeyFunc        string `mapstructure:"key_func"` // "ip", "header", "api_key"
}

// RateLimiter provides rate limiting functionality
type RateLimiter struct {
	config        RateLimitConfig
	logger        zerolog.Logger
	buckets       map[string]*bucket
	mu            sync.RWMutex
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

// bucket represents a token bucket for rate limiting
type bucket struct {
	tokens     int
	lastRefill time.Time
	mu         sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(config RateLimitConfig, logger zerolog.Logger) *RateLimiter {
	rl := &RateLimiter{
		config:      config,
		logger:      logger.With().Str("component", "ratelimit").Logger(),
		buckets:     make(map[string]*bucket),
		stopCleanup: make(chan struct{}),
	}

	// Start cleanup goroutine to remove old buckets
	rl.cleanupTicker = time.NewTicker(1 * time.Minute)
	go rl.cleanupLoop()

	return rl
}

// Middleware returns a Gin middleware function for rate limiting
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rl.config.Enabled {
			c.Next()
			return
		}

		key := rl.getKey(c)

		if !rl.allow(key) {
			rl.logger.Warn().
				Str("key", key).
				Str("client_ip", c.ClientIP()).
				Msg("Rate limit exceeded")

			c.Header("X-RateLimit-Limit", strconv.Itoa(rl.config.RequestsPerMin))
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("Retry-After", "60")

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":               "Rate limit exceeded",
				"message":             "Too many requests. Please try again later",
				"retry_after_seconds": 60,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// allow checks if a request is allowed based on rate limiting
func (rl *RateLimiter) allow(key string) bool {
	rl.mu.Lock()
	b, exists := rl.buckets[key]
	if !exists {
		b = &bucket{
			tokens:     rl.config.BurstSize,
			lastRefill: time.Now(),
		}
		rl.buckets[key] = b
	}
	rl.mu.Unlock()

	b.mu.Lock()
	defer b.mu.Unlock()

	// Refill tokens based on time passed
	now := time.Now()
	timePassed := now.Sub(b.lastRefill)
	tokensToAdd := int(timePassed.Minutes() * float64(rl.config.RequestsPerMin))

	if tokensToAdd > 0 {
		b.tokens += tokensToAdd
		if b.tokens > rl.config.BurstSize {
			b.tokens = rl.config.BurstSize
		}
		b.lastRefill = now
	}

	// Check if we have tokens available
	if b.tokens > 0 {
		b.tokens--
		return true
	}

	return false
}

// getKey extracts the rate limiting key from the request
func (rl *RateLimiter) getKey(c *gin.Context) string {
	switch rl.config.KeyFunc {
	case "header":
		// Use X-Forwarded-For or X-Real-IP if available
		if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
			return xff
		}
		if xri := c.GetHeader("X-Real-IP"); xri != "" {
			return xri
		}
		return c.ClientIP()

	case "api_key":
		// Use API key if available
		if apiKey := c.GetHeader("X-API-Key"); apiKey != "" {
			return "key:" + apiKey
		}
		return "ip:" + c.ClientIP()

	default: // "ip"
		return c.ClientIP()
	}
}

// cleanupLoop periodically removes old buckets
func (rl *RateLimiter) cleanupLoop() {
	for {
		select {
		case <-rl.cleanupTicker.C:
			rl.cleanup()
		case <-rl.stopCleanup:
			rl.cleanupTicker.Stop()
			return
		}
	}
}

// cleanup removes buckets that haven't been used recently
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-5 * time.Minute)
	removed := 0

	for key, b := range rl.buckets {
		b.mu.Lock()
		lastUsed := b.lastRefill
		b.mu.Unlock()

		if lastUsed.Before(cutoff) {
			delete(rl.buckets, key)
			removed++
		}
	}

	if removed > 0 {
		rl.logger.Debug().
			Int("removed", removed).
			Int("remaining", len(rl.buckets)).
			Msg("Cleaned up rate limit buckets")
	}
}

// Stop stops the rate limiter and cleanup goroutine
func (rl *RateLimiter) Stop() {
	close(rl.stopCleanup)
}

// Stats returns rate limiter statistics
func (rl *RateLimiter) Stats() RateLimiterStats {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	return RateLimiterStats{
		Enabled:        rl.config.Enabled,
		ActiveBuckets:  len(rl.buckets),
		RequestsPerMin: rl.config.RequestsPerMin,
		BurstSize:      rl.config.BurstSize,
	}
}

// RateLimiterStats holds rate limiter statistics
type RateLimiterStats struct {
	Enabled        bool `json:"enabled"`
	ActiveBuckets  int  `json:"active_buckets"`
	RequestsPerMin int  `json:"requests_per_minute"`
	BurstSize      int  `json:"burst_size"`
}
