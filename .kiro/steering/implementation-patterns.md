# LogSieve Implementation Patterns

## Component Implementation Pattern

### Standard Component Structure
```go
type Component struct {
    config   ComponentConfig
    logger   zerolog.Logger
    metrics  *metrics.Registry
    // component-specific fields
    mu       sync.RWMutex
    running  bool
    stopCh   chan struct{}
    wg       sync.WaitGroup
}

func NewComponent(cfg ComponentConfig, metrics *metrics.Registry, logger zerolog.Logger) *Component {
    return &Component{
        config:  cfg,
        logger:  logger.With().Str("component", "name").Logger(),
        metrics: metrics,
        stopCh:  make(chan struct{}),
    }
}
```

### Lifecycle Management
```go
func (c *Component) Start(ctx context.Context) error {
    c.mu.Lock()
    if c.running {
        c.mu.Unlock()
        return fmt.Errorf("component already running")
    }
    c.running = true
    c.mu.Unlock()

    c.logger.Info().Msg("Starting component")
    
    c.wg.Add(1)
    go c.processingLoop(ctx)
    
    return nil
}

func (c *Component) Stop() error {
    c.mu.Lock()
    if !c.running {
        c.mu.Unlock()
        return nil
    }
    c.running = false
    c.mu.Unlock()

    c.logger.Info().Msg("Stopping component")
    
    close(c.stopCh)
    c.wg.Wait()
    
    return nil
}
```

## HTTP Handler Pattern

### Standard Handler Structure
```go
type Handler struct {
    config    *config.Config
    metrics   *metrics.Registry
    logger    zerolog.Logger
    processor Processor
}

func (h *Handler) HandleEndpoint(c *gin.Context) {
    start := time.Now()
    
    // Extract parameters
    param := c.Query("param")
    source := c.GetHeader("X-Source")
    
    // Validate request
    var req RequestType
    if err := c.ShouldBindJSON(&req); err != nil {
        h.metrics.ErrorsTotal.WithLabelValues(source, "parse_error").Inc()
        c.JSON(http.StatusBadRequest, ErrorResponse{
            Status:  "error",
            Message: fmt.Sprintf("Invalid JSON: %v", err),
        })
        return
    }
    
    // Process request
    result, err := h.processRequest(&req)
    if err != nil {
        h.logger.Error().Err(err).Msg("Processing failed")
        h.metrics.ErrorsTotal.WithLabelValues(source, "process_error").Inc()
        c.JSON(http.StatusInternalServerError, ErrorResponse{
            Status:  "error", 
            Message: "Processing failed",
        })
        return
    }
    
    // Update metrics
    duration := time.Since(start)
    h.metrics.RequestsTotal.WithLabelValues(source).Inc()
    h.metrics.RequestDuration.WithLabelValues(source).Observe(duration.Seconds())
    
    // Return response
    c.JSON(http.StatusOK, result)
}
```

## Processing Pipeline Pattern

### Batch Processing
```go
func (p *Processor) processingLoop(ctx context.Context) {
    defer p.wg.Done()
    
    ticker := time.NewTicker(p.config.FlushInterval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-p.stopCh:
            return
        case batch := <-p.batchCh:
            if err := p.processBatch(batch); err != nil {
                p.logger.Error().Err(err).Msg("Batch processing failed")
            }
        case <-ticker.C:
            if err := p.flushPendingBatches(); err != nil {
                p.logger.Error().Err(err).Msg("Flush failed")
            }
        }
    }
}

func (p *Processor) processBatch(batch []*LogEntry) error {
    start := time.Now()
    defer func() {
        p.metrics.BatchProcessingDuration.Observe(time.Since(start).Seconds())
        p.metrics.BatchSize.Observe(float64(len(batch)))
    }()
    
    var outputEntries []*LogEntry
    
    for _, entry := range batch {
        processed, shouldOutput := p.processEntry(entry)
        if shouldOutput {
            outputEntries = append(outputEntries, processed)
        }
    }
    
    if len(outputEntries) > 0 {
        return p.router.Route(outputEntries)
    }
    
    return nil
}
```

## Configuration Loading Pattern

### Hierarchical Configuration
```go
func Load(configPath string) (*Config, error) {
    v := viper.New()
    
    // Set config file
    if configPath != "" {
        v.SetConfigFile(configPath)
    } else {
        v.SetConfigName("config")
        v.SetConfigType("yaml")
        v.AddConfigPath(".")
        v.AddConfigPath("/etc/logsieve")
        v.AddConfigPath("$HOME/.logsieve")
    }
    
    // Environment variables
    v.SetEnvPrefix("LOGSIEVE")
    v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
    v.AutomaticEnv()
    
    // Set defaults
    setDefaults(v)
    
    // Read config
    if err := v.ReadInConfig(); err != nil {
        if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
            return nil, fmt.Errorf("error reading config: %w", err)
        }
    }
    
    // Unmarshal
    config := DefaultConfig()
    if err := v.Unmarshal(config); err != nil {
        return nil, fmt.Errorf("error unmarshaling config: %w", err)
    }
    
    // Validate
    if err := validateConfig(config); err != nil {
        return nil, fmt.Errorf("config validation failed: %w", err)
    }
    
    return config, nil
}
```

## Profile Processing Pattern

### Rule-Based Processing
```go
func (p *ProfileProcessor) processEntry(entry *LogEntry, profile *Profile) (*ProcessedEntry, error) {
    result := &ProcessedEntry{
        Entry:   entry,
        Profile: profile.Metadata.Name,
        Actions: []string{},
    }
    
    // Apply fingerprint rules
    for _, rule := range profile.Spec.Fingerprints {
        if matched, err := rule.Matches(entry.Message); err != nil {
            return nil, fmt.Errorf("pattern match error: %w", err)
        } else if matched {
            result.Actions = append(result.Actions, rule.Action)
            
            switch rule.Action {
            case "drop":
                result.Drop = true
                return result, nil
            case "template":
                result.Template = true
            case "keep":
                result.Keep = true
            }
            
            break // First match wins
        }
    }
    
    // Apply sampling rules
    for _, rule := range profile.Spec.Sampling {
        if matched, err := rule.Matches(entry.Message); err != nil {
            continue
        } else if matched {
            if rand.Float64() > rule.Rate {
                result.Drop = true
                return result, nil
            }
            break
        }
    }
    
    // Apply transforms
    for _, transform := range profile.Spec.Transforms {
        if err := transform.Apply(entry); err != nil {
            p.logger.Error().Err(err).Msg("Transform failed")
        } else {
            result.Modified = true
        }
    }
    
    return result, nil
}
```

## Output Adapter Pattern

### Standard Output Adapter
```go
type OutputAdapter struct {
    name     string
    config   OutputConfig
    client   HTTPClient
    logger   zerolog.Logger
    metrics  *metrics.Registry
    buffer   []LogEntry
    mu       sync.Mutex
}

func (o *OutputAdapter) Send(entries []*LogEntry) error {
    start := time.Now()
    defer func() {
        o.metrics.OutputDuration.WithLabelValues(o.name).Observe(time.Since(start).Seconds())
    }()
    
    // Convert to output format
    payload, err := o.formatEntries(entries)
    if err != nil {
        o.metrics.OutputErrorsTotal.WithLabelValues(o.name, "format_error").Inc()
        return fmt.Errorf("format error: %w", err)
    }
    
    // Send with retries
    for attempt := 0; attempt < o.config.Retries; attempt++ {
        if err := o.sendPayload(payload); err != nil {
            if attempt == o.config.Retries-1 {
                o.metrics.OutputErrorsTotal.WithLabelValues(o.name, "send_error").Inc()
                return fmt.Errorf("send failed after %d attempts: %w", o.config.Retries, err)
            }
            
            backoff := time.Duration(attempt+1) * time.Second
            time.Sleep(backoff)
            continue
        }
        
        // Success
        o.metrics.OutputLogsTotal.WithLabelValues(o.name, "success").Add(float64(len(entries)))
        return nil
    }
    
    return nil
}
```

## Cache Implementation Pattern

### TTL Cache with Size Limits
```go
type TTLCache struct {
    items    map[string]*cacheItem
    maxSize  int
    ttl      time.Duration
    mu       sync.RWMutex
    stopCh   chan struct{}
    wg       sync.WaitGroup
}

type cacheItem struct {
    value     interface{}
    expiresAt time.Time
}

func NewTTLCache(maxSize int, ttl time.Duration) *TTLCache {
    c := &TTLCache{
        items:   make(map[string]*cacheItem),
        maxSize: maxSize,
        ttl:     ttl,
        stopCh:  make(chan struct{}),
    }
    
    c.wg.Add(1)
    go c.cleanupLoop()
    
    return c
}

func (c *TTLCache) Get(key string) (interface{}, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    item, exists := c.items[key]
    if !exists || time.Now().After(item.expiresAt) {
        return nil, false
    }
    
    return item.value, true
}

func (c *TTLCache) Set(key string, value interface{}) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // Evict if at capacity
    if len(c.items) >= c.maxSize {
        c.evictOldest()
    }
    
    c.items[key] = &cacheItem{
        value:     value,
        expiresAt: time.Now().Add(c.ttl),
    }
}
```

## Deduplication Engine Pattern

### Official Drain3 Implementation
```go
type Drain3Engine struct {
    config    Drain3Config
    clusters  map[string]*LogCluster
    tree      *PrefixTree
    mu        sync.RWMutex
}

// Official Drain3 API - Training mode
func (d *Drain3Engine) AddLogMessage(logMessage string) *AddLogResult {
    // Mask tokens (IPs, numbers, UUIDs)
    maskedMessage := d.maskTokens(logMessage)
    
    // Traverse prefix tree by depth
    cluster := d.traverseTree(maskedMessage)
    
    if cluster == nil {
        // Create new cluster with template
        cluster = d.createCluster(maskedMessage)
    } else {
        // Update existing cluster template
        cluster = d.updateTemplate(cluster, maskedMessage)
    }
    
    return &AddLogResult{
        ClusterID: cluster.ClusterID,
        Template:  cluster.Template,
        IsNew:     cluster.Size == 1,
    }
}

// Official Drain3 API - Inference mode
func (d *Drain3Engine) Match(logMessage string, fullSearchStrategy bool) *TemplateMatch {
    maskedMessage := d.maskTokens(logMessage)
    cluster := d.findBestMatch(maskedMessage, fullSearchStrategy)
    
    if cluster != nil {
        return &TemplateMatch{
            ClusterID:  cluster.ClusterID,
            Template:   cluster.Template,
            Similarity: d.calculateSimilarity(maskedMessage, cluster.Template),
        }
    }
    return nil
}

// Parameter extraction from templates
func (d *Drain3Engine) ExtractParameters(logMessage string, template string) []ExtractedParameter {
    return d.extractVariableParts(logMessage, template)
}
```

## Testing Patterns

### Table-Driven Tests
```go
func TestProcessEntry(t *testing.T) {
    tests := []struct {
        name     string
        entry    *LogEntry
        profile  *Profile
        expected *ProcessedEntry
        wantErr  bool
    }{
        {
            name: "drop rule matches",
            entry: &LogEntry{Message: "GET /health"},
            profile: &Profile{
                Spec: ProfileSpec{
                    Fingerprints: []FingerprintRule{
                        {Pattern: "GET /health", Action: "drop"},
                    },
                },
            },
            expected: &ProcessedEntry{Drop: true},
            wantErr:  false,
        },
        // More test cases...
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            processor := NewProfileProcessor(logger)
            result, err := processor.processEntry(tt.entry, tt.profile)
            
            if tt.wantErr {
                assert.Error(t, err)
                return
            }
            
            assert.NoError(t, err)
            assert.Equal(t, tt.expected.Drop, result.Drop)
        })
    }
}
```