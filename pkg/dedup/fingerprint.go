package dedup

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

type FingerprintCache struct {
	fingerprints map[string]time.Time
	ttl          time.Duration
	logger       zerolog.Logger
	mu           sync.RWMutex
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

func NewFingerprintCache(ttl time.Duration, logger zerolog.Logger) *FingerprintCache {
	fc := &FingerprintCache{
		fingerprints: make(map[string]time.Time),
		ttl:          ttl,
		logger:       logger.With().Str("component", "fingerprint").Logger(),
		stopCleanup:  make(chan struct{}),
	}

	fc.cleanupTicker = time.NewTicker(ttl / 2)
	go fc.cleanupLoop()

	return fc
}

func (fc *FingerprintCache) GetFingerprint(message string) string {
	hash := sha256.Sum256([]byte(message))
	return fmt.Sprintf("%x", hash)
}

func (fc *FingerprintCache) Add(fingerprint string) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	
	fc.fingerprints[fingerprint] = time.Now().Add(fc.ttl)
}

func (fc *FingerprintCache) Exists(fingerprint string) bool {
    // Use write lock because we may delete expired entries
    fc.mu.Lock()
    defer fc.mu.Unlock()

    expiryTime, exists := fc.fingerprints[fingerprint]
    if !exists {
        return false
    }

    if time.Now().After(expiryTime) {
        delete(fc.fingerprints, fingerprint)
        return false
    }

    return true
}

func (fc *FingerprintCache) Size() int {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	
	return len(fc.fingerprints)
}

func (fc *FingerprintCache) Clear() {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	
	fc.fingerprints = make(map[string]time.Time)
}

func (fc *FingerprintCache) cleanupLoop() {
	for {
		select {
		case <-fc.cleanupTicker.C:
			fc.cleanup()
		case <-fc.stopCleanup:
			fc.cleanupTicker.Stop()
			return
		}
	}
}

func (fc *FingerprintCache) cleanup() {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	
	now := time.Now()
	expired := 0
	
	for fingerprint, expiryTime := range fc.fingerprints {
		if now.After(expiryTime) {
			delete(fc.fingerprints, fingerprint)
			expired++
		}
	}
	
	if expired > 0 {
		fc.logger.Debug().
			Int("expired", expired).
			Int("remaining", len(fc.fingerprints)).
			Msg("Cleaned up expired fingerprints")
	}
}

func (fc *FingerprintCache) Stop() {
	close(fc.stopCleanup)
}

func (fc *FingerprintCache) Stats() FingerprintStats {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	
	return FingerprintStats{
		Count: len(fc.fingerprints),
		TTL:   fc.ttl,
	}
}

type FingerprintStats struct {
	Count int           `json:"count"`
	TTL   time.Duration `json:"ttl"`
}
