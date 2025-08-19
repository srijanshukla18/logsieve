package ingestion

import (
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
)

type Buffer struct {
	config   config.IngestionConfig
	logger   zerolog.Logger
	buffer   chan *LogEntry
	batches  chan []*LogEntry
	mu       sync.RWMutex
	closed   bool
	wg       sync.WaitGroup
}

func NewBuffer(config config.IngestionConfig, logger zerolog.Logger) *Buffer {
	b := &Buffer{
		config:  config,
		logger:  logger.With().Str("component", "buffer").Logger(),
		buffer:  make(chan *LogEntry, config.BufferSize),
		batches: make(chan []*LogEntry, 100),
	}

	b.wg.Add(1)
	go b.batchProcessor()

	return b
}

func (b *Buffer) Add(entry *LogEntry) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return fmt.Errorf("buffer is closed")
	}
	b.mu.RUnlock()

	select {
	case b.buffer <- entry:
		return nil
	default:
		return fmt.Errorf("buffer is full")
	}
}

func (b *Buffer) GetBatch() <-chan []*LogEntry {
	return b.batches
}

func (b *Buffer) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	
	b.closed = true
	close(b.buffer)
	b.mu.Unlock()

	b.wg.Wait()
	close(b.batches)

	return nil
}

func (b *Buffer) batchProcessor() {
	defer b.wg.Done()

	batch := make([]*LogEntry, 0, b.config.MaxBatchSize)
	ticker := time.NewTicker(b.config.FlushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) > 0 {
			batchCopy := make([]*LogEntry, len(batch))
			copy(batchCopy, batch)
			
			select {
			case b.batches <- batchCopy:
				b.logger.Debug().Int("batch_size", len(batchCopy)).Msg("Flushed batch")
			default:
				b.logger.Warn().Int("batch_size", len(batchCopy)).Msg("Batch channel full, dropping batch")
			}
			
			batch = batch[:0]
		}
	}

	for {
		select {
		case entry, ok := <-b.buffer:
			if !ok {
				flush()
				return
			}

			batch = append(batch, entry)
			
			if len(batch) >= b.config.MaxBatchSize {
				flush()
			}

		case <-ticker.C:
			flush()
		}
	}
}

func (b *Buffer) Stats() BufferStats {
	return BufferStats{
		BufferSize:    len(b.buffer),
		BufferCap:     cap(b.buffer),
		BatchQueueSize: len(b.batches),
		BatchQueueCap:  cap(b.batches),
	}
}

type BufferStats struct {
	BufferSize     int `json:"buffer_size"`
	BufferCap      int `json:"buffer_capacity"`
	BatchQueueSize int `json:"batch_queue_size"`
	BatchQueueCap  int `json:"batch_queue_capacity"`
}