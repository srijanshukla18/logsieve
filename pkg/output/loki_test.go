package output

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/ingestion"
)

func TestNewLokiAdapter(t *testing.T) {
	cfg := config.OutputConfig{
		Name:    "test-loki",
		Type:    "loki",
		URL:     "http://localhost:3100",
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, err := NewLokiAdapter(cfg, logger)
	if err != nil {
		t.Fatalf("NewLokiAdapter failed: %v", err)
	}
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}

	if adapter.Name() != "test-loki" {
		t.Errorf("expected name test-loki, got %s", adapter.Name())
	}
	if adapter.pushURL != "http://localhost:3100/loki/api/v1/push" {
		t.Errorf("unexpected push URL: %s", adapter.pushURL)
	}
}

func TestNewLokiAdapter_TrimsTrailingSlash(t *testing.T) {
	cfg := config.OutputConfig{
		Name:    "loki",
		Type:    "loki",
		URL:     "http://localhost:3100/",
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, _ := NewLokiAdapter(cfg, logger)
	if adapter.pushURL != "http://localhost:3100/loki/api/v1/push" {
		t.Errorf("unexpected push URL: %s", adapter.pushURL)
	}
}

func TestLokiAdapter_Send_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/loki/api/v1/push" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}

		body, _ := io.ReadAll(r.Body)
		var req LokiPushRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("failed to parse request: %v", err)
		}
		if len(req.Streams) == 0 {
			t.Error("expected at least one stream")
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := config.OutputConfig{
		Name:    "loki",
		Type:    "loki",
		URL:     server.URL,
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, _ := NewLokiAdapter(cfg, logger)

	entries := []*ingestion.LogEntry{
		{
			Timestamp: time.Now(),
			Message:   "test log",
			Level:     "INFO",
			Source:    "test",
			Labels:    map[string]string{"app": "test"},
		},
	}

	err := adapter.Send(entries)
	if err != nil {
		t.Errorf("Send failed: %v", err)
	}
}

func TestLokiAdapter_Send_Empty(t *testing.T) {
	cfg := config.OutputConfig{
		Name:    "loki",
		Type:    "loki",
		URL:     "http://localhost:3100",
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, _ := NewLokiAdapter(cfg, logger)

	err := adapter.Send([]*ingestion.LogEntry{})
	if err != nil {
		t.Errorf("Send with empty entries should not error: %v", err)
	}
}

func TestLokiAdapter_Send_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	cfg := config.OutputConfig{
		Name:    "loki",
		Type:    "loki",
		URL:     server.URL,
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, _ := NewLokiAdapter(cfg, logger)

	entries := []*ingestion.LogEntry{
		{Message: "test", Timestamp: time.Now()},
	}

	err := adapter.Send(entries)
	if err == nil {
		t.Error("expected error on server error")
	}
}

func TestLokiAdapter_GroupByLabels(t *testing.T) {
	cfg := config.OutputConfig{
		Name:    "loki",
		Type:    "loki",
		URL:     "http://localhost:3100",
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, _ := NewLokiAdapter(cfg, logger)

	entries := []*ingestion.LogEntry{
		{Message: "log1", Level: "INFO", Source: "app1", Timestamp: time.Now()},
		{Message: "log2", Level: "INFO", Source: "app1", Timestamp: time.Now()},
		{Message: "log3", Level: "ERROR", Source: "app1", Timestamp: time.Now()},
	}

	streams := adapter.groupByLabels(entries)

	// Should have 2 streams: INFO/app1 and ERROR/app1
	if len(streams) < 1 {
		t.Error("expected at least one stream")
	}
}

func TestLokiAdapter_ExtractLabels(t *testing.T) {
	cfg := config.OutputConfig{
		Name:    "loki",
		Type:    "loki",
		URL:     "http://localhost:3100",
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, _ := NewLokiAdapter(cfg, logger)

	entry := &ingestion.LogEntry{
		Level:         "INFO",
		Source:        "test-source",
		ContainerName: "my-container",
		PodName:       "my-pod",
		Namespace:     "my-namespace",
		Labels:        map[string]string{"custom": "value"},
	}

	labels := adapter.extractLabels(entry)

	if labels["level"] != "INFO" {
		t.Errorf("expected level INFO, got %s", labels["level"])
	}
	if labels["source"] != "test-source" {
		t.Errorf("expected source test-source, got %s", labels["source"])
	}
	if labels["container"] != "my-container" {
		t.Errorf("expected container my-container, got %s", labels["container"])
	}
	if labels["pod"] != "my-pod" {
		t.Errorf("expected pod my-pod, got %s", labels["pod"])
	}
	if labels["namespace"] != "my-namespace" {
		t.Errorf("expected namespace my-namespace, got %s", labels["namespace"])
	}
	if labels["custom"] != "value" {
		t.Errorf("expected custom value, got %s", labels["custom"])
	}
}

func TestLokiAdapter_ExtractLabels_DefaultJob(t *testing.T) {
	cfg := config.OutputConfig{
		Name:    "loki",
		Type:    "loki",
		URL:     "http://localhost:3100",
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, _ := NewLokiAdapter(cfg, logger)

	entry := &ingestion.LogEntry{
		Message: "empty labels entry",
	}

	labels := adapter.extractLabels(entry)

	if labels["job"] != "logsieve" {
		t.Errorf("expected default job label, got %s", labels["job"])
	}
}

func TestLokiAdapter_IsHighCardinalityLabel(t *testing.T) {
	cfg := config.OutputConfig{
		Name:    "loki",
		Type:    "loki",
		URL:     "http://localhost:3100",
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, _ := NewLokiAdapter(cfg, logger)

	highCardLabels := []string{"request_id", "trace_id", "user_id", "session_id", "ip_address"}
	for _, label := range highCardLabels {
		if !adapter.isHighCardinalityLabel(label) {
			t.Errorf("expected %s to be high cardinality", label)
		}
	}

	lowCardLabels := []string{"level", "source", "app", "env"}
	for _, label := range lowCardLabels {
		if adapter.isHighCardinalityLabel(label) {
			t.Errorf("expected %s to be low cardinality", label)
		}
	}
}

func TestLokiAdapter_StructuredMetadata(t *testing.T) {
	cfg := config.OutputConfig{
		Name:    "loki",
		Type:    "loki",
		URL:     "http://localhost:3100",
		Timeout: 10 * time.Second,
		Config:  map[string]interface{}{"structuredMetadata": true},
	}
	logger := zerolog.Nop()

	adapter, _ := NewLokiAdapter(cfg, logger)

	if !adapter.useStructuredMetadata {
		t.Error("structured metadata should be enabled")
	}

	entry := &ingestion.LogEntry{
		Labels: map[string]string{
			"request_id":       "abc123",
			"context_position": "before",
		},
	}

	metadata := adapter.extractStructuredMetadata(entry)
	if metadata["request_id"] != "abc123" {
		t.Error("request_id should be in structured metadata")
	}
	if metadata["context_position"] != "before" {
		t.Error("context_ labels should be in structured metadata")
	}
}

func TestLokiAdapter_CreateStreamKey(t *testing.T) {
	cfg := config.OutputConfig{
		Name:    "loki",
		Type:    "loki",
		URL:     "http://localhost:3100",
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, _ := NewLokiAdapter(cfg, logger)

	labels1 := map[string]string{"a": "1", "b": "2"}
	labels2 := map[string]string{"b": "2", "a": "1"} // Same labels, different order

	key1 := adapter.createStreamKey(labels1)
	key2 := adapter.createStreamKey(labels2)

	if key1 != key2 {
		t.Error("stream keys should be same regardless of label order")
	}
}

func TestLokiAdapter_Close(t *testing.T) {
	cfg := config.OutputConfig{
		Name:    "loki",
		Type:    "loki",
		URL:     "http://localhost:3100",
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, _ := NewLokiAdapter(cfg, logger)

	err := adapter.Close()
	if err != nil {
		t.Errorf("Close should not error: %v", err)
	}
}

func TestLokiAdapter_WithHeaders(t *testing.T) {
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := config.OutputConfig{
		Name:    "loki",
		Type:    "loki",
		URL:     server.URL,
		Timeout: 10 * time.Second,
		Headers: map[string]string{
			"X-Custom-Header": "custom-value",
			"Authorization":   "Bearer token",
		},
	}
	logger := zerolog.Nop()

	adapter, _ := NewLokiAdapter(cfg, logger)

	entries := []*ingestion.LogEntry{
		{Message: "test", Timestamp: time.Now()},
	}

	adapter.Send(entries)

	if receivedHeaders.Get("X-Custom-Header") != "custom-value" {
		t.Error("custom header not sent")
	}
	if receivedHeaders.Get("Authorization") != "Bearer token" {
		t.Error("auth header not sent")
	}
}

func TestLokiPushRequest_Structure(t *testing.T) {
	req := LokiPushRequest{
		Streams: []LokiStream{
			{
				Stream: map[string]string{"level": "INFO"},
				Values: [][]interface{}{
					{"1234567890000000000", "log message"},
				},
			},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled LokiPushRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(unmarshaled.Streams) != 1 {
		t.Errorf("expected 1 stream, got %d", len(unmarshaled.Streams))
	}
}
