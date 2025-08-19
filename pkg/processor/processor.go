package processor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/dedup"
	"github.com/logsieve/logsieve/pkg/ingestion"
	"github.com/logsieve/logsieve/pkg/metrics"
	"github.com/logsieve/logsieve/pkg/output"
	"github.com/logsieve/logsieve/pkg/profiles"
)

type Processor struct {
	config         *config.Config
	logger         zerolog.Logger
	metrics        *metrics.Registry
	dedup          *dedup.Engine
	profiles       *profiles.Manager
	router         *output.Router
	buffer         *ingestion.Buffer
	running        bool
	stopChan       chan struct{}
	wg             sync.WaitGroup
	mu             sync.RWMutex
}

func NewProcessor(cfg *config.Config, metrics *metrics.Registry, logger zerolog.Logger) (*Processor, error) {
	p := &Processor{
		config:   cfg,
		logger:   logger.With().Str("component", "processor").Logger(),
		metrics:  metrics,
		stopChan: make(chan struct{}),
	}

	if err := p.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize processor: %w", err)
	}

	return p, nil
}

func (p *Processor) initialize() error {
	p.dedup = dedup.NewEngine(p.config.Dedup, p.metrics, p.logger)
	
	profileManager := profiles.NewManager(p.config.Profiles, p.logger)
	if err := profileManager.LoadProfiles(); err != nil {
		return fmt.Errorf("failed to load profiles: %w", err)
	}
	p.profiles = profileManager

	router, err := output.NewRouter(p.config.Outputs, p.metrics, p.logger)
	if err != nil {
		return fmt.Errorf("failed to create output router: %w", err)
	}
	p.router = router

	p.buffer = ingestion.NewBuffer(p.config.Ingestion, p.logger)

	return nil
}

func (p *Processor) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return fmt.Errorf("processor already running")
	}
	p.running = true
	p.mu.Unlock()

	p.logger.Info().Msg("Starting log processor")

	p.wg.Add(1)
	go p.processingLoop(ctx)

	return nil
}

func (p *Processor) Stop() error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return nil
	}
	p.running = false
	p.mu.Unlock()

	p.logger.Info().Msg("Stopping log processor")

	close(p.stopChan)
	p.wg.Wait()

	if err := p.buffer.Close(); err != nil {
		p.logger.Error().Err(err).Msg("Error closing buffer")
	}

	if err := p.router.Close(); err != nil {
		p.logger.Error().Err(err).Msg("Error closing router")
	}

	return nil
}

func (p *Processor) processingLoop(ctx context.Context) {
	defer p.wg.Done()

	batches := p.buffer.GetBatch()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info().Msg("Context cancelled, stopping processor")
			return
		case <-p.stopChan:
			p.logger.Info().Msg("Stop signal received")
			return
		case batch, ok := <-batches:
			if !ok {
				p.logger.Info().Msg("Batch channel closed")
				return
			}
			
			if err := p.processBatch(batch); err != nil {
				p.logger.Error().Err(err).Msg("Error processing batch")
			}
		}
	}
}

func (p *Processor) processBatch(batch []*ingestion.LogEntry) error {
	if len(batch) == 0 {
		return nil
	}

	start := time.Now()
	defer func() {
		p.logger.Debug().
			Int("batch_size", len(batch)).
			Dur("duration", time.Since(start)).
			Msg("Processed batch")
	}()

	var outputEntries []*ingestion.LogEntry

	for _, entry := range batch {
		processedEntry, shouldOutput := p.processEntry(entry)
		if shouldOutput {
			outputEntries = append(outputEntries, processedEntry)
		}
	}

	if len(outputEntries) > 0 {
		if err := p.router.Route(outputEntries); err != nil {
			return fmt.Errorf("failed to route entries: %w", err)
		}
	}

	reductionRatio := 1.0 - (float64(len(outputEntries)) / float64(len(batch)))
	p.logger.Debug().
		Int("input", len(batch)).
		Int("output", len(outputEntries)).
		Float64("reduction", reductionRatio).
		Msg("Batch processing completed")

	return nil
}

func (p *Processor) processEntry(entry *ingestion.LogEntry) (*ingestion.LogEntry, bool) {
	profileName := p.profiles.DetectProfile(entry)
	
	processed, err := p.profiles.ProcessWithProfile(entry, profileName)
	if err != nil {
		p.logger.Error().Err(err).Str("profile", profileName).Msg("Profile processing error")
		return entry, true
	}

	if processed.Drop {
		return nil, false
	}

	dedupResult, err := p.dedup.Process(entry)
	if err != nil {
		p.logger.Error().Err(err).Msg("Dedup processing error")
		return entry, true
	}

	if dedupResult.IsDuplicate && !dedupResult.ShouldOutput {
		return nil, false
	}

	if len(dedupResult.Context) > 0 {
		contextEntries := make([]*ingestion.LogEntry, len(dedupResult.Context))
		copy(contextEntries, dedupResult.Context)
		
		for _, contextEntry := range contextEntries {
			if contextEntry != entry {
				go func(ce *ingestion.LogEntry) {
					if err := p.router.Route([]*ingestion.LogEntry{ce}); err != nil {
						p.logger.Error().Err(err).Msg("Failed to route context entry")
					}
				}(contextEntry)
			}
		}
	}

	return entry, true
}

func (p *Processor) AddEntry(entry *ingestion.LogEntry) error {
	return p.buffer.Add(entry)
}

func (p *Processor) GetStats() ProcessorStats {
	bufferStats := p.buffer.Stats()
	dedupStats := p.dedup.GetStats()
	profileStats := p.profiles.GetStats()
	routerStats := p.router.Stats()

	return ProcessorStats{
		Running:       p.isRunning(),
		BufferStats:   bufferStats,
		DedupStats:    dedupStats,
		ProfileStats:  profileStats,
		RouterStats:   routerStats,
	}
}

func (p *Processor) isRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.running
}

type ProcessorStats struct {
	Running       bool                    `json:"running"`
	BufferStats   ingestion.BufferStats   `json:"buffer_stats"`
	DedupStats    dedup.Stats             `json:"dedup_stats"`
	ProfileStats  profiles.ManagerStats   `json:"profile_stats"`
	RouterStats   output.RouterStats      `json:"router_stats"`
}