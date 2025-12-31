package output

import (
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/ingestion"
	"github.com/logsieve/logsieve/pkg/metrics"
)

type Router struct {
    config   []config.OutputConfig
    logger   zerolog.Logger
    metrics  *metrics.Registry
    adapters map[string]Adapter
    cfgByName map[string]config.OutputConfig
    breakers map[string]*circuitBreaker
    mu       sync.RWMutex
}

type Adapter interface {
	Send(entries []*ingestion.LogEntry) error
	Name() string
	Close() error
}

func NewRouter(configs []config.OutputConfig, metrics *metrics.Registry, logger zerolog.Logger) (*Router, error) {
    router := &Router{
        config:   configs,
        logger:   logger.With().Str("component", "output").Logger(),
        metrics:  metrics,
        adapters: make(map[string]Adapter),
        cfgByName: make(map[string]config.OutputConfig),
        breakers: make(map[string]*circuitBreaker),
    }

	if err := router.initializeAdapters(); err != nil {
		return nil, fmt.Errorf("failed to initialize adapters: %w", err)
	}

	return router, nil
}

func (r *Router) initializeAdapters() error {
    for _, cfg := range r.config {
        adapter, err := r.createAdapter(cfg)
        if err != nil {
            r.logger.Error().Err(err).Str("output", cfg.Name).Msg("Failed to create adapter")
            continue
        }

        r.adapters[cfg.Name] = adapter
        r.cfgByName[cfg.Name] = cfg
        r.breakers[cfg.Name] = &circuitBreaker{maxFailures: cfg.MaxFailures, cooldown: cfg.Cooldown}
        r.logger.Info().Str("output", cfg.Name).Str("type", cfg.Type).Msg("Initialized output adapter")
    }

	if len(r.adapters) == 0 {
		return fmt.Errorf("no output adapters initialized")
	}

	return nil
}

func (r *Router) createAdapter(cfg config.OutputConfig) (Adapter, error) {
	switch cfg.Type {
	case "stdout":
		return NewStdoutAdapter(cfg, r.logger), nil
	case "loki":
		return NewLokiAdapter(cfg, r.logger)
	case "elasticsearch":
		return NewElasticsearchAdapter(cfg, r.logger)
	case "s3":
		return NewS3Adapter(cfg, r.metrics, r.logger)
	default:
		return nil, fmt.Errorf("unsupported output type: %s", cfg.Type)
	}
}

func (r *Router) Route(entries []*ingestion.LogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	routingMap := make(map[string][]*ingestion.LogEntry)
	
	for _, entry := range entries {
		outputNames := r.determineOutputs(entry)
		
		for _, outputName := range outputNames {
			routingMap[outputName] = append(routingMap[outputName], entry)
		}
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(routingMap))

	for outputName, outputEntries := range routingMap {
		wg.Add(1)
		go func(name string, entries []*ingestion.LogEntry) {
			defer wg.Done()
			
			if err := r.sendToOutput(name, entries); err != nil {
				errChan <- fmt.Errorf("output %s: %w", name, err)
			}
		}(outputName, outputEntries)
	}

	wg.Wait()
	close(errChan)

	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("routing errors: %v", errors)
	}

	return nil
}

func (r *Router) determineOutputs(entry *ingestion.LogEntry) []string {
	if entry.Labels != nil {
		if output, ok := entry.Labels["output"]; ok && output != "" {
			if r.hasAdapter(output) {
				return []string{output}
			}
		}
	}

	var outputs []string
	for name := range r.adapters {
		outputs = append(outputs, name)
	}

	return outputs
}

func (r *Router) hasAdapter(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	_, exists := r.adapters[name]
	return exists
}

func (r *Router) sendToOutput(outputName string, entries []*ingestion.LogEntry) error {
    r.mu.RLock()
    adapter, exists := r.adapters[outputName]
    cfg := r.cfgByName[outputName]
    br := r.breakers[outputName]
    r.mu.RUnlock()

	if !exists {
		r.metrics.OutputErrorsTotal.WithLabelValues(outputName, "not_found").Inc()
		return fmt.Errorf("adapter not found: %s", outputName)
	}

	start := time.Now()
	
    // Circuit breaker check
    if br != nil && br.isOpen() {
        r.metrics.OutputErrorsTotal.WithLabelValues(outputName, "circuit_open").Inc()
        return fmt.Errorf("circuit open for output %s", outputName)
    }

    // Retry with exponential backoff
    var err error
    backoff := cfg.InitialBackoff
    if backoff <= 0 { backoff = 250 * time.Millisecond }
    maxBackoff := cfg.MaxBackoff
    if maxBackoff <= 0 { maxBackoff = 5 * time.Second }

    attempts := cfg.Retries
    if attempts <= 0 { attempts = 1 }

    for i := 0; i < attempts; i++ {
        err = adapter.Send(entries)
        if err == nil {
            if br != nil { br.onSuccess() }
            break
        }
        if br != nil { br.onFailure() }
        if i < attempts-1 {
            time.Sleep(backoff)
            backoff = backoff * 2
            if backoff > maxBackoff { backoff = maxBackoff }
        }
    }
	
	duration := time.Since(start)
	r.metrics.OutputDuration.WithLabelValues(outputName).Observe(duration.Seconds())

	if err != nil {
		r.metrics.OutputErrorsTotal.WithLabelValues(outputName, "send_error").Inc()
		r.logger.Error().Err(err).Str("output", outputName).Int("entries", len(entries)).Msg("Failed to send to output")
		return err
	}

	r.metrics.OutputLogsTotal.WithLabelValues(outputName, "success").Add(float64(len(entries)))
	
	totalBytes := 0
	for _, entry := range entries {
		totalBytes += len(entry.Message)
	}
	r.metrics.OutputBytesTotal.WithLabelValues(outputName).Add(float64(totalBytes))

	r.logger.Debug().
		Str("output", outputName).
		Int("entries", len(entries)).
		Dur("duration", duration).
		Msg("Sent entries to output")

	return nil
}

func (r *Router) AddAdapter(name string, adapter Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.adapters[name] = adapter
	r.logger.Info().Str("output", name).Msg("Added adapter")
}

func (r *Router) RemoveAdapter(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if adapter, exists := r.adapters[name]; exists {
		adapter.Close()
		delete(r.adapters, name)
		r.logger.Info().Str("output", name).Msg("Removed adapter")
	}
}

func (r *Router) GetAdapterNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	names := make([]string, 0, len(r.adapters))
	for name := range r.adapters {
		names = append(names, name)
	}
	
	return names
}

func (r *Router) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	var errors []error
	for name, adapter := range r.adapters {
		if err := adapter.Close(); err != nil {
			errors = append(errors, fmt.Errorf("adapter %s: %w", name, err))
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("close errors: %v", errors)
	}
	
	return nil
}

type RouterStats struct {
    AdapterCount int      `json:"adapter_count"`
    Adapters     []string `json:"adapters"`
}

type circuitBreaker struct {
    failures    int
    maxFailures int
    cooldown    time.Duration
    openUntil   time.Time
}

func (cb *circuitBreaker) isOpen() bool {
    if cb == nil { return false }
    if time.Now().Before(cb.openUntil) { return true }
    // cooldown passed; allow attempt
    return false
}

func (cb *circuitBreaker) onFailure() {
    if cb == nil { return }
    cb.failures++
    if cb.failures >= cb.maxFailures && cb.maxFailures > 0 {
        cb.openUntil = time.Now().Add(cb.cooldown)
        cb.failures = 0
    }
}

func (cb *circuitBreaker) onSuccess() {
    if cb == nil { return }
    cb.failures = 0
}

func (r *Router) Stats() RouterStats {
	return RouterStats{
		AdapterCount: len(r.adapters),
		Adapters:     r.GetAdapterNames(),
	}
}
