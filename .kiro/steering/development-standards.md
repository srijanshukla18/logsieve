# LogSieve Development Standards

## Code Organization

### Package Structure
```
pkg/
├── config/         # Configuration management (Viper-based)
├── ingestion/      # HTTP handlers and log parsing
├── processor/      # Main processing pipeline orchestration
├── dedup/          # Deduplication engines (Drain3, fingerprinting)
├── profiles/       # Profile management and auto-detection
├── output/         # Output adapters and routing
├── metrics/        # Prometheus metrics registry
├── hub/            # Profile hub client (future)
└── cache/          # Caching utilities
```

### Interface Design Patterns

1. **Processor Interface**: Components that process log entries
```go
type Processor interface {
    AddEntry(entry *LogEntry) error
}
```

2. **Stats Interface**: Components that provide runtime statistics
```go
type Stats interface {
    GetStats() ComponentStats
}
```

3. **Lifecycle Interface**: Components with start/stop lifecycle
```go
type Lifecycle interface {
    Start(ctx context.Context) error
    Stop() error
}
```

## Configuration Standards

### Configuration Structure
- Use `config.Config` struct with mapstructure tags
- Provide sensible defaults in `DefaultConfig()`
- Support environment variables with `LOGSIEVE_` prefix
- Validate configuration in `validateConfig()`

### Example Configuration Pattern
```go
type ComponentConfig struct {
    Enabled  bool          `mapstructure:"enabled"`
    Timeout  time.Duration `mapstructure:"timeout"`
    MaxSize  int           `mapstructure:"maxSize"`
}
```

## Error Handling

### Error Patterns
1. **Wrap errors** with context: `fmt.Errorf("operation failed: %w", err)`
2. **Log errors** at appropriate levels with structured fields
3. **Graceful degradation** - continue processing when possible
4. **Circuit breakers** for external dependencies

### Logging Standards
```go
logger.Error().
    Err(err).
    Str("component", "processor").
    Int("batch_size", len(batch)).
    Msg("Failed to process batch")
```

## Concurrency Patterns

### Safe Patterns
1. **Mutex protection** for shared state: `sync.RWMutex`
2. **Channel communication** for async processing
3. **Context cancellation** for graceful shutdown
4. **WaitGroups** for coordinated shutdown

### Example Pattern
```go
type Component struct {
    mu      sync.RWMutex
    running bool
    stopCh  chan struct{}
    wg      sync.WaitGroup
}

func (c *Component) Start(ctx context.Context) error {
    c.mu.Lock()
    if c.running {
        c.mu.Unlock()
        return fmt.Errorf("already running")
    }
    c.running = true
    c.mu.Unlock()
    
    c.wg.Add(1)
    go c.processingLoop(ctx)
    return nil
}
```

## Testing Standards

### Test Organization
- Unit tests: `*_test.go` files alongside source
- Integration tests: `test/integration/` directory
- Test fixtures: `test/fixtures/` directory

### Test Patterns
1. **Table-driven tests** for multiple scenarios
2. **Mock interfaces** for external dependencies
3. **Test helpers** for common setup/teardown
4. **Benchmarks** for performance-critical code

## Metrics and Observability

### Metric Naming Convention (Prometheus Best Practices)
- Prefix: `logsieve_`
- Component: `logsieve_ingestion_`, `logsieve_dedup_`, etc.
- **Required suffixes**: `_total` for counters, `_seconds` for durations, `_bytes` for sizes
- **Standard metrics**: `build_info`, `start_time_seconds`, `uptime_seconds`

### Required Metrics
1. **Throughput**: `*_total` counters (e.g., `logsieve_ingestion_logs_total`)
2. **Latency**: `*_duration_seconds` histograms  
3. **Errors**: `*_errors_total` counters
4. **Resource usage**: `*_bytes`, `*_ratio` gauges
5. **Cardinality management**: Avoid high-cardinality labels like `path` or `user_id`

### Structured Logging
```go
logger.Info().
    Str("component", "processor").
    Int("processed", count).
    Dur("duration", elapsed).
    Float64("reduction_ratio", ratio).
    Msg("Batch processed")
```

## Profile Development

### Profile Structure
```yaml
apiVersion: hub.logsieve.io/v1
kind: LogProfile
metadata:
  name: component-name
  version: "1.0.0"
  author: "@username"
  description: "Brief description"
  tags: ["tag1", "tag2"]
  images: ["image:*"]

spec:
  fingerprints:     # Pattern matching rules
  contextTriggers:  # Context preservation rules
  sampling:         # Sampling rules
  transforms:       # Data transformation rules
  routing:          # Output routing rules
```

### Profile Best Practices
1. **Start specific, then general**: Most specific patterns first
2. **Preserve context** around errors and critical events
3. **Sample heavily** for high-volume, low-value logs
4. **Scrub sensitive data** in transforms
5. **Test coverage** should be >95% for production profiles

## Official API Compliance

### Specialized Library Integration
When integrating with specialized libraries, always verify against official documentation:

1. **Drain3**: Use official API methods (`AddLogMessage`, `Match`, `ExtractParameters`)
2. **Loki**: Support v3+ features like structured metadata
3. **Elasticsearch**: Follow ECS (Elastic Common Schema) for document structure
4. **Prometheus**: Follow naming conventions and include standard metrics

### API Verification Process
1. **Cross-check implementation** against official documentation
2. **Use official data structures** (e.g., `LogCluster` for Drain3)
3. **Implement all required methods** for full compatibility
4. **Test against official examples** and test cases
5. **Follow configuration patterns** from official specs

## Performance Guidelines

### Memory Management
1. **Bounded caches** with TTL and size limits
2. **Object pooling** for frequently allocated objects
3. **Streaming processing** - avoid loading entire datasets
4. **Memory profiling** for optimization

### CPU Optimization
1. **Avoid regex** in hot paths when possible
2. **Batch operations** to amortize overhead
3. **Parallel processing** where safe
4. **CPU profiling** for bottleneck identification

### I/O Patterns
1. **Buffered writes** to outputs
2. **Connection pooling** for HTTP clients
3. **Timeout handling** for all external calls
4. **Retry with backoff** for transient failures