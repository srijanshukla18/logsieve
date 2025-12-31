package output

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/ingestion"
	"github.com/logsieve/logsieve/pkg/metrics"
)

type mockAdapter struct {
	name    string
	entries []*ingestion.LogEntry
	err     error
	mu      sync.Mutex
}

func (m *mockAdapter) Send(entries []*ingestion.LogEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.entries = append(m.entries, entries...)
	return nil
}

func (m *mockAdapter) Name() string {
	return m.name
}

func (m *mockAdapter) Close() error {
	return nil
}

func (m *mockAdapter) GetEntries() []*ingestion.LogEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.entries
}

func newTestRouter() (*Router, error) {
	configs := []config.OutputConfig{
		{
			Name:           "stdout",
			Type:           "stdout",
			BatchSize:      100,
			Timeout:        10 * time.Second,
			Retries:        3,
			InitialBackoff: 100 * time.Millisecond,
			MaxBackoff:     1 * time.Second,
			MaxFailures:    5,
			Cooldown:       5 * time.Second,
		},
	}
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()
	return NewRouter(configs, metricsRegistry, logger)
}

func TestNewRouter(t *testing.T) {
	router, err := newTestRouter()
	if err != nil {
		t.Fatalf("NewRouter failed: %v", err)
	}
	defer router.Close()

	if router == nil {
		t.Fatal("expected non-nil router")
	}

	if len(router.adapters) == 0 {
		t.Error("expected at least one adapter")
	}
}

func TestNewRouter_NoAdapters(t *testing.T) {
	configs := []config.OutputConfig{}
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	_, err := NewRouter(configs, metricsRegistry, logger)
	if err == nil {
		t.Error("expected error with no adapters")
	}
}

func TestNewRouter_InvalidType(t *testing.T) {
	configs := []config.OutputConfig{
		{
			Name: "invalid",
			Type: "unknown-type",
		},
	}
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	_, err := NewRouter(configs, metricsRegistry, logger)
	if err == nil {
		t.Error("expected error with invalid adapter type")
	}
}

func TestRouter_Route_Empty(t *testing.T) {
	router, _ := newTestRouter()
	defer router.Close()

	err := router.Route([]*ingestion.LogEntry{})
	if err != nil {
		t.Errorf("empty route should not error: %v", err)
	}
}

func TestRouter_Route_SingleEntry(t *testing.T) {
	router, _ := newTestRouter()
	defer router.Close()

	entries := []*ingestion.LogEntry{
		{Message: "test message", Timestamp: time.Now()},
	}

	err := router.Route(entries)
	if err != nil {
		t.Errorf("route failed: %v", err)
	}
}

func TestRouter_Route_WithOutputLabel(t *testing.T) {
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	router := &Router{
		config:    nil,
		logger:    logger,
		metrics:   metricsRegistry,
		adapters:  make(map[string]Adapter),
		cfgByName: make(map[string]config.OutputConfig),
		breakers:  make(map[string]*circuitBreaker),
	}

	mockOut := &mockAdapter{name: "target"}
	router.adapters["target"] = mockOut
	router.cfgByName["target"] = config.OutputConfig{Name: "target", Retries: 1}

	entries := []*ingestion.LogEntry{
		{
			Message: "test",
			Labels:  map[string]string{"output": "target"},
		},
	}

	err := router.Route(entries)
	if err != nil {
		t.Errorf("route failed: %v", err)
	}

	if len(mockOut.GetEntries()) != 1 {
		t.Errorf("expected 1 entry sent to target, got %d", len(mockOut.GetEntries()))
	}
}

func TestRouter_AddAdapter(t *testing.T) {
	router, _ := newTestRouter()
	defer router.Close()

	mock := &mockAdapter{name: "new-adapter"}
	router.AddAdapter("new-adapter", mock)

	names := router.GetAdapterNames()
	found := false
	for _, name := range names {
		if name == "new-adapter" {
			found = true
			break
		}
	}
	if !found {
		t.Error("added adapter not found")
	}
}

func TestRouter_RemoveAdapter(t *testing.T) {
	router, _ := newTestRouter()
	defer router.Close()

	mock := &mockAdapter{name: "to-remove"}
	router.AddAdapter("to-remove", mock)
	router.RemoveAdapter("to-remove")

	names := router.GetAdapterNames()
	for _, name := range names {
		if name == "to-remove" {
			t.Error("removed adapter should not be found")
		}
	}
}

func TestRouter_GetAdapterNames(t *testing.T) {
	router, _ := newTestRouter()
	defer router.Close()

	names := router.GetAdapterNames()
	if len(names) == 0 {
		t.Error("expected at least one adapter name")
	}
}

func TestRouter_HasAdapter(t *testing.T) {
	router, _ := newTestRouter()
	defer router.Close()

	if !router.hasAdapter("stdout") {
		t.Error("expected stdout adapter to exist")
	}
	if router.hasAdapter("nonexistent") {
		t.Error("nonexistent adapter should not exist")
	}
}

func TestRouter_Close(t *testing.T) {
	router, _ := newTestRouter()

	err := router.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestRouter_Stats(t *testing.T) {
	router, _ := newTestRouter()
	defer router.Close()

	stats := router.Stats()
	if stats.AdapterCount == 0 {
		t.Error("expected at least one adapter")
	}
	if len(stats.Adapters) == 0 {
		t.Error("expected adapter names in stats")
	}
}

func TestRouter_DetermineOutputs_WithLabel(t *testing.T) {
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	router := &Router{
		logger:   logger,
		metrics:  metricsRegistry,
		adapters: map[string]Adapter{"loki": &mockAdapter{name: "loki"}},
	}

	entry := &ingestion.LogEntry{
		Labels: map[string]string{"output": "loki"},
	}

	outputs := router.determineOutputs(entry)
	if len(outputs) != 1 {
		t.Errorf("expected 1 output, got %d", len(outputs))
	}
	if outputs[0] != "loki" {
		t.Errorf("expected loki output, got %s", outputs[0])
	}
}

func TestRouter_DetermineOutputs_NoLabel(t *testing.T) {
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	router := &Router{
		logger:   logger,
		metrics:  metricsRegistry,
		adapters: map[string]Adapter{"a": &mockAdapter{}, "b": &mockAdapter{}},
	}

	entry := &ingestion.LogEntry{}

	outputs := router.determineOutputs(entry)
	if len(outputs) != 2 {
		t.Errorf("expected 2 outputs (all adapters), got %d", len(outputs))
	}
}

func TestCircuitBreaker_IsOpen(t *testing.T) {
	cb := &circuitBreaker{
		maxFailures: 3,
		cooldown:    time.Second,
	}

	if cb.isOpen() {
		t.Error("new circuit breaker should be closed")
	}

	cb.onFailure()
	cb.onFailure()
	if cb.isOpen() {
		t.Error("circuit breaker should still be closed after 2 failures")
	}

	cb.onFailure()
	if !cb.isOpen() {
		t.Error("circuit breaker should be open after 3 failures")
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := &circuitBreaker{
		maxFailures: 2,
		cooldown:    50 * time.Millisecond,
	}

	cb.onFailure()
	cb.onFailure()

	time.Sleep(100 * time.Millisecond)

	if cb.isOpen() {
		t.Error("circuit breaker should be closed after cooldown")
	}
}

func TestCircuitBreaker_OnSuccess(t *testing.T) {
	cb := &circuitBreaker{
		maxFailures: 3,
		cooldown:    time.Second,
	}

	cb.onFailure()
	cb.onFailure()
	cb.onSuccess()

	if cb.failures != 0 {
		t.Error("success should reset failures")
	}
}

func TestCircuitBreaker_Nil(t *testing.T) {
	var cb *circuitBreaker
	if cb.isOpen() {
		t.Error("nil circuit breaker should not be open")
	}
	cb.onFailure() // Should not panic
	cb.onSuccess() // Should not panic
}

func TestRouter_SendWithRetry(t *testing.T) {
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	failCount := 0
	mock := &mockAdapter{name: "retry-test"}
	mock.err = errors.New("temporary failure")

	router := &Router{
		logger:   logger,
		metrics:  metricsRegistry,
		adapters: map[string]Adapter{"retry-test": mock},
		cfgByName: map[string]config.OutputConfig{
			"retry-test": {
				Name:           "retry-test",
				Retries:        3,
				InitialBackoff: 10 * time.Millisecond,
				MaxBackoff:     50 * time.Millisecond,
				MaxFailures:    5,
				Cooldown:       time.Second,
			},
		},
		breakers: map[string]*circuitBreaker{
			"retry-test": {maxFailures: 5, cooldown: time.Second},
		},
	}

	entries := []*ingestion.LogEntry{{Message: "test"}}
	err := router.sendToOutput("retry-test", entries)
	if err == nil {
		t.Error("expected error after all retries failed")
	}

	_ = failCount // Just to use the variable
}

func TestRouterStats_Fields(t *testing.T) {
	stats := RouterStats{
		AdapterCount: 3,
		Adapters:     []string{"a", "b", "c"},
	}

	if stats.AdapterCount != 3 {
		t.Errorf("unexpected adapter count: %d", stats.AdapterCount)
	}
	if len(stats.Adapters) != 3 {
		t.Errorf("unexpected adapters length: %d", len(stats.Adapters))
	}
}
