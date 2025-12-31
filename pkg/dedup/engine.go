package dedup

import (
    "sync"
    "time"
    "strconv"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/ingestion"
	"github.com/logsieve/logsieve/pkg/metrics"
)

type Engine struct {
	config    config.DedupConfig
	logger    zerolog.Logger
	metrics   *metrics.Registry
	drain     *Drain3
	fingerprints *FingerprintCache
	context   *ContextWindow
	mu        sync.RWMutex
}

type Result struct {
	Entry         *ingestion.LogEntry
	IsDuplicate   bool
	TemplateID    string
	Fingerprint   string
	ShouldOutput  bool
	Context       []*ingestion.LogEntry
}

func NewEngine(cfg config.DedupConfig, metrics *metrics.Registry, logger zerolog.Logger) *Engine {
	return &Engine{
		config:       cfg,
		logger:       logger.With().Str("component", "dedup").Logger(),
		metrics:      metrics,
		drain:        NewDrain3(cfg, logger),
		fingerprints: NewFingerprintCache(cfg.FingerprintTTL, logger),
		context:      NewContextWindow(cfg.ContextLines, logger),
	}
}

func (e *Engine) Process(entry *ingestion.LogEntry) (*Result, error) {
	start := time.Now()
	defer func() {
		profile := "unknown"
		if entry.Labels != nil {
			if p, ok := entry.Labels["profile"]; ok {
				profile = p
			}
		}
		e.metrics.DedupProcessingDuration.WithLabelValues(profile, e.config.Engine).Observe(time.Since(start).Seconds())
	}()

	e.mu.Lock()
	defer e.mu.Unlock()

    result := &Result{
        Entry:       entry,
        IsDuplicate: false,
        ShouldOutput: true,
    }

	fingerprint := e.fingerprints.GetFingerprint(entry.Message)
	result.Fingerprint = fingerprint

	if e.fingerprints.Exists(fingerprint) {
		result.IsDuplicate = true
		result.ShouldOutput = false
		
		profile := e.getProfile(entry)
		e.metrics.DedupCacheHitsTotal.WithLabelValues(profile, "fingerprint").Inc()
		
		e.logger.Debug().
			Str("fingerprint", fingerprint[:8]).
			Str("message", entry.Message[:min(50, len(entry.Message))]).
			Msg("Duplicate detected via fingerprint")
		
		return result, nil
	}

    addRes := e.drain.AddLogMessage(entry.Message)
    templateID := strconv.Itoa(addRes.ClusterID)
    result.TemplateID = templateID

    changeType := addRes.ChangeType
    profile := e.getProfile(entry)
    if changeType == "cluster_created" {
        // new pattern
        e.metrics.DedupTemplateChangesTotal.WithLabelValues(profile, "cluster_created").Inc()
    } else if changeType == "cluster_template_changed" {
        e.metrics.DedupTemplateChangesTotal.WithLabelValues(profile, "cluster_template_changed").Inc()
    } else if changeType == "cluster_size_changed" {
        e.metrics.DedupTemplateChangesTotal.WithLabelValues(profile, "cluster_size_changed").Inc()
    }

    if changeType != "cluster_created" {
        e.metrics.DedupCacheHitsTotal.WithLabelValues(profile, "template").Inc()
        if e.shouldSkipBasedOnTemplate(templateID) {
            result.IsDuplicate = true
            result.ShouldOutput = false
        }
    }

	e.fingerprints.Add(fingerprint)

    if e.shouldPreserveContext(entry) {
        // Collect context (last N before + trigger) and include inline
        contextEntries := e.context.GetContext(entry)
        result.Context = contextEntries
        result.ShouldOutput = true
    }

    // Add the current entry to the context window after extracting context
    e.context.Add(entry)

	e.updateMetrics(entry, result)

	return result, nil
}

func (e *Engine) shouldSkipBasedOnTemplate(templateID string) bool {
	template := e.drain.GetTemplate(templateID)
	if template == nil {
		return false
	}
	
	return template.Count > 1
}

func (e *Engine) shouldPreserveContext(entry *ingestion.LogEntry) bool {
	if entry.Level == "" {
		return false
	}
	
	errorLevels := map[string]bool{
		"ERROR": true,
		"FATAL": true,
		"PANIC": true,
	}
	
	return errorLevels[entry.Level]
}

func (e *Engine) getProfile(entry *ingestion.LogEntry) string {
	if entry.Labels != nil {
		if profile, ok := entry.Labels["profile"]; ok {
			return profile
		}
	}
	return "unknown"
}

func (e *Engine) updateMetrics(entry *ingestion.LogEntry, result *Result) {
    profile := e.getProfile(entry)
    // Dedup ratio is computed accurately in processor per batch
    e.metrics.DedupPatternsTotal.WithLabelValues(profile).Set(float64(e.drain.GetPatternCount()))
}

func (e *Engine) GetStats() Stats {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	return Stats{
		PatternCount:     e.drain.GetPatternCount(),
		FingerprintCount: e.fingerprints.Size(),
		ContextSize:      e.context.Size(),
		LastProcessed:    time.Now(),
	}
}

func (e *Engine) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	e.drain.Reset()
	e.fingerprints.Clear()
	e.context.Clear()
}

// Close releases resources owned by the engine
func (e *Engine) Close() {
    e.mu.Lock()
    defer e.mu.Unlock()
    if e.fingerprints != nil {
        e.fingerprints.Stop()
    }
}

type Stats struct {
	PatternCount     int       `json:"pattern_count"`
	FingerprintCount int       `json:"fingerprint_count"`
	ContextSize      int       `json:"context_size"`
	LastProcessed    time.Time `json:"last_processed"`
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
