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

func TestNewElasticsearchAdapter(t *testing.T) {
	cfg := config.OutputConfig{
		Name:    "test-es",
		Type:    "elasticsearch",
		URL:     "http://localhost:9200",
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, err := NewElasticsearchAdapter(cfg, logger)
	if err != nil {
		t.Fatalf("NewElasticsearchAdapter failed: %v", err)
	}
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}

	if adapter.Name() != "test-es" {
		t.Errorf("expected name test-es, got %s", adapter.Name())
	}
	if adapter.bulkURL != "http://localhost:9200/_bulk" {
		t.Errorf("unexpected bulk URL: %s", adapter.bulkURL)
	}
	if adapter.indexName != "logs" {
		t.Errorf("expected default index logs, got %s", adapter.indexName)
	}
}

func TestNewElasticsearchAdapter_CustomIndex(t *testing.T) {
	cfg := config.OutputConfig{
		Name:    "es",
		Type:    "elasticsearch",
		URL:     "http://localhost:9200",
		Timeout: 10 * time.Second,
		Config:  map[string]interface{}{"index": "custom-logs"},
	}
	logger := zerolog.Nop()

	adapter, _ := NewElasticsearchAdapter(cfg, logger)
	if adapter.indexName != "custom-logs" {
		t.Errorf("expected index custom-logs, got %s", adapter.indexName)
	}
}

func TestElasticsearchAdapter_Send_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/_bulk" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/x-ndjson" {
			t.Errorf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}

		body, _ := io.ReadAll(r.Body)
		if len(body) == 0 {
			t.Error("expected non-empty body")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"took": 10, "errors": false, "items": []}`))
	}))
	defer server.Close()

	cfg := config.OutputConfig{
		Name:    "es",
		Type:    "elasticsearch",
		URL:     server.URL,
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, _ := NewElasticsearchAdapter(cfg, logger)

	entries := []*ingestion.LogEntry{
		{
			Timestamp:     time.Now(),
			Message:       "test log",
			Level:         "INFO",
			Source:        "test",
			ContainerName: "container",
			PodName:       "pod",
			Namespace:     "namespace",
			Labels:        map[string]string{"app": "test"},
		},
	}

	err := adapter.Send(entries)
	if err != nil {
		t.Errorf("Send failed: %v", err)
	}
}

func TestElasticsearchAdapter_Send_Empty(t *testing.T) {
	cfg := config.OutputConfig{
		Name:    "es",
		Type:    "elasticsearch",
		URL:     "http://localhost:9200",
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, _ := NewElasticsearchAdapter(cfg, logger)

	err := adapter.Send([]*ingestion.LogEntry{})
	if err != nil {
		t.Errorf("Send with empty entries should not error: %v", err)
	}
}

func TestElasticsearchAdapter_Send_BulkErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"took": 10,
			"errors": true,
			"items": [
				{"index": {"_index": "logs", "status": 400, "error": {"type": "mapper_parsing_exception"}}}
			]
		}`))
	}))
	defer server.Close()

	cfg := config.OutputConfig{
		Name:    "es",
		Type:    "elasticsearch",
		URL:     server.URL,
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, _ := NewElasticsearchAdapter(cfg, logger)

	entries := []*ingestion.LogEntry{
		{Message: "test", Timestamp: time.Now()},
	}

	err := adapter.Send(entries)
	if err == nil {
		t.Error("expected error on bulk errors")
	}
}

func TestElasticsearchAdapter_Send_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal error"}`))
	}))
	defer server.Close()

	cfg := config.OutputConfig{
		Name:    "es",
		Type:    "elasticsearch",
		URL:     server.URL,
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, _ := NewElasticsearchAdapter(cfg, logger)

	entries := []*ingestion.LogEntry{
		{Message: "test", Timestamp: time.Now()},
	}

	err := adapter.Send(entries)
	if err == nil {
		t.Error("expected error on server error")
	}
}

func TestElasticsearchAdapter_GetIndexName(t *testing.T) {
	cfg := config.OutputConfig{
		Name:    "es",
		Type:    "elasticsearch",
		URL:     "http://localhost:9200",
		Timeout: 10 * time.Second,
		Config:  map[string]interface{}{"index": "myindex"},
	}
	logger := zerolog.Nop()

	adapter, _ := NewElasticsearchAdapter(cfg, logger)

	ts := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	entry := &ingestion.LogEntry{
		Timestamp: ts,
		Message:   "test",
	}

	indexName := adapter.getIndexName(entry)
	expected := "myindex-2024.01.15"
	if indexName != expected {
		t.Errorf("expected index %s, got %s", expected, indexName)
	}
}

func TestElasticsearchAdapter_GetIndexName_FromLabel(t *testing.T) {
	cfg := config.OutputConfig{
		Name:    "es",
		Type:    "elasticsearch",
		URL:     "http://localhost:9200",
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, _ := NewElasticsearchAdapter(cfg, logger)

	entry := &ingestion.LogEntry{
		Timestamp: time.Now(),
		Message:   "test",
		Labels:    map[string]string{"index": "custom-index"},
	}

	indexName := adapter.getIndexName(entry)
	if indexName != "custom-index" {
		t.Errorf("expected index custom-index, got %s", indexName)
	}
}

func TestElasticsearchAdapter_ConvertToESDocument(t *testing.T) {
	cfg := config.OutputConfig{
		Name:    "es",
		Type:    "elasticsearch",
		URL:     "http://localhost:9200",
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, _ := NewElasticsearchAdapter(cfg, logger)

	ts := time.Now()
	entry := &ingestion.LogEntry{
		Timestamp:     ts,
		Message:       "test message",
		Level:         "ERROR",
		Source:        "source",
		ContainerName: "container",
		ContainerID:   "abc123",
		PodName:       "pod",
		Namespace:     "namespace",
		NodeName:      "node",
		Labels: map[string]string{
			"host_name":       "myhost",
			"host_ip":         "192.168.1.1",
			"service_name":    "myservice",
			"service_version": "1.0.0",
		},
	}

	doc := adapter.convertToESDocument(entry)

	if doc.Timestamp != ts {
		t.Error("timestamp mismatch")
	}
	if doc.Message != "test message" {
		t.Error("message mismatch")
	}
	if doc.Level != "ERROR" {
		t.Error("level mismatch")
	}
	if doc.ContainerName != "container" {
		t.Error("container name mismatch")
	}
	if doc.PodName != "pod" {
		t.Error("pod name mismatch")
	}
	if doc.Namespace != "namespace" {
		t.Error("namespace mismatch")
	}
	if doc.NodeName != "node" {
		t.Error("node name mismatch")
	}
	if doc.Host.Name != "myhost" {
		t.Error("host name mismatch")
	}
	if doc.Host.IP != "192.168.1.1" {
		t.Error("host IP mismatch")
	}
	if doc.Service.Name != "myservice" {
		t.Error("service name mismatch")
	}
	if doc.Service.Version != "1.0.0" {
		t.Error("service version mismatch")
	}
}

func TestElasticsearchAdapter_ExtractBulkErrors(t *testing.T) {
	cfg := config.OutputConfig{
		Name:    "es",
		Type:    "elasticsearch",
		URL:     "http://localhost:9200",
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, _ := NewElasticsearchAdapter(cfg, logger)

	items := []map[string]interface{}{
		{"index": map[string]interface{}{"status": 400, "error": map[string]interface{}{"type": "error1"}}},
		{"index": map[string]interface{}{"status": 200}},
		{"index": map[string]interface{}{"status": 400, "error": map[string]interface{}{"type": "error2"}}},
	}

	errors := adapter.extractBulkErrors(items)
	if errors == "" {
		t.Error("expected error string")
	}
}

func TestElasticsearchAdapter_Close(t *testing.T) {
	cfg := config.OutputConfig{
		Name:    "es",
		Type:    "elasticsearch",
		URL:     "http://localhost:9200",
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, _ := NewElasticsearchAdapter(cfg, logger)

	err := adapter.Close()
	if err != nil {
		t.Errorf("Close should not error: %v", err)
	}
}

func TestElasticsearchAdapter_WithHeaders(t *testing.T) {
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"took": 10, "errors": false, "items": []}`))
	}))
	defer server.Close()

	cfg := config.OutputConfig{
		Name:    "es",
		Type:    "elasticsearch",
		URL:     server.URL,
		Timeout: 10 * time.Second,
		Headers: map[string]string{
			"Authorization": "ApiKey abc123",
		},
	}
	logger := zerolog.Nop()

	adapter, _ := NewElasticsearchAdapter(cfg, logger)

	entries := []*ingestion.LogEntry{
		{Message: "test", Timestamp: time.Now()},
	}

	adapter.Send(entries)

	if receivedHeaders.Get("Authorization") != "ApiKey abc123" {
		t.Error("auth header not sent")
	}
}

func TestESDocument_JSONSerialization(t *testing.T) {
	doc := ESDocument{
		Timestamp:     time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Message:       "test",
		Level:         "INFO",
		ContainerName: "container",
		PodName:       "pod",
		Namespace:     "ns",
		Labels:        map[string]string{"key": "value"},
		Host:          ESHost{Name: "host", IP: "1.2.3.4"},
		Service:       ESService{Name: "svc", Version: "1.0"},
	}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled map[string]interface{}
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled["message"] != "test" {
		t.Error("message not serialized correctly")
	}
	if unmarshaled["log.level"] != "INFO" {
		t.Error("level not serialized with ECS field name")
	}
}
