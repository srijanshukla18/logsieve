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
	buffer         ingestion.Bufferer
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
	
    profileManager := profiles.NewManager(p.config.Profiles, p.metrics, p.logger)
	if err := profileManager.LoadProfiles(); err != nil {
		return fmt.Errorf("failed to load profiles: %w", err)
	}
	p.profiles = profileManager

	router, err := output.NewRouter(p.config.Outputs, p.metrics, p.logger)
	if err != nil {
		return fmt.Errorf("failed to create output router: %w", err)
	}
	p.router = router

    // Select buffer type based on config (memory or disk)
    p.buffer = ingestion.NewBufferer(p.config.Ingestion, p.metrics, p.logger)

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

    if p.dedup != nil {
        p.dedup.Close()
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
    // Track per-profile reduction
    inputByProfile := make(map[string]int)
    outputByProfile := make(map[string]int)
    matchedByProfile := make(map[string]int)

    for _, entry := range batch {
        profile := "unknown"
        if entry.Labels != nil {
            if v, ok := entry.Labels["profile"]; ok {
                profile = v
            }
        }
        inputByProfile[profile]++

        entries, matched := p.processEntry(entry)
        if len(entries) > 0 {
            outputEntries = append(outputEntries, entries...)
            // Count outputs for the same profile of the trigger entry
            outputByProfile[profile] += len(entries)
        }
        if matched {
            matchedByProfile[profile]++
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

    // Update per-profile dedup ratios based on this batch
    for profile, in := range inputByProfile {
        if in <= 0 {
            continue
        }
        out := outputByProfile[profile]
        ratio := 1.0 - (float64(out) / float64(in))
        if ratio < 0 {
            ratio = 0
        }
        p.metrics.DedupRatio.WithLabelValues(profile).Set(ratio)
        // Approximate coverage as matched/in
        cov := float64(matchedByProfile[profile]) / float64(in)
        if cov < 0 { cov = 0 }
        if cov > 1 { cov = 1 }
        p.metrics.ProfileCoverage.WithLabelValues(profile).Set(cov)
    }

	return nil
}

func (p *Processor) processEntry(entry *ingestion.LogEntry) ([]*ingestion.LogEntry, bool) {
    profileName := p.profiles.DetectProfile(entry)
    
    processed, err := p.profiles.ProcessWithProfile(entry, profileName)
    if err != nil {
        p.logger.Error().Err(err).Str("profile", profileName).Msg("Profile processing error")
        return []*ingestion.LogEntry{entry}, false
    }

    if processed.Drop {
        return nil, processed.Matched
    }

    dedupResult, err := p.dedup.Process(entry)
    if err != nil {
        p.logger.Error().Err(err).Msg("Dedup processing error")
        return []*ingestion.LogEntry{entry}, processed.Matched
    }

    if dedupResult.IsDuplicate && !dedupResult.ShouldOutput {
        return nil, processed.Matched
    }

    // Build ordered output: context entries first, then the trigger/current entry
    out := make([]*ingestion.LogEntry, 0, 1+len(dedupResult.Context))
    if len(dedupResult.Context) > 0 {
        for _, ce := range dedupResult.Context {
            out = append(out, ce)
        }
    }
    out = append(out, entry)
    return out, processed.Matched
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
