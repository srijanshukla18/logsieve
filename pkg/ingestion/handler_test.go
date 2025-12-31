package ingestion

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/metrics"
)

type mockProcessor struct {
	entries []*LogEntry
	err     error
}

func (m *mockProcessor) AddEntry(entry *LogEntry) error {
	if m.err != nil {
		return m.err
	}
	m.entries = append(m.entries, entry)
	return nil
}

func newTestHandler() (*Handler, *mockProcessor) {
	cfg := config.DefaultConfig()
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	handler := NewHandler(cfg, metricsRegistry, logger)
	processor := &mockProcessor{}
	handler.SetProcessor(processor)

	return handler, processor
}

func TestNewHandler(t *testing.T) {
	cfg := config.DefaultConfig()
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	handler := NewHandler(cfg, metricsRegistry, logger)
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
	if handler.parser == nil {
		t.Error("expected non-nil parser")
	}
}

func TestHandler_SetProcessor(t *testing.T) {
	handler, processor := newTestHandler()
	if handler.processor != processor {
		t.Error("processor not set correctly")
	}
}

func TestHandler_HandleIngest_SingleLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, processor := newTestHandler()

	router := gin.New()
	router.POST("/ingest", handler.HandleIngest)

	reqBody := IngestRequest{
		Log:       "test log message",
		Timestamp: "2024-01-01T12:00:00Z",
		Stream:    "stdout",
		Source:    "test-source",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Source", "test-client")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp IngestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Status != "success" {
		t.Errorf("expected status success, got %s", resp.Status)
	}
	if resp.Processed != 1 {
		t.Errorf("expected 1 processed, got %d", resp.Processed)
	}
	if len(processor.entries) != 1 {
		t.Errorf("expected 1 entry in processor, got %d", len(processor.entries))
	}
}

func TestHandler_HandleIngest_BatchLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, processor := newTestHandler()

	router := gin.New()
	router.POST("/ingest", handler.HandleIngest)

	reqBody := IngestRequest{
		Logs: []LogEntry{
			{Message: "log 1", Timestamp: time.Now()},
			{Message: "log 2", Timestamp: time.Now()},
			{Message: "log 3", Timestamp: time.Now()},
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp IngestResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Processed != 3 {
		t.Errorf("expected 3 processed, got %d", resp.Processed)
	}
	if len(processor.entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(processor.entries))
	}
}

func TestHandler_HandleIngest_InvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := newTestHandler()

	router := gin.New()
	router.POST("/ingest", handler.HandleIngest)

	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandler_HandleIngest_NoLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := newTestHandler()

	router := gin.New()
	router.POST("/ingest", handler.HandleIngest)

	reqBody := IngestRequest{}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for no logs, got %d", w.Code)
	}
}

func TestHandler_HandleIngest_WithProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, processor := newTestHandler()

	router := gin.New()
	router.POST("/ingest", handler.HandleIngest)

	reqBody := IngestRequest{
		Log: "test message",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/ingest?profile=nginx&output=loki", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if len(processor.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(processor.entries))
	}

	entry := processor.entries[0]
	if entry.Labels["profile"] != "nginx" {
		t.Errorf("expected profile nginx, got %s", entry.Labels["profile"])
	}
	if entry.Labels["output"] != "loki" {
		t.Errorf("expected output loki, got %s", entry.Labels["output"])
	}
}

func TestHandler_HandleIngest_RequestTooLarge(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.DefaultConfig()
	cfg.Ingestion.MaxRequestSize = 10 // Very small limit
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	handler := NewHandler(cfg, metricsRegistry, logger)
	processor := &mockProcessor{}
	handler.SetProcessor(processor)

	router := gin.New()
	router.POST("/ingest", handler.HandleIngest)

	reqBody := IngestRequest{
		Log: "this is a message that exceeds the limit",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413, got %d", w.Code)
	}
}

func TestHandler_HandleIngest_ProcessorError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.DefaultConfig()
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	handler := NewHandler(cfg, metricsRegistry, logger)
	processor := &mockProcessor{err: errMock}
	handler.SetProcessor(processor)

	router := gin.New()
	router.POST("/ingest", handler.HandleIngest)

	reqBody := IngestRequest{
		Log: "test message",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should still return 200 but with partial status
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp IngestResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Status != "partial" {
		t.Errorf("expected status partial, got %s", resp.Status)
	}
	if resp.Errors != 1 {
		t.Errorf("expected 1 error, got %d", resp.Errors)
	}
}

var errMock = &mockError{}

type mockError struct{}

func (e *mockError) Error() string { return "mock error" }

func TestHandler_HandleIngest_NoProcessor(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.DefaultConfig()
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()

	handler := NewHandler(cfg, metricsRegistry, logger)
	// Don't set processor

	router := gin.New()
	router.POST("/ingest", handler.HandleIngest)

	reqBody := IngestRequest{
		Log: "test message",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should succeed but not process anything
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp IngestResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Processed != 1 {
		t.Errorf("expected 1 processed (counted), got %d", resp.Processed)
	}
}

func TestLogEntry_Fields(t *testing.T) {
	entry := LogEntry{
		Timestamp:     time.Now(),
		Message:       "test message",
		Level:         "INFO",
		Source:        "test-source",
		ContainerName: "test-container",
		ContainerID:   "abc123",
		PodName:       "test-pod",
		Namespace:     "default",
		NodeName:      "node1",
		Labels:        map[string]string{"env": "test"},
		Annotations:   map[string]string{"note": "value"},
		Metadata:      map[string]interface{}{"key": "value"},
	}

	if entry.Message != "test message" {
		t.Errorf("unexpected message: %s", entry.Message)
	}
	if entry.Level != "INFO" {
		t.Errorf("unexpected level: %s", entry.Level)
	}
	if entry.ContainerName != "test-container" {
		t.Errorf("unexpected container name: %s", entry.ContainerName)
	}
}

func TestIngestRequest_Fields(t *testing.T) {
	req := IngestRequest{
		Logs:      []LogEntry{{Message: "log1"}, {Message: "log2"}},
		Log:       "single log",
		Timestamp: "2024-01-01T00:00:00Z",
		Time:      "1704067200",
		Stream:    "stdout",
		Tag:       "app.log",
		Source:    "fluent-bit",
		Labels:    map[string]string{"app": "test"},
	}

	if len(req.Logs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(req.Logs))
	}
	if req.Log != "single log" {
		t.Errorf("unexpected log: %s", req.Log)
	}
	if req.Stream != "stdout" {
		t.Errorf("unexpected stream: %s", req.Stream)
	}
}

func TestIngestResponse_Fields(t *testing.T) {
	resp := IngestResponse{
		Status:    "success",
		Processed: 10,
		Errors:    2,
		Message:   "processed with errors",
	}

	if resp.Status != "success" {
		t.Errorf("unexpected status: %s", resp.Status)
	}
	if resp.Processed != 10 {
		t.Errorf("unexpected processed: %d", resp.Processed)
	}
	if resp.Errors != 2 {
		t.Errorf("unexpected errors: %d", resp.Errors)
	}
}
