# LogSieve Testing Guidelines

## Testing Strategy Overview

LogSieve uses a multi-layered testing approach:
1. **Unit Tests**: Individual component testing
2. **Integration Tests**: Component interaction testing  
3. **Performance Tests**: Load and benchmark testing
4. **Profile Tests**: Profile effectiveness validation

## Unit Testing Standards

### Test File Organization
```
pkg/
├── component/
│   ├── component.go
│   ├── component_test.go      # Unit tests
│   └── testdata/              # Test fixtures
│       ├── input.json
│       └── expected.json
```

### Test Naming Convention
```go
func TestComponentName_MethodName_Scenario(t *testing.T) {
    // Test implementation
}

// Examples:
func TestProcessor_ProcessBatch_EmptyBatch(t *testing.T)
func TestProfileManager_DetectProfile_NginxContainer(t *testing.T)
func TestDedupEngine_Process_DuplicateEntry(t *testing.T)
```

### Table-Driven Test Pattern
```go
func TestLogParser_Parse_VariousFormats(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected *LogEntry
        wantErr  bool
    }{
        {
            name:  "fluent_bit_format",
            input: `{"log":"test message","time":"2023-01-01T00:00:00Z"}`,
            expected: &LogEntry{
                Message:   "test message",
                Timestamp: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
            },
            wantErr: false,
        },
        {
            name:    "invalid_json",
            input:   `{"log":"test"`,
            expected: nil,
            wantErr: true,
        },
    }

    parser := NewParser(logger)
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := parser.Parse(tt.input)
            
            if tt.wantErr {
                assert.Error(t, err)
                return
            }
            
            assert.NoError(t, err)
            assert.Equal(t, tt.expected.Message, result.Message)
            assert.Equal(t, tt.expected.Timestamp, result.Timestamp)
        })
    }
}
```

### Mock Interfaces
```go
// Use interfaces for testability
type OutputWriter interface {
    Write(entries []*LogEntry) error
}

// Mock implementation
type MockOutputWriter struct {
    WrittenEntries []*LogEntry
    WriteError     error
}

func (m *MockOutputWriter) Write(entries []*LogEntry) error {
    if m.WriteError != nil {
        return m.WriteError
    }
    m.WrittenEntries = append(m.WrittenEntries, entries...)
    return nil
}

// Test usage
func TestProcessor_ProcessBatch_WritesToOutput(t *testing.T) {
    mockWriter := &MockOutputWriter{}
    processor := NewProcessor(config, mockWriter, logger)
    
    entries := []*LogEntry{{Message: "test"}}
    err := processor.ProcessBatch(entries)
    
    assert.NoError(t, err)
    assert.Len(t, mockWriter.WrittenEntries, 1)
    assert.Equal(t, "test", mockWriter.WrittenEntries[0].Message)
}
```

### Test Helpers
```go
// test/helpers/helpers.go
package helpers

func CreateTestLogEntry(message string) *ingestion.LogEntry {
    return &ingestion.LogEntry{
        Timestamp: time.Now(),
        Message:   message,
        Level:     "INFO",
        Source:    "test",
        Labels:    make(map[string]string),
    }
}

func CreateTestProfile(name string, rules []profiles.FingerprintRule) *profiles.Profile {
    return &profiles.Profile{
        Metadata: profiles.ProfileMetadata{
            Name:    name,
            Version: "1.0.0",
        },
        Spec: profiles.ProfileSpec{
            Fingerprints: rules,
        },
    }
}

func AssertLogEntryEqual(t *testing.T, expected, actual *ingestion.LogEntry) {
    assert.Equal(t, expected.Message, actual.Message)
    assert.Equal(t, expected.Level, actual.Level)
    assert.Equal(t, expected.Source, actual.Source)
}
```

## Integration Testing

### Test Structure
```
test/
├── integration/
│   ├── ingestion_test.go      # End-to-end ingestion tests
│   ├── dedup_test.go          # Deduplication integration tests
│   ├── profile_test.go        # Profile processing tests
│   └── output_test.go         # Output routing tests
├── fixtures/
│   ├── nginx_logs.json        # Sample log data
│   ├── postgres_logs.json
│   └── profiles/
│       ├── nginx.yaml
│       └── postgres.yaml
└── docker-compose.test.yml    # Test infrastructure
```

### HTTP Integration Tests
```go
func TestIngestionHandler_Integration(t *testing.T) {
    // Setup test server
    cfg := config.DefaultConfig()
    metrics := metrics.NewRegistry()
    logger := zerolog.New(os.Stdout)
    
    processor := processor.NewProcessor(cfg, metrics, logger)
    handler := ingestion.NewHandler(cfg, metrics, logger)
    handler.SetProcessor(processor)
    
    router := gin.New()
    router.POST("/ingest", handler.HandleIngest)
    
    server := httptest.NewServer(router)
    defer server.Close()
    
    // Test request
    payload := `{
        "logs": [
            {"log": "test message", "time": "2023-01-01T00:00:00Z"}
        ]
    }`
    
    resp, err := http.Post(server.URL+"/ingest", "application/json", strings.NewReader(payload))
    assert.NoError(t, err)
    assert.Equal(t, http.StatusOK, resp.StatusCode)
    
    var response ingestion.IngestResponse
    err = json.NewDecoder(resp.Body).Decode(&response)
    assert.NoError(t, err)
    assert.Equal(t, "success", response.Status)
    assert.Equal(t, 1, response.Processed)
}
```

### Database Integration Tests
```go
func TestOutputAdapter_Loki_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }
    
    // Start test Loki instance
    lokiContainer := testcontainers.GenericContainer{
        ContainerRequest: testcontainers.ContainerRequest{
            Image:        "grafana/loki:latest",
            ExposedPorts: []string{"3100/tcp"},
            WaitingFor:   wait.ForHTTP("/ready").WithPort("3100"),
        },
    }
    
    ctx := context.Background()
    container, err := testcontainers.GenericContainer(ctx, lokiContainer)
    require.NoError(t, err)
    defer container.Terminate(ctx)
    
    // Get container endpoint
    endpoint, err := container.Endpoint(ctx, "")
    require.NoError(t, err)
    
    // Test output adapter
    cfg := output.LokiConfig{
        URL: "http://" + endpoint,
    }
    
    adapter := output.NewLokiAdapter(cfg, logger)
    
    entries := []*ingestion.LogEntry{
        {Message: "test log", Timestamp: time.Now()},
    }
    
    err = adapter.Send(entries)
    assert.NoError(t, err)
}
```

## Performance Testing

### Benchmark Tests
```go
func BenchmarkDedupEngine_Process(b *testing.B) {
    engine := dedup.NewEngine(config.DedupConfig{
        Engine:    "drain3",
        CacheSize: 10000,
    }, metrics.NewRegistry(), logger)
    
    entry := &ingestion.LogEntry{
        Message: "Test log message with variable data: " + strconv.Itoa(rand.Int()),
    }
    
    b.ResetTimer()
    b.ReportAllocs()
    
    for i := 0; i < b.N; i++ {
        _, err := engine.Process(entry)
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkProfileProcessor_ProcessEntry(b *testing.B) {
    profile := createTestProfile()
    processor := profiles.NewProcessor(logger)
    
    entry := &ingestion.LogEntry{
        Message: "GET /api/users/123 HTTP/1.1 200 1234",
    }
    
    b.ResetTimer()
    
    for i := 0; i < b.N; i++ {
        _, err := processor.ProcessEntry(entry, profile)
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

### Load Testing
```go
func TestProcessor_LoadTest(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping load test in short mode")
    }
    
    cfg := config.DefaultConfig()
    cfg.Ingestion.MaxBatchSize = 1000
    
    processor := processor.NewProcessor(cfg, metrics.NewRegistry(), logger)
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    err := processor.Start(ctx)
    require.NoError(t, err)
    defer processor.Stop()
    
    // Generate load
    const numWorkers = 10
    const entriesPerWorker = 1000
    
    var wg sync.WaitGroup
    for i := 0; i < numWorkers; i++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()
            
            for j := 0; j < entriesPerWorker; j++ {
                entry := &ingestion.LogEntry{
                    Message: fmt.Sprintf("Worker %d message %d", workerID, j),
                    Timestamp: time.Now(),
                }
                
                err := processor.AddEntry(entry)
                if err != nil {
                    t.Errorf("Worker %d failed to add entry: %v", workerID, err)
                    return
                }
            }
        }(i)
    }
    
    wg.Wait()
    
    // Verify processing completed
    stats := processor.GetStats()
    assert.True(t, stats.BufferStats.Processed >= numWorkers*entriesPerWorker)
}
```

## Profile Testing

### Profile Validation Tests
```go
func TestProfile_Nginx_Validation(t *testing.T) {
    profileData, err := os.ReadFile("../../profiles/nginx.yaml")
    require.NoError(t, err)
    
    var profile profiles.Profile
    err = yaml.Unmarshal(profileData, &profile)
    require.NoError(t, err)
    
    // Validate metadata
    assert.Equal(t, "nginx", profile.Metadata.Name)
    assert.NotEmpty(t, profile.Metadata.Version)
    assert.NotEmpty(t, profile.Metadata.Description)
    
    // Validate patterns compile
    for i, rule := range profile.Spec.Fingerprints {
        _, err := regexp.Compile(rule.Pattern)
        assert.NoError(t, err, "Pattern %d should compile: %s", i, rule.Pattern)
    }
}
```

### Profile Coverage Tests
```go
func TestProfile_Nginx_Coverage(t *testing.T) {
    // Load profile
    profile := loadTestProfile("nginx")
    
    // Load test logs
    testLogs := loadTestLogs("nginx_sample.log")
    
    processor := profiles.NewProcessor(logger)
    
    matched := 0
    total := len(testLogs)
    
    for _, logEntry := range testLogs {
        result, err := processor.ProcessEntry(logEntry, profile)
        require.NoError(t, err)
        
        if len(result.Actions) > 0 {
            matched++
        }
    }
    
    coverage := float64(matched) / float64(total)
    assert.Greater(t, coverage, 0.95, "Profile should cover >95% of logs")
    
    t.Logf("Profile coverage: %.2f%% (%d/%d)", coverage*100, matched, total)
}
```

### Profile Performance Tests
```go
func TestProfile_Performance_Impact(t *testing.T) {
    profile := loadTestProfile("nginx")
    processor := profiles.NewProcessor(logger)
    
    entry := &ingestion.LogEntry{
        Message: `192.168.1.1 - - [01/Jan/2023:00:00:00 +0000] "GET /api/users HTTP/1.1" 200 1234`,
    }
    
    // Baseline without profile
    start := time.Now()
    for i := 0; i < 10000; i++ {
        // Simulate basic processing
        _ = entry.Message
    }
    baseline := time.Since(start)
    
    // With profile processing
    start = time.Now()
    for i := 0; i < 10000; i++ {
        _, err := processor.ProcessEntry(entry, profile)
        require.NoError(t, err)
    }
    withProfile := time.Since(start)
    
    overhead := float64(withProfile-baseline) / float64(baseline)
    assert.Less(t, overhead, 0.1, "Profile processing should add <10% overhead")
    
    t.Logf("Baseline: %v, With profile: %v, Overhead: %.2f%%", 
           baseline, withProfile, overhead*100)
}
```

## Official API Compliance Testing

### Drain3 Compliance Tests
```go
func TestDrain3_OfficialAPICompliance(t *testing.T) {
    drain := dedup.NewDrain3(dedup.DefaultDrain3Config(), logger)
    
    // Test official AddLogMessage API
    result := drain.AddLogMessage("User 123 logged in from 192.168.1.1")
    assert.NotNil(t, result)
    assert.NotEmpty(t, result.ClusterID)
    assert.Contains(t, result.Template, "<*>") // Should contain wildcards
    
    // Test official Match API
    match := drain.Match("User 456 logged in from 10.0.0.1", false)
    assert.NotNil(t, match)
    assert.Equal(t, result.ClusterID, match.ClusterID)
    assert.Greater(t, match.Similarity, 0.8)
    
    // Test parameter extraction
    params := drain.ExtractParameters("User 789 logged in from 172.16.0.1", match.Template)
    assert.Len(t, params, 2) // Should extract user ID and IP
}
```

### Output Adapter Compliance Tests
```go
func TestLokiAdapter_StructuredMetadataSupport(t *testing.T) {
    adapter := output.NewLokiAdapter(config, logger)
    
    entries := []*ingestion.LogEntry{
        {
            Message: "test log",
            Labels: map[string]string{
                "service": "api",
                "level":   "info",
            },
            StructuredMetadata: map[string]interface{}{
                "user_id":    "12345",
                "request_id": "req-abc-123",
                "duration":   "150ms",
            },
        },
    }
    
    // Should support Loki v3+ structured metadata
    err := adapter.Send(entries)
    assert.NoError(t, err)
    
    // Verify structured metadata is properly formatted
    // (This would require integration test with real Loki)
}
```

## Test Data Management

### Fixture Organization
```
test/fixtures/
├── logs/
│   ├── nginx/
│   │   ├── access.log
│   │   ├── error.log
│   │   └── combined.json
│   ├── postgres/
│   │   ├── startup.log
│   │   ├── queries.log
│   │   └── errors.log
│   └── java/
│       ├── application.log
│       └── gc.log
├── profiles/
│   ├── nginx.yaml
│   ├── postgres.yaml
│   └── java-spring.yaml
└── configs/
    ├── minimal.yaml
    ├── full.yaml
    └── performance.yaml
```

### Test Data Generation
```go
// Generate realistic test data
func generateNginxLogs(count int) []*ingestion.LogEntry {
    entries := make([]*ingestion.LogEntry, count)
    
    ips := []string{"192.168.1.1", "10.0.0.1", "172.16.0.1"}
    methods := []string{"GET", "POST", "PUT", "DELETE"}
    paths := []string{"/api/users", "/api/orders", "/health", "/metrics"}
    statuses := []int{200, 201, 400, 404, 500}
    
    for i := 0; i < count; i++ {
        ip := ips[rand.Intn(len(ips))]
        method := methods[rand.Intn(len(methods))]
        path := paths[rand.Intn(len(paths))]
        status := statuses[rand.Intn(len(statuses))]
        size := rand.Intn(10000)
        
        message := fmt.Sprintf(`%s - - [%s] "%s %s HTTP/1.1" %d %d`,
            ip, time.Now().Format("02/Jan/2006:15:04:05 -0700"),
            method, path, status, size)
        
        entries[i] = &ingestion.LogEntry{
            Message:   message,
            Timestamp: time.Now(),
            Level:     "INFO",
            Source:    "nginx",
        }
    }
    
    return entries
}
```

## Continuous Integration

### Test Commands
```makefile
# Makefile test targets
.PHONY: test test-unit test-integration test-performance test-profiles

test: test-unit test-integration

test-unit:
	go test -v -race -coverprofile=coverage.out ./pkg/...

test-integration:
	go test -v -tags=integration ./test/integration/...

test-performance:
	go test -v -bench=. -benchmem ./pkg/...

test-profiles:
	go test -v ./test/profiles/...

test-coverage:
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"
```

### GitHub Actions Workflow
```yaml
name: Test
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - uses: actions/setup-go@v3
        with:
          go-version: '1.21'
      
      - name: Run unit tests
        run: make test-unit
      
      - name: Run integration tests
        run: make test-integration
      
      - name: Upload coverage
        uses: codecov/codecov-action@v3
        with:
          file: ./coverage.out
```

## Test Quality Standards

### Coverage Requirements
- **Unit tests**: Minimum 80% code coverage
- **Integration tests**: Cover all major user flows
- **Profile tests**: >95% pattern coverage for production profiles

### Performance Benchmarks
- **Throughput**: >10,000 logs/second per instance
- **Latency**: <10ms p99 processing time
- **Memory**: <500MB under load
- **Profile overhead**: <10% processing time increase