package output

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/ingestion"
)

func TestNewS3Adapter(t *testing.T) {
	cfg := config.OutputConfig{
		Name:    "test-s3",
		Type:    "s3",
		URL:     "s3://my-bucket/logs",
		Timeout: 10 * time.Second,
	}
	logger := zerolog.Nop()

	adapter, err := NewS3Adapter(cfg, logger)
	if err != nil {
		t.Fatalf("NewS3Adapter failed: %v", err)
	}
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}

	if adapter.Name() != "test-s3" {
		t.Errorf("expected name test-s3, got %s", adapter.Name())
	}
}

func TestS3Adapter_Send(t *testing.T) {
	cfg := config.OutputConfig{
		Name: "s3",
		Type: "s3",
		URL:  "s3://bucket/logs",
	}
	logger := zerolog.Nop()

	adapter, _ := NewS3Adapter(cfg, logger)

	entries := []*ingestion.LogEntry{
		{
			Timestamp: time.Now(),
			Message:   "test log message",
			Level:     "INFO",
		},
	}

	// S3 adapter is not fully implemented, should log and return nil
	err := adapter.Send(entries)
	if err != nil {
		t.Errorf("Send failed: %v", err)
	}
}

func TestS3Adapter_Send_Empty(t *testing.T) {
	cfg := config.OutputConfig{
		Name: "s3",
		Type: "s3",
		URL:  "s3://bucket/logs",
	}
	logger := zerolog.Nop()

	adapter, _ := NewS3Adapter(cfg, logger)

	err := adapter.Send([]*ingestion.LogEntry{})
	if err != nil {
		t.Errorf("Send with empty entries should not error: %v", err)
	}
}

func TestS3Adapter_Send_Multiple(t *testing.T) {
	cfg := config.OutputConfig{
		Name: "s3",
		Type: "s3",
		URL:  "s3://bucket/logs",
	}
	logger := zerolog.Nop()

	adapter, _ := NewS3Adapter(cfg, logger)

	entries := []*ingestion.LogEntry{
		{Message: "log 1", Timestamp: time.Now()},
		{Message: "log 2", Timestamp: time.Now()},
		{Message: "log 3", Timestamp: time.Now()},
	}

	err := adapter.Send(entries)
	if err != nil {
		t.Errorf("Send failed: %v", err)
	}
}

func TestS3Adapter_Name(t *testing.T) {
	cfg := config.OutputConfig{
		Name: "custom-s3-output",
		Type: "s3",
	}
	logger := zerolog.Nop()

	adapter, _ := NewS3Adapter(cfg, logger)

	if adapter.Name() != "custom-s3-output" {
		t.Errorf("expected name custom-s3-output, got %s", adapter.Name())
	}
}

func TestS3Adapter_Close(t *testing.T) {
	cfg := config.OutputConfig{
		Name: "s3",
		Type: "s3",
	}
	logger := zerolog.Nop()

	adapter, _ := NewS3Adapter(cfg, logger)

	err := adapter.Close()
	if err != nil {
		t.Errorf("Close should not error: %v", err)
	}
}
