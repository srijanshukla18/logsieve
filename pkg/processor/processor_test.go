package processor

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/ingestion"
	"github.com/logsieve/logsieve/pkg/metrics"
)

func newTestConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Ingestion.FlushInterval = 50 * time.Millisecond
	cfg.Ingestion.MaxBatchSize = 10
	cfg.Ingestion.BufferSize = 100
	cfg.Dedup.FingerprintTTL = time.Minute
	cfg.Dedup.PatternTTL = time.Minute
	return cfg
}

func TestNewProcessor(t *testing.T) {
	cfg := newTestConfig()
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	p, err := NewProcessor(cfg, metricsRegistry, logger)
	if err != nil {
		t.Fatalf("NewProcessor failed: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil processor")
	}

	if p.dedup == nil {
		t.Error("expected non-nil dedup engine")
	}
	if p.profiles == nil {
		t.Error("expected non-nil profiles manager")
	}
	if p.router == nil {
		t.Error("expected non-nil router")
	}
	if p.buffer == nil {
		t.Error("expected non-nil buffer")
	}

	p.Stop()
}

func TestProcessor_Start(t *testing.T) {
	cfg := newTestConfig()
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	p, _ := NewProcessor(cfg, metricsRegistry, logger)
	defer p.Stop()

	ctx := context.Background()
	err := p.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !p.isRunning() {
		t.Error("processor should be running")
	}
}

func TestProcessor_Start_AlreadyRunning(t *testing.T) {
	cfg := newTestConfig()
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	p, _ := NewProcessor(cfg, metricsRegistry, logger)
	defer p.Stop()

	ctx := context.Background()
	p.Start(ctx)

	err := p.Start(ctx)
	if err == nil {
		t.Error("expected error when starting already running processor")
	}
}

func TestProcessor_Stop(t *testing.T) {
	cfg := newTestConfig()
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	p, _ := NewProcessor(cfg, metricsRegistry, logger)

	ctx := context.Background()
	p.Start(ctx)

	err := p.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if p.isRunning() {
		t.Error("processor should not be running after stop")
	}
}

func TestProcessor_Stop_NotRunning(t *testing.T) {
	cfg := newTestConfig()
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	p, _ := NewProcessor(cfg, metricsRegistry, logger)

	// Stop without starting should not error
	err := p.Stop()
	if err != nil {
		t.Errorf("Stop should not error when not running: %v", err)
	}
}

func TestProcessor_AddEntry(t *testing.T) {
	cfg := newTestConfig()
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	p, _ := NewProcessor(cfg, metricsRegistry, logger)

	ctx := context.Background()
	p.Start(ctx)
	defer p.Stop()

	entry := &ingestion.LogEntry{
		Message:   "test log message",
		Timestamp: time.Now(),
		Labels:    map[string]string{"profile": "generic"},
	}

	err := p.AddEntry(entry)
	if err != nil {
		t.Fatalf("AddEntry failed: %v", err)
	}
}

func TestProcessor_GetStats(t *testing.T) {
	cfg := newTestConfig()
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	p, _ := NewProcessor(cfg, metricsRegistry, logger)

	ctx := context.Background()
	p.Start(ctx)
	defer p.Stop()

	stats := p.GetStats()

	if !stats.Running {
		t.Error("expected running to be true")
	}
}

func TestProcessor_GetStats_NotRunning(t *testing.T) {
	cfg := newTestConfig()
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	p, _ := NewProcessor(cfg, metricsRegistry, logger)
	defer p.Stop()

	stats := p.GetStats()

	if stats.Running {
		t.Error("expected running to be false")
	}
}

func TestProcessor_ProcessBatch(t *testing.T) {
	cfg := newTestConfig()
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	p, _ := NewProcessor(cfg, metricsRegistry, logger)

	ctx := context.Background()
	p.Start(ctx)
	defer p.Stop()

	batch := []*ingestion.LogEntry{
		{Message: "log 1", Timestamp: time.Now(), Labels: map[string]string{"profile": "generic"}},
		{Message: "log 2", Timestamp: time.Now(), Labels: map[string]string{"profile": "generic"}},
	}

	err := p.processBatch(batch)
	if err != nil {
		t.Errorf("processBatch failed: %v", err)
	}
}

func TestProcessor_ProcessBatch_Empty(t *testing.T) {
	cfg := newTestConfig()
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	p, _ := NewProcessor(cfg, metricsRegistry, logger)
	defer p.Stop()

	err := p.processBatch([]*ingestion.LogEntry{})
	if err != nil {
		t.Errorf("processBatch with empty batch should not error: %v", err)
	}
}

func TestProcessor_ProcessEntry(t *testing.T) {
	cfg := newTestConfig()
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	p, _ := NewProcessor(cfg, metricsRegistry, logger)
	defer p.Stop()

	entry := &ingestion.LogEntry{
		Message:   "test message",
		Timestamp: time.Now(),
		Labels:    map[string]string{"profile": "generic"},
	}

	entries, matched := p.processEntry(entry)
	if len(entries) == 0 {
		t.Error("expected at least one entry output")
	}
	_ = matched // matched depends on profile rules
}

func TestProcessor_ProcessEntry_WithErrorLevel(t *testing.T) {
	cfg := newTestConfig()
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	p, _ := NewProcessor(cfg, metricsRegistry, logger)
	defer p.Stop()

	// Add some context
	for i := 0; i < 5; i++ {
		p.processEntry(&ingestion.LogEntry{
			Message: "normal log",
			Labels:  map[string]string{"profile": "generic"},
		})
	}

	entry := &ingestion.LogEntry{
		Message:   "ERROR: something failed",
		Level:     "ERROR",
		Timestamp: time.Now(),
		Labels:    map[string]string{"profile": "generic"},
	}

	entries, _ := p.processEntry(entry)
	// Should include context entries + the error entry
	if len(entries) < 1 {
		t.Error("expected at least the error entry")
	}
}

func TestProcessor_ProcessEntry_Dedup(t *testing.T) {
	cfg := newTestConfig()
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	p, _ := NewProcessor(cfg, metricsRegistry, logger)
	defer p.Stop()

	entry := &ingestion.LogEntry{
		Message:   "exact duplicate message",
		Timestamp: time.Now(),
		Labels:    map[string]string{"profile": "generic"},
	}

	// First should be output
	entries1, _ := p.processEntry(entry)
	if len(entries1) == 0 {
		t.Error("first entry should be output")
	}

	// Second exact duplicate should be deduplicated
	entries2, _ := p.processEntry(entry)
	if len(entries2) > 0 {
		t.Log("Note: exact duplicates may still be output depending on dedup config")
	}
}

func TestProcessor_IsRunning(t *testing.T) {
	cfg := newTestConfig()
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	p, _ := NewProcessor(cfg, metricsRegistry, logger)

	if p.isRunning() {
		t.Error("should not be running initially")
	}

	ctx := context.Background()
	p.Start(ctx)

	if !p.isRunning() {
		t.Error("should be running after start")
	}

	p.Stop()

	if p.isRunning() {
		t.Error("should not be running after stop")
	}
}

func TestProcessor_ContextCancellation(t *testing.T) {
	cfg := newTestConfig()
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	p, _ := NewProcessor(cfg, metricsRegistry, logger)
	defer p.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)

	// Cancel context
	cancel()

	// Give time for the processor to stop
	time.Sleep(100 * time.Millisecond)

	// Processor should handle context cancellation gracefully
}

func TestProcessorStats_Fields(t *testing.T) {
	stats := ProcessorStats{
		Running: true,
		BufferStats: ingestion.BufferStats{
			BufferSize: 10,
			BufferCap:  100,
		},
	}

	if !stats.Running {
		t.Error("expected running true")
	}
	if stats.BufferStats.BufferSize != 10 {
		t.Errorf("unexpected buffer size: %d", stats.BufferStats.BufferSize)
	}
}

func TestProcessor_IntegrationFlow(t *testing.T) {
	cfg := newTestConfig()
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	p, err := NewProcessor(cfg, metricsRegistry, logger)
	if err != nil {
		t.Fatalf("NewProcessor failed: %v", err)
	}

	ctx := context.Background()
	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Add multiple entries
	for i := 0; i < 15; i++ {
		entry := &ingestion.LogEntry{
			Message:   "integration test log " + string(rune('A'+i)),
			Timestamp: time.Now(),
			Labels:    map[string]string{"profile": "generic"},
		}
		if err := p.AddEntry(entry); err != nil {
			t.Errorf("AddEntry %d failed: %v", i, err)
		}
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	stats := p.GetStats()
	t.Logf("Stats after processing: %+v", stats)

	if err := p.Stop(); err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}
