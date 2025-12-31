package dedup

import (
	"sync"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/ingestion"
)

type ContextWindow struct {
	buffer     []*ingestion.LogEntry
	size       int
	contextLines int
	logger     zerolog.Logger
	mu         sync.RWMutex
}

func NewContextWindow(contextLines int, logger zerolog.Logger) *ContextWindow {
	return &ContextWindow{
		buffer:       make([]*ingestion.LogEntry, 0),
		size:         contextLines * 3, // Keep more entries for better context
		contextLines: contextLines,
		logger:       logger.With().Str("component", "context").Logger(),
	}
}

func (cw *ContextWindow) Add(entry *ingestion.LogEntry) {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	cw.buffer = append(cw.buffer, entry)
	
	if len(cw.buffer) > cw.size {
		cw.buffer = cw.buffer[1:]
	}
}

func (cw *ContextWindow) GetContext(triggerEntry *ingestion.LogEntry) []*ingestion.LogEntry {
    cw.mu.RLock()
    defer cw.mu.RUnlock()

    // Take last N entries as "before" context and append the trigger.
    if cw.contextLines <= 0 {
        return []*ingestion.LogEntry{triggerEntry}
    }

    beforeCount := cw.contextLines
    if beforeCount > len(cw.buffer) {
        beforeCount = len(cw.buffer)
    }

    startIndex := len(cw.buffer) - beforeCount
    context := make([]*ingestion.LogEntry, 0, beforeCount+1)

    for i := startIndex; i < len(cw.buffer); i++ {
        entryCopy := *cw.buffer[i]
        if entryCopy.Labels == nil {
            entryCopy.Labels = make(map[string]string)
        }
        entryCopy.Labels["context_position"] = "before"
        context = append(context, &entryCopy)
    }

    // Append the trigger entry as the last item in context set
    triggerCopy := *triggerEntry
    if triggerCopy.Labels == nil {
        triggerCopy.Labels = make(map[string]string)
    }
    triggerCopy.Labels["context_trigger"] = "true"
    context = append(context, &triggerCopy)

    return context
}

func (cw *ContextWindow) Size() int {
	cw.mu.RLock()
	defer cw.mu.RUnlock()
	
	return len(cw.buffer)
}

func (cw *ContextWindow) Clear() {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	
	cw.buffer = cw.buffer[:0]
}

func (cw *ContextWindow) GetRecentEntries(count int) []*ingestion.LogEntry {
	cw.mu.RLock()
	defer cw.mu.RUnlock()

	if count <= 0 || len(cw.buffer) == 0 {
		return nil
	}

	startIndex := len(cw.buffer) - count
	if startIndex < 0 {
		startIndex = 0
	}

	recent := make([]*ingestion.LogEntry, 0, len(cw.buffer)-startIndex)
	for i := startIndex; i < len(cw.buffer); i++ {
		recent = append(recent, cw.buffer[i])
	}

	return recent
}

type ContextStats struct {
	BufferSize    int `json:"buffer_size"`
	BufferCap     int `json:"buffer_capacity"`
	ContextLines  int `json:"context_lines"`
}

func (cw *ContextWindow) Stats() ContextStats {
	cw.mu.RLock()
	defer cw.mu.RUnlock()
	
	return ContextStats{
		BufferSize:   len(cw.buffer),
		BufferCap:    cw.size,
		ContextLines: cw.contextLines,
	}
}
