package ingestion

import (
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/metrics"
)

func newTestBuffer() *Buffer {
	cfg := config.IngestionConfig{
		MaxBatchSize:  10,
		FlushInterval: 100 * time.Millisecond,
		BufferSize:    100,
	}
	logger := zerolog.Nop()
	return NewBuffer(cfg, logger)
}

func TestNewBuffer(t *testing.T) {
	buf := newTestBuffer()
	if buf == nil {
		t.Fatal("expected non-nil buffer")
	}
	defer buf.Close()

	stats := buf.Stats()
	if stats.BufferSize != 0 {
		t.Errorf("expected buffer size 0, got %d", stats.BufferSize)
	}
	if stats.BufferCap != 100 {
		t.Errorf("expected buffer capacity 100, got %d", stats.BufferCap)
	}
}

func TestBuffer_Add(t *testing.T) {
	buf := newTestBuffer()
	defer buf.Close()

	entry := &LogEntry{
		Message:   "test log",
		Timestamp: time.Now(),
	}

	err := buf.Add(entry)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	stats := buf.Stats()
	if stats.BufferSize != 1 {
		t.Errorf("expected buffer size 1, got %d", stats.BufferSize)
	}
}

func TestBuffer_Add_Full(t *testing.T) {
	cfg := config.IngestionConfig{
		MaxBatchSize:  10,
		FlushInterval: 10 * time.Second, // Long interval to prevent flushing
		BufferSize:    2,                // Very small buffer
	}
	logger := zerolog.Nop()
	buf := NewBuffer(cfg, logger)
	defer buf.Close()

	entry := &LogEntry{Message: "test"}

	buf.Add(entry)
	buf.Add(entry)

	// Third add should fail - buffer is full
	err := buf.Add(entry)
	if err == nil {
		t.Error("expected error when buffer is full")
	}
}

func TestBuffer_Add_Closed(t *testing.T) {
	buf := newTestBuffer()
	buf.Close()

	entry := &LogEntry{Message: "test"}
	err := buf.Add(entry)
	if err == nil {
		t.Error("expected error adding to closed buffer")
	}
}

func TestBuffer_GetBatch(t *testing.T) {
	buf := newTestBuffer()
	defer buf.Close()

	batches := buf.GetBatch()
	if batches == nil {
		t.Error("expected non-nil batch channel")
	}
}

func TestBuffer_BatchFlush_BySize(t *testing.T) {
	cfg := config.IngestionConfig{
		MaxBatchSize:  3,
		FlushInterval: 10 * time.Second,
		BufferSize:    100,
	}
	logger := zerolog.Nop()
	buf := NewBuffer(cfg, logger)
	defer buf.Close()

	batches := buf.GetBatch()

	for i := 0; i < 3; i++ {
		buf.Add(&LogEntry{Message: "test"})
	}

	select {
	case batch := <-batches:
		if len(batch) != 3 {
			t.Errorf("expected batch of 3, got %d", len(batch))
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for batch")
	}
}

func TestBuffer_BatchFlush_ByInterval(t *testing.T) {
	cfg := config.IngestionConfig{
		MaxBatchSize:  100,
		FlushInterval: 50 * time.Millisecond,
		BufferSize:    100,
	}
	logger := zerolog.Nop()
	buf := NewBuffer(cfg, logger)
	defer buf.Close()

	batches := buf.GetBatch()

	buf.Add(&LogEntry{Message: "test1"})
	buf.Add(&LogEntry{Message: "test2"})

	select {
	case batch := <-batches:
		if len(batch) != 2 {
			t.Errorf("expected batch of 2, got %d", len(batch))
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout waiting for batch flush by interval")
	}
}

func TestBuffer_Close(t *testing.T) {
	buf := newTestBuffer()

	err := buf.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Double close should be safe
	err = buf.Close()
	if err != nil {
		t.Errorf("Double close should not error: %v", err)
	}
}

func TestBuffer_Close_FlushesRemaining(t *testing.T) {
	cfg := config.IngestionConfig{
		MaxBatchSize:  100,
		FlushInterval: 10 * time.Second,
		BufferSize:    100,
	}
	logger := zerolog.Nop()
	buf := NewBuffer(cfg, logger)

	batches := buf.GetBatch()

	buf.Add(&LogEntry{Message: "test"})

	buf.Close()

	select {
	case batch := <-batches:
		if len(batch) != 1 {
			t.Errorf("expected final batch of 1, got %d", len(batch))
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for final flush")
	}
}

func TestBuffer_WithMetrics(t *testing.T) {
	cfg := config.IngestionConfig{
		MaxBatchSize:  10,
		FlushInterval: 100 * time.Millisecond,
		BufferSize:    100,
	}
	logger := zerolog.Nop()
	metricsRegistry := metrics.NewRegistry()

	buf := NewBuffer(cfg, logger).WithMetrics(metricsRegistry, "test")
	defer buf.Close()

	if buf.metrics != metricsRegistry {
		t.Error("metrics not set correctly")
	}
	if buf.name != "test" {
		t.Errorf("expected name test, got %s", buf.name)
	}
}

func TestBuffer_Stats(t *testing.T) {
	buf := newTestBuffer()
	defer buf.Close()

	buf.Add(&LogEntry{Message: "test1"})
	buf.Add(&LogEntry{Message: "test2"})

	stats := buf.Stats()

	if stats.BufferSize != 2 {
		t.Errorf("expected buffer size 2, got %d", stats.BufferSize)
	}
	if stats.BufferCap != 100 {
		t.Errorf("expected buffer capacity 100, got %d", stats.BufferCap)
	}
	if stats.BatchQueueCap != 100 {
		t.Errorf("expected batch queue capacity 100, got %d", stats.BatchQueueCap)
	}
}

func TestBuffer_ConcurrentAdd(t *testing.T) {
	cfg := config.IngestionConfig{
		MaxBatchSize:  10,
		FlushInterval: 50 * time.Millisecond,
		BufferSize:    1000,
	}
	logger := zerolog.Nop()
	buf := NewBuffer(cfg, logger)
	defer buf.Close()

	var wg sync.WaitGroup
	numGoroutines := 10
	entriesPerGoroutine := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < entriesPerGoroutine; j++ {
				buf.Add(&LogEntry{Message: "concurrent test"})
			}
		}(i)
	}

	wg.Wait()
	// Just verify no panics occurred
}

func TestNewBufferer_Memory(t *testing.T) {
	cfg := config.IngestionConfig{
		MaxBatchSize:  10,
		FlushInterval: 100 * time.Millisecond,
		BufferSize:    100,
		QueueType:     "memory",
	}
	logger := zerolog.Nop()
	metricsRegistry := metrics.NewRegistry()

	buf := NewBufferer(cfg, metricsRegistry, logger)
	defer buf.Close()

	_, ok := buf.(*Buffer)
	if !ok {
		t.Error("expected *Buffer for memory queue type")
	}
}

func TestNewBufferer_Disk(t *testing.T) {
	cfg := config.IngestionConfig{
		MaxBatchSize:  10,
		FlushInterval: 100 * time.Millisecond,
		BufferSize:    100,
		QueueType:     "disk",
		DiskPath:      t.TempDir(),
		MaxDiskBytes:  1024 * 1024,
	}
	logger := zerolog.Nop()
	metricsRegistry := metrics.NewRegistry()

	buf := NewBufferer(cfg, metricsRegistry, logger)
	defer buf.Close()

	_, ok := buf.(*DiskBuffer)
	if !ok {
		t.Error("expected *DiskBuffer for disk queue type")
	}
}

func TestBufferStats_Fields(t *testing.T) {
	stats := BufferStats{
		BufferSize:     10,
		BufferCap:      100,
		BatchQueueSize: 5,
		BatchQueueCap:  50,
	}

	if stats.BufferSize != 10 {
		t.Errorf("unexpected buffer size: %d", stats.BufferSize)
	}
	if stats.BufferCap != 100 {
		t.Errorf("unexpected buffer cap: %d", stats.BufferCap)
	}
	if stats.BatchQueueSize != 5 {
		t.Errorf("unexpected batch queue size: %d", stats.BatchQueueSize)
	}
	if stats.BatchQueueCap != 50 {
		t.Errorf("unexpected batch queue cap: %d", stats.BatchQueueCap)
	}
}
