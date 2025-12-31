package output

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/ingestion"
)

func TestNewStdoutAdapter(t *testing.T) {
	cfg := config.OutputConfig{
		Name:      "test-stdout",
		Type:      "stdout",
		BatchSize: 100,
	}
	logger := zerolog.Nop()

	adapter := NewStdoutAdapter(cfg, logger)
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}

	if adapter.Name() != "test-stdout" {
		t.Errorf("expected name test-stdout, got %s", adapter.Name())
	}
}

func TestStdoutAdapter_Send(t *testing.T) {
	cfg := config.OutputConfig{
		Name: "stdout",
		Type: "stdout",
	}
	logger := zerolog.Nop()

	adapter := NewStdoutAdapter(cfg, logger)

	entries := []*ingestion.LogEntry{
		{
			Timestamp:     time.Now(),
			Message:       "test log message",
			Level:         "INFO",
			Source:        "test-source",
			ContainerName: "test-container",
			PodName:       "test-pod",
			Namespace:     "default",
			Labels:        map[string]string{"app": "test"},
		},
	}

	err := adapter.Send(entries)
	if err != nil {
		t.Errorf("Send failed: %v", err)
	}
}

func TestStdoutAdapter_Send_Empty(t *testing.T) {
	cfg := config.OutputConfig{Name: "stdout", Type: "stdout"}
	logger := zerolog.Nop()
	adapter := NewStdoutAdapter(cfg, logger)

	err := adapter.Send([]*ingestion.LogEntry{})
	if err != nil {
		t.Errorf("Send with empty entries should not error: %v", err)
	}
}

func TestStdoutAdapter_Send_Multiple(t *testing.T) {
	cfg := config.OutputConfig{Name: "stdout", Type: "stdout"}
	logger := zerolog.Nop()
	adapter := NewStdoutAdapter(cfg, logger)

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

func TestStdoutAdapter_Name(t *testing.T) {
	cfg := config.OutputConfig{Name: "custom-name", Type: "stdout"}
	logger := zerolog.Nop()
	adapter := NewStdoutAdapter(cfg, logger)

	if adapter.Name() != "custom-name" {
		t.Errorf("expected name custom-name, got %s", adapter.Name())
	}
}

func TestStdoutAdapter_Close(t *testing.T) {
	cfg := config.OutputConfig{Name: "stdout", Type: "stdout"}
	logger := zerolog.Nop()
	adapter := NewStdoutAdapter(cfg, logger)

	err := adapter.Close()
	if err != nil {
		t.Errorf("Close should not error: %v", err)
	}
}

func TestStdoutAdapter_Send_AllFields(t *testing.T) {
	cfg := config.OutputConfig{Name: "stdout", Type: "stdout"}
	logger := zerolog.Nop()
	adapter := NewStdoutAdapter(cfg, logger)

	entries := []*ingestion.LogEntry{
		{
			Timestamp:     time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			Message:       "complete entry",
			Level:         "ERROR",
			Source:        "source",
			ContainerName: "container",
			ContainerID:   "abc123",
			PodName:       "pod",
			Namespace:     "namespace",
			NodeName:      "node",
			Labels:        map[string]string{"key": "value"},
			Annotations:   map[string]string{"anno": "val"},
			Metadata:      map[string]interface{}{"meta": "data"},
		},
	}

	err := adapter.Send(entries)
	if err != nil {
		t.Errorf("Send failed: %v", err)
	}
}

func TestStdoutAdapter_Send_NilLabels(t *testing.T) {
	cfg := config.OutputConfig{Name: "stdout", Type: "stdout"}
	logger := zerolog.Nop()
	adapter := NewStdoutAdapter(cfg, logger)

	entries := []*ingestion.LogEntry{
		{
			Message:   "no labels",
			Timestamp: time.Now(),
			Labels:    nil,
		},
	}

	err := adapter.Send(entries)
	if err != nil {
		t.Errorf("Send with nil labels should not error: %v", err)
	}
}
