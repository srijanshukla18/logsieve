package output

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/ingestion"
	"github.com/logsieve/logsieve/pkg/metrics"
)

func TestNewS3Adapter(t *testing.T) {
	cfg := config.OutputConfig{
		Name:    "test-s3",
		Type:    "s3",
		Timeout: 10 * time.Second,
		Config: map[string]interface{}{
			"bucket": "my-bucket",
			"region": "us-east-1",
			"prefix": "logs/",
		},
	}
	logger := zerolog.Nop()
	metricsRegistry := metrics.NewRegistry()

	adapter, err := NewS3Adapter(cfg, metricsRegistry, logger)
	if err != nil {
		t.Fatalf("NewS3Adapter failed: %v", err)
	}
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}
	defer adapter.Close()

	if adapter.Name() != "test-s3" {
		t.Errorf("expected name test-s3, got %s", adapter.Name())
	}
}

func TestS3Adapter_Send(t *testing.T) {
	cfg := config.OutputConfig{
		Name: "s3",
		Type: "s3",
		Config: map[string]interface{}{
			"bucket": "test-bucket",
			"region": "us-east-1",
		},
	}
	logger := zerolog.Nop()
	metricsRegistry := metrics.NewRegistry()

	adapter, err := NewS3Adapter(cfg, metricsRegistry, logger)
	if err != nil {
		t.Fatalf("NewS3Adapter failed: %v", err)
	}
	defer adapter.Close()

	entries := []*ingestion.LogEntry{
		{
			Timestamp: time.Now(),
			Message:   "test log message",
			Level:     "INFO",
		},
	}

	// S3 adapter buffers entries, should not error
	err = adapter.Send(entries)
	if err != nil {
		t.Errorf("Send failed: %v", err)
	}
}

func TestS3Adapter_Send_Empty(t *testing.T) {
	cfg := config.OutputConfig{
		Name: "s3",
		Type: "s3",
		Config: map[string]interface{}{
			"bucket": "test-bucket",
			"region": "us-east-1",
		},
	}
	logger := zerolog.Nop()
	metricsRegistry := metrics.NewRegistry()

	adapter, err := NewS3Adapter(cfg, metricsRegistry, logger)
	if err != nil {
		t.Fatalf("NewS3Adapter failed: %v", err)
	}
	defer adapter.Close()

	err = adapter.Send([]*ingestion.LogEntry{})
	if err != nil {
		t.Errorf("Send with empty entries should not error: %v", err)
	}
}

func TestS3Adapter_Send_Multiple(t *testing.T) {
	cfg := config.OutputConfig{
		Name: "s3",
		Type: "s3",
		Config: map[string]interface{}{
			"bucket": "test-bucket",
			"region": "us-east-1",
		},
	}
	logger := zerolog.Nop()
	metricsRegistry := metrics.NewRegistry()

	adapter, err := NewS3Adapter(cfg, metricsRegistry, logger)
	if err != nil {
		t.Fatalf("NewS3Adapter failed: %v", err)
	}
	defer adapter.Close()

	entries := []*ingestion.LogEntry{
		{Message: "log 1", Timestamp: time.Now()},
		{Message: "log 2", Timestamp: time.Now()},
		{Message: "log 3", Timestamp: time.Now()},
	}

	err = adapter.Send(entries)
	if err != nil {
		t.Errorf("Send failed: %v", err)
	}
}

func TestS3Adapter_Name(t *testing.T) {
	cfg := config.OutputConfig{
		Name: "custom-s3-output",
		Type: "s3",
		Config: map[string]interface{}{
			"bucket": "test-bucket",
		},
	}
	logger := zerolog.Nop()
	metricsRegistry := metrics.NewRegistry()

	adapter, err := NewS3Adapter(cfg, metricsRegistry, logger)
	if err != nil {
		t.Fatalf("NewS3Adapter failed: %v", err)
	}
	defer adapter.Close()

	if adapter.Name() != "custom-s3-output" {
		t.Errorf("expected name custom-s3-output, got %s", adapter.Name())
	}
}

func TestS3Adapter_Close(t *testing.T) {
	cfg := config.OutputConfig{
		Name: "s3",
		Type: "s3",
		Config: map[string]interface{}{
			"bucket": "test-bucket",
		},
	}
	logger := zerolog.Nop()
	metricsRegistry := metrics.NewRegistry()

	adapter, err := NewS3Adapter(cfg, metricsRegistry, logger)
	if err != nil {
		t.Fatalf("NewS3Adapter failed: %v", err)
	}

	err = adapter.Close()
	if err != nil {
		t.Errorf("Close should not error: %v", err)
	}
}

func TestNewS3Adapter_MissingBucket(t *testing.T) {
	cfg := config.OutputConfig{
		Name: "s3",
		Type: "s3",
		Config: map[string]interface{}{
			"region": "us-east-1",
		},
	}
	logger := zerolog.Nop()
	metricsRegistry := metrics.NewRegistry()

	_, err := NewS3Adapter(cfg, metricsRegistry, logger)
	if err == nil {
		t.Error("expected error for missing bucket")
	}
}
