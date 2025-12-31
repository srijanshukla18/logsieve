package dedup

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/ingestion"
)

func TestNewContextWindow(t *testing.T) {
	logger := zerolog.Nop()
	cw := NewContextWindow(5, logger)
	if cw == nil {
		t.Fatal("expected non-nil context window")
	}

	if cw.Size() != 0 {
		t.Errorf("expected size 0, got %d", cw.Size())
	}
}

func TestContextWindow_Add(t *testing.T) {
	logger := zerolog.Nop()
	cw := NewContextWindow(5, logger)

	entry := &ingestion.LogEntry{
		Message:   "test message",
		Timestamp: time.Now(),
	}

	cw.Add(entry)

	if cw.Size() != 1 {
		t.Errorf("expected size 1, got %d", cw.Size())
	}
}

func TestContextWindow_Add_MaxSize(t *testing.T) {
	logger := zerolog.Nop()
	cw := NewContextWindow(2, logger) // size = contextLines * 3 = 6

	for i := 0; i < 10; i++ {
		cw.Add(&ingestion.LogEntry{Message: "test"})
	}

	// Should be capped at contextLines * 3 = 6
	if cw.Size() > 6 {
		t.Errorf("size should be capped at 6, got %d", cw.Size())
	}
}

func TestContextWindow_GetContext(t *testing.T) {
	logger := zerolog.Nop()
	cw := NewContextWindow(3, logger)

	for i := 0; i < 5; i++ {
		cw.Add(&ingestion.LogEntry{
			Message:   "message " + string(rune('A'+i)),
			Timestamp: time.Now(),
		})
	}

	trigger := &ingestion.LogEntry{
		Message: "TRIGGER",
		Level:   "ERROR",
	}

	context := cw.GetContext(trigger)

	// Should have up to contextLines before + trigger
	if len(context) > 4 { // 3 before + 1 trigger
		t.Errorf("context should have at most 4 entries, got %d", len(context))
	}

	// Last entry should be trigger
	if len(context) > 0 {
		last := context[len(context)-1]
		if last.Labels["context_trigger"] != "true" {
			t.Error("last entry should be marked as trigger")
		}
	}

	// Before entries should be marked
	for i := 0; i < len(context)-1; i++ {
		if context[i].Labels["context_position"] != "before" {
			t.Errorf("entry %d should be marked as before", i)
		}
	}
}

func TestContextWindow_GetContext_Empty(t *testing.T) {
	logger := zerolog.Nop()
	cw := NewContextWindow(3, logger)

	trigger := &ingestion.LogEntry{Message: "TRIGGER"}
	context := cw.GetContext(trigger)

	if len(context) != 1 {
		t.Errorf("expected 1 entry (just trigger), got %d", len(context))
	}
}

func TestContextWindow_GetContext_ZeroLines(t *testing.T) {
	logger := zerolog.Nop()
	cw := NewContextWindow(0, logger)

	cw.Add(&ingestion.LogEntry{Message: "before"})

	trigger := &ingestion.LogEntry{Message: "TRIGGER"}
	context := cw.GetContext(trigger)

	if len(context) != 1 {
		t.Errorf("expected 1 entry with zero context lines, got %d", len(context))
	}
}

func TestContextWindow_Clear(t *testing.T) {
	logger := zerolog.Nop()
	cw := NewContextWindow(5, logger)

	cw.Add(&ingestion.LogEntry{Message: "test1"})
	cw.Add(&ingestion.LogEntry{Message: "test2"})

	cw.Clear()

	if cw.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", cw.Size())
	}
}

func TestContextWindow_GetRecentEntries(t *testing.T) {
	logger := zerolog.Nop()
	cw := NewContextWindow(5, logger)

	for i := 0; i < 5; i++ {
		cw.Add(&ingestion.LogEntry{
			Message: "message " + string(rune('0'+i)),
		})
	}

	recent := cw.GetRecentEntries(3)
	if len(recent) != 3 {
		t.Errorf("expected 3 recent entries, got %d", len(recent))
	}
}

func TestContextWindow_GetRecentEntries_MoreThanAvailable(t *testing.T) {
	logger := zerolog.Nop()
	cw := NewContextWindow(5, logger)

	cw.Add(&ingestion.LogEntry{Message: "test"})

	recent := cw.GetRecentEntries(10)
	if len(recent) != 1 {
		t.Errorf("expected 1 entry, got %d", len(recent))
	}
}

func TestContextWindow_GetRecentEntries_Empty(t *testing.T) {
	logger := zerolog.Nop()
	cw := NewContextWindow(5, logger)

	recent := cw.GetRecentEntries(5)
	if recent != nil {
		t.Errorf("expected nil for empty buffer, got %v", recent)
	}
}

func TestContextWindow_GetRecentEntries_ZeroCount(t *testing.T) {
	logger := zerolog.Nop()
	cw := NewContextWindow(5, logger)

	cw.Add(&ingestion.LogEntry{Message: "test"})

	recent := cw.GetRecentEntries(0)
	if recent != nil {
		t.Errorf("expected nil for zero count, got %v", recent)
	}
}

func TestContextWindow_Stats(t *testing.T) {
	logger := zerolog.Nop()
	cw := NewContextWindow(5, logger)

	cw.Add(&ingestion.LogEntry{Message: "test"})
	cw.Add(&ingestion.LogEntry{Message: "test2"})

	stats := cw.Stats()

	if stats.BufferSize != 2 {
		t.Errorf("expected buffer size 2, got %d", stats.BufferSize)
	}
	if stats.BufferCap != 15 { // contextLines * 3
		t.Errorf("expected buffer cap 15, got %d", stats.BufferCap)
	}
	if stats.ContextLines != 5 {
		t.Errorf("expected context lines 5, got %d", stats.ContextLines)
	}
}

func TestContextStats_Fields(t *testing.T) {
	stats := ContextStats{
		BufferSize:   10,
		BufferCap:    30,
		ContextLines: 10,
	}

	if stats.BufferSize != 10 {
		t.Errorf("unexpected buffer size: %d", stats.BufferSize)
	}
	if stats.BufferCap != 30 {
		t.Errorf("unexpected buffer cap: %d", stats.BufferCap)
	}
	if stats.ContextLines != 10 {
		t.Errorf("unexpected context lines: %d", stats.ContextLines)
	}
}

func TestContextWindow_PreservesLabels(t *testing.T) {
	logger := zerolog.Nop()
	cw := NewContextWindow(3, logger)

	entry := &ingestion.LogEntry{
		Message: "test",
		Labels:  map[string]string{"existing": "value"},
	}
	cw.Add(entry)

	trigger := &ingestion.LogEntry{
		Message: "TRIGGER",
		Labels:  map[string]string{"trigger_label": "trigger_value"},
	}

	context := cw.GetContext(trigger)

	// Check that original labels are preserved
	if len(context) >= 2 {
		if context[0].Labels["existing"] != "value" {
			t.Error("existing labels should be preserved")
		}
	}
}

func TestContextWindow_NilLabels(t *testing.T) {
	logger := zerolog.Nop()
	cw := NewContextWindow(3, logger)

	entry := &ingestion.LogEntry{Message: "test", Labels: nil}
	cw.Add(entry)

	trigger := &ingestion.LogEntry{Message: "TRIGGER", Labels: nil}
	context := cw.GetContext(trigger)

	// Should not panic with nil labels
	for _, e := range context {
		if e.Labels == nil {
			t.Error("labels should be initialized")
		}
	}
}
