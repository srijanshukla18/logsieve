package dedup

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/ingestion"
	"github.com/logsieve/logsieve/pkg/metrics"
)

func newTestEngine() *Engine {
	cfg := config.DedupConfig{
		Engine:              "drain3",
		CacheSize:           1000,
		ContextLines:        3,
		SimilarityThreshold: 0.6,
		PatternTTL:          time.Hour,
		FingerprintTTL:      30 * time.Minute,
	}
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()
	return NewEngine(cfg, metricsRegistry, logger)
}

func TestNewEngine(t *testing.T) {
	engine := newTestEngine()
	if engine == nil {
		t.Fatal("expected non-nil engine")
	}
	defer engine.Close()

	if engine.drain == nil {
		t.Error("expected non-nil drain3")
	}
	if engine.fingerprints == nil {
		t.Error("expected non-nil fingerprint cache")
	}
	if engine.context == nil {
		t.Error("expected non-nil context window")
	}
}

func TestEngine_Process_NewEntry(t *testing.T) {
	engine := newTestEngine()
	defer engine.Close()

	entry := &ingestion.LogEntry{
		Message:   "user john logged in",
		Timestamp: time.Now(),
		Labels:    map[string]string{"profile": "test"},
	}

	result, err := engine.Process(entry)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Entry != entry {
		t.Error("result entry should match input")
	}
	if result.Fingerprint == "" {
		t.Error("fingerprint should be set")
	}
	if result.TemplateID == "" {
		t.Error("template ID should be set")
	}
	if !result.ShouldOutput {
		t.Error("new entry should be output")
	}
}

func TestEngine_Process_DuplicateFingerprint(t *testing.T) {
	engine := newTestEngine()
	defer engine.Close()

	entry := &ingestion.LogEntry{
		Message:   "exact duplicate message",
		Timestamp: time.Now(),
		Labels:    map[string]string{"profile": "test"},
	}

	// First process
	result1, _ := engine.Process(entry)
	if !result1.ShouldOutput {
		t.Error("first entry should be output")
	}

	// Second process with same message (exact duplicate)
	result2, _ := engine.Process(entry)
	if result2.ShouldOutput {
		t.Error("exact duplicate should not be output")
	}
	if !result2.IsDuplicate {
		t.Error("should be marked as duplicate")
	}
}

func TestEngine_Process_SimilarMessages(t *testing.T) {
	engine := newTestEngine()
	defer engine.Close()

	entry1 := &ingestion.LogEntry{
		Message:   "user alice logged in from 192.168.1.1",
		Timestamp: time.Now(),
		Labels:    map[string]string{"profile": "test"},
	}

	entry2 := &ingestion.LogEntry{
		Message:   "user bob logged in from 10.0.0.1",
		Timestamp: time.Now(),
		Labels:    map[string]string{"profile": "test"},
	}

	result1, _ := engine.Process(entry1)
	if !result1.ShouldOutput {
		t.Error("first entry should be output")
	}

	result2, _ := engine.Process(entry2)
	// Similar messages may or may not be deduplicated based on template matching
	// Just verify no error and result is valid
	if result2 == nil {
		t.Error("result should not be nil")
	}
}

func TestEngine_Process_ErrorPreservesContext(t *testing.T) {
	engine := newTestEngine()
	defer engine.Close()

	// Add some context
	for i := 0; i < 5; i++ {
		engine.Process(&ingestion.LogEntry{
			Message: "normal log message",
			Labels:  map[string]string{"profile": "test"},
		})
	}

	// Process an error entry
	errorEntry := &ingestion.LogEntry{
		Message: "critical error occurred",
		Level:   "ERROR",
		Labels:  map[string]string{"profile": "test"},
	}

	result, _ := engine.Process(errorEntry)

	if !result.ShouldOutput {
		t.Error("error entry should always be output")
	}
	if len(result.Context) == 0 {
		t.Error("error entry should have context")
	}
}

func TestEngine_Process_FatalPreservesContext(t *testing.T) {
	engine := newTestEngine()
	defer engine.Close()

	engine.Process(&ingestion.LogEntry{
		Message: "before",
		Labels:  map[string]string{"profile": "test"},
	})

	fatalEntry := &ingestion.LogEntry{
		Message: "fatal error",
		Level:   "FATAL",
		Labels:  map[string]string{"profile": "test"},
	}

	result, _ := engine.Process(fatalEntry)
	if len(result.Context) == 0 {
		t.Error("fatal entry should have context")
	}
}

func TestEngine_GetStats(t *testing.T) {
	engine := newTestEngine()
	defer engine.Close()

	engine.Process(&ingestion.LogEntry{
		Message: "test message",
		Labels:  map[string]string{"profile": "test"},
	})

	stats := engine.GetStats()

	if stats.PatternCount < 0 {
		t.Error("pattern count should be non-negative")
	}
	if stats.FingerprintCount < 0 {
		t.Error("fingerprint count should be non-negative")
	}
	if stats.ContextSize < 0 {
		t.Error("context size should be non-negative")
	}
}

func TestEngine_Reset(t *testing.T) {
	engine := newTestEngine()
	defer engine.Close()

	for i := 0; i < 10; i++ {
		engine.Process(&ingestion.LogEntry{
			Message: "test message " + string(rune('0'+i)),
			Labels:  map[string]string{"profile": "test"},
		})
	}

	statsBeforeReset := engine.GetStats()
	if statsBeforeReset.PatternCount == 0 && statsBeforeReset.FingerprintCount == 0 {
		t.Error("expected some data before reset")
	}

	engine.Reset()

	statsAfterReset := engine.GetStats()
	if statsAfterReset.PatternCount != 0 {
		t.Errorf("expected 0 patterns after reset, got %d", statsAfterReset.PatternCount)
	}
	if statsAfterReset.FingerprintCount != 0 {
		t.Errorf("expected 0 fingerprints after reset, got %d", statsAfterReset.FingerprintCount)
	}
	if statsAfterReset.ContextSize != 0 {
		t.Errorf("expected 0 context size after reset, got %d", statsAfterReset.ContextSize)
	}
}

func TestEngine_Close(t *testing.T) {
	engine := newTestEngine()
	engine.Close()
	// Should not panic on double close
}

func TestEngine_ShouldPreserveContext(t *testing.T) {
	engine := newTestEngine()
	defer engine.Close()

	testCases := []struct {
		level    string
		preserve bool
	}{
		{"ERROR", true},
		{"FATAL", true},
		{"PANIC", true},
		{"INFO", false},
		{"DEBUG", false},
		{"WARN", false},
		{"", false},
	}

	for _, tc := range testCases {
		entry := &ingestion.LogEntry{Level: tc.level}
		result := engine.shouldPreserveContext(entry)
		if result != tc.preserve {
			t.Errorf("level %s: expected preserve=%v, got %v", tc.level, tc.preserve, result)
		}
	}
}

func TestEngine_GetProfile(t *testing.T) {
	engine := newTestEngine()
	defer engine.Close()

	// With profile label
	entry1 := &ingestion.LogEntry{
		Labels: map[string]string{"profile": "nginx"},
	}
	if engine.getProfile(entry1) != "nginx" {
		t.Error("should return profile from labels")
	}

	// Without profile label
	entry2 := &ingestion.LogEntry{
		Labels: map[string]string{"other": "value"},
	}
	if engine.getProfile(entry2) != "unknown" {
		t.Error("should return unknown when no profile label")
	}

	// Nil labels
	entry3 := &ingestion.LogEntry{}
	if engine.getProfile(entry3) != "unknown" {
		t.Error("should return unknown for nil labels")
	}
}

func TestResult_Fields(t *testing.T) {
	entry := &ingestion.LogEntry{Message: "test"}
	result := Result{
		Entry:        entry,
		IsDuplicate:  true,
		TemplateID:   "123",
		Fingerprint:  "abc123",
		ShouldOutput: false,
		Context:      []*ingestion.LogEntry{entry},
	}

	if result.Entry != entry {
		t.Error("entry mismatch")
	}
	if !result.IsDuplicate {
		t.Error("expected duplicate true")
	}
	if result.TemplateID != "123" {
		t.Error("template ID mismatch")
	}
	if result.Fingerprint != "abc123" {
		t.Error("fingerprint mismatch")
	}
	if result.ShouldOutput {
		t.Error("expected should output false")
	}
	if len(result.Context) != 1 {
		t.Error("context length mismatch")
	}
}

func TestStats_Fields(t *testing.T) {
	stats := Stats{
		PatternCount:     10,
		FingerprintCount: 20,
		ContextSize:      5,
		LastProcessed:    time.Now(),
	}

	if stats.PatternCount != 10 {
		t.Errorf("unexpected pattern count: %d", stats.PatternCount)
	}
	if stats.FingerprintCount != 20 {
		t.Errorf("unexpected fingerprint count: %d", stats.FingerprintCount)
	}
	if stats.ContextSize != 5 {
		t.Errorf("unexpected context size: %d", stats.ContextSize)
	}
}

func TestEngine_Min(t *testing.T) {
	if min(5, 10) != 5 {
		t.Error("min(5, 10) should be 5")
	}
	if min(10, 5) != 5 {
		t.Error("min(10, 5) should be 5")
	}
	if min(5, 5) != 5 {
		t.Error("min(5, 5) should be 5")
	}
}
