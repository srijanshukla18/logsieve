package dedup

import (
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestNewFingerprintCache(t *testing.T) {
	logger := zerolog.Nop()
	fc := NewFingerprintCache(time.Minute, logger)
	if fc == nil {
		t.Fatal("expected non-nil fingerprint cache")
	}
	defer fc.Stop()

	if fc.Size() != 0 {
		t.Errorf("expected size 0, got %d", fc.Size())
	}
}

func TestFingerprintCache_GetFingerprint(t *testing.T) {
	logger := zerolog.Nop()
	fc := NewFingerprintCache(time.Minute, logger)
	defer fc.Stop()

	fp1 := fc.GetFingerprint("test message 1")
	fp2 := fc.GetFingerprint("test message 2")
	fp3 := fc.GetFingerprint("test message 1")

	if fp1 == "" {
		t.Error("fingerprint should not be empty")
	}
	if fp1 == fp2 {
		t.Error("different messages should have different fingerprints")
	}
	if fp1 != fp3 {
		t.Error("same messages should have same fingerprints")
	}
	if len(fp1) != 64 { // SHA256 hex is 64 chars
		t.Errorf("expected fingerprint length 64, got %d", len(fp1))
	}
}

func TestFingerprintCache_Add(t *testing.T) {
	logger := zerolog.Nop()
	fc := NewFingerprintCache(time.Minute, logger)
	defer fc.Stop()

	fp := fc.GetFingerprint("test")
	fc.Add(fp)

	if fc.Size() != 1 {
		t.Errorf("expected size 1, got %d", fc.Size())
	}
}

func TestFingerprintCache_Exists(t *testing.T) {
	logger := zerolog.Nop()
	fc := NewFingerprintCache(time.Minute, logger)
	defer fc.Stop()

	fp := fc.GetFingerprint("test")

	if fc.Exists(fp) {
		t.Error("fingerprint should not exist before adding")
	}

	fc.Add(fp)

	if !fc.Exists(fp) {
		t.Error("fingerprint should exist after adding")
	}
}

func TestFingerprintCache_Expiry(t *testing.T) {
	logger := zerolog.Nop()
	fc := NewFingerprintCache(50*time.Millisecond, logger)
	defer fc.Stop()

	fp := fc.GetFingerprint("test")
	fc.Add(fp)

	if !fc.Exists(fp) {
		t.Error("fingerprint should exist immediately after adding")
	}

	time.Sleep(100 * time.Millisecond)

	if fc.Exists(fp) {
		t.Error("fingerprint should have expired")
	}
}

func TestFingerprintCache_Clear(t *testing.T) {
	logger := zerolog.Nop()
	fc := NewFingerprintCache(time.Minute, logger)
	defer fc.Stop()

	fc.Add(fc.GetFingerprint("test1"))
	fc.Add(fc.GetFingerprint("test2"))
	fc.Add(fc.GetFingerprint("test3"))

	if fc.Size() != 3 {
		t.Errorf("expected size 3, got %d", fc.Size())
	}

	fc.Clear()

	if fc.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", fc.Size())
	}
}

func TestFingerprintCache_Stats(t *testing.T) {
	logger := zerolog.Nop()
	ttl := time.Minute
	fc := NewFingerprintCache(ttl, logger)
	defer fc.Stop()

	fc.Add(fc.GetFingerprint("test"))

	stats := fc.Stats()
	if stats.Count != 1 {
		t.Errorf("expected count 1, got %d", stats.Count)
	}
	if stats.TTL != ttl {
		t.Errorf("expected TTL %v, got %v", ttl, stats.TTL)
	}
}

func TestFingerprintCache_Stop(t *testing.T) {
	logger := zerolog.Nop()
	fc := NewFingerprintCache(time.Minute, logger)

	fc.Stop()
	// Should not panic on double stop or after stop
}

func TestFingerprintCache_CleanupLoop(t *testing.T) {
	logger := zerolog.Nop()
	// TTL of 50ms means cleanup runs every 25ms
	fc := NewFingerprintCache(50*time.Millisecond, logger)
	defer fc.Stop()

	fp := fc.GetFingerprint("test")
	fc.Add(fp)

	// Wait for cleanup to run
	time.Sleep(100 * time.Millisecond)

	if fc.Size() != 0 {
		t.Errorf("expected size 0 after cleanup, got %d", fc.Size())
	}
}

func TestFingerprintCache_ConcurrentAccess(t *testing.T) {
	logger := zerolog.Nop()
	fc := NewFingerprintCache(time.Minute, logger)
	defer fc.Stop()

	done := make(chan bool)

	go func() {
		for i := 0; i < 100; i++ {
			fp := fc.GetFingerprint("message")
			fc.Add(fp)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			fp := fc.GetFingerprint("message")
			fc.Exists(fp)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 10; i++ {
			fc.Size()
			fc.Stats()
		}
		done <- true
	}()

	<-done
	<-done
	<-done
	// Just verify no panics
}

func TestFingerprintStats_Fields(t *testing.T) {
	stats := FingerprintStats{
		Count: 42,
		TTL:   time.Hour,
	}

	if stats.Count != 42 {
		t.Errorf("unexpected count: %d", stats.Count)
	}
	if stats.TTL != time.Hour {
		t.Errorf("unexpected TTL: %v", stats.TTL)
	}
}
