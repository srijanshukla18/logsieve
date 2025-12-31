package config

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	// Server defaults
	if cfg.Server.Port != 8080 {
		t.Errorf("expected server port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.Address != "0.0.0.0" {
		t.Errorf("expected server address 0.0.0.0, got %s", cfg.Server.Address)
	}
	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Errorf("expected read timeout 30s, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 30*time.Second {
		t.Errorf("expected write timeout 30s, got %v", cfg.Server.WriteTimeout)
	}
	if cfg.Server.IdleTimeout != 60*time.Second {
		t.Errorf("expected idle timeout 60s, got %v", cfg.Server.IdleTimeout)
	}

	// Ingestion defaults
	if cfg.Ingestion.MaxBatchSize != 1000 {
		t.Errorf("expected max batch size 1000, got %d", cfg.Ingestion.MaxBatchSize)
	}
	if cfg.Ingestion.FlushInterval != 5*time.Second {
		t.Errorf("expected flush interval 5s, got %v", cfg.Ingestion.FlushInterval)
	}
	if cfg.Ingestion.MaxMemoryMB != 100 {
		t.Errorf("expected max memory 100MB, got %d", cfg.Ingestion.MaxMemoryMB)
	}
	if cfg.Ingestion.BufferSize != 10000 {
		t.Errorf("expected buffer size 10000, got %d", cfg.Ingestion.BufferSize)
	}
	if cfg.Ingestion.MaxRequestSize != 10*1024*1024 {
		t.Errorf("expected max request size 10MB, got %d", cfg.Ingestion.MaxRequestSize)
	}
	if cfg.Ingestion.QueueType != "memory" {
		t.Errorf("expected queue type memory, got %s", cfg.Ingestion.QueueType)
	}

	// Dedup defaults
	if cfg.Dedup.Engine != "drain3" {
		t.Errorf("expected dedup engine drain3, got %s", cfg.Dedup.Engine)
	}
	if cfg.Dedup.CacheSize != 10000 {
		t.Errorf("expected cache size 10000, got %d", cfg.Dedup.CacheSize)
	}
	if cfg.Dedup.SimilarityThreshold != 0.9 {
		t.Errorf("expected similarity threshold 0.9, got %f", cfg.Dedup.SimilarityThreshold)
	}
	if cfg.Dedup.ContextLines != 5 {
		t.Errorf("expected context lines 5, got %d", cfg.Dedup.ContextLines)
	}

	// Profiles defaults
	if !cfg.Profiles.AutoDetect {
		t.Error("expected auto detect to be true")
	}
	if cfg.Profiles.HubURL != "https://hub.logsieve.io" {
		t.Errorf("expected hub URL https://hub.logsieve.io, got %s", cfg.Profiles.HubURL)
	}
	if cfg.Profiles.DefaultProfile != "generic" {
		t.Errorf("expected default profile generic, got %s", cfg.Profiles.DefaultProfile)
	}
	if cfg.Profiles.TrustMode != "relaxed" {
		t.Errorf("expected trust mode relaxed, got %s", cfg.Profiles.TrustMode)
	}

	// Metrics defaults
	if !cfg.Metrics.Enabled {
		t.Error("expected metrics to be enabled")
	}
	if cfg.Metrics.Port != 9090 {
		t.Errorf("expected metrics port 9090, got %d", cfg.Metrics.Port)
	}
	if cfg.Metrics.Path != "/metrics" {
		t.Errorf("expected metrics path /metrics, got %s", cfg.Metrics.Path)
	}

	// Logging defaults
	if cfg.Logging.Level != "info" {
		t.Errorf("expected log level info, got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("expected log format json, got %s", cfg.Logging.Format)
	}
	if cfg.Logging.Output != "stdout" {
		t.Errorf("expected log output stdout, got %s", cfg.Logging.Output)
	}
	if !cfg.Logging.Structured {
		t.Error("expected structured logging to be true")
	}

	// Default outputs
	if len(cfg.Outputs) != 1 {
		t.Fatalf("expected 1 default output, got %d", len(cfg.Outputs))
	}
	if cfg.Outputs[0].Name != "stdout" {
		t.Errorf("expected output name stdout, got %s", cfg.Outputs[0].Name)
	}
	if cfg.Outputs[0].Type != "stdout" {
		t.Errorf("expected output type stdout, got %s", cfg.Outputs[0].Type)
	}
}

func TestServerConfig_Timeouts(t *testing.T) {
	cfg := ServerConfig{
		Port:         8080,
		Address:      "localhost",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 20 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	if cfg.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Port)
	}
	if cfg.Address != "localhost" {
		t.Errorf("expected address localhost, got %s", cfg.Address)
	}
}

func TestIngestionConfig_Fields(t *testing.T) {
	cfg := IngestionConfig{
		MaxBatchSize:   500,
		FlushInterval:  10 * time.Second,
		MaxMemoryMB:    200,
		BufferSize:     5000,
		MaxRequestSize: 5 * 1024 * 1024,
		QueueType:      "disk",
		DiskPath:       "/tmp/queue",
		MaxDiskBytes:   2 * 1024 * 1024 * 1024,
	}

	if cfg.MaxBatchSize != 500 {
		t.Errorf("expected max batch size 500, got %d", cfg.MaxBatchSize)
	}
	if cfg.QueueType != "disk" {
		t.Errorf("expected queue type disk, got %s", cfg.QueueType)
	}
}

func TestDedupConfig_Fields(t *testing.T) {
	cfg := DedupConfig{
		Engine:              "simple",
		CacheSize:           5000,
		ContextLines:        3,
		SimilarityThreshold: 0.8,
		PatternTTL:          2 * time.Hour,
		FingerprintTTL:      15 * time.Minute,
	}

	if cfg.Engine != "simple" {
		t.Errorf("expected engine simple, got %s", cfg.Engine)
	}
	if cfg.SimilarityThreshold != 0.8 {
		t.Errorf("expected similarity 0.8, got %f", cfg.SimilarityThreshold)
	}
}

func TestProfilesConfig_Fields(t *testing.T) {
	cfg := ProfilesConfig{
		AutoDetect:     false,
		HubURL:         "https://custom.hub",
		SyncInterval:   30 * time.Minute,
		LocalPath:      "/custom/profiles",
		CachePath:      "/custom/cache",
		DefaultProfile: "custom",
		TrustMode:      "strict",
		PublicKeys:     []string{"key1", "key2"},
	}

	if cfg.AutoDetect {
		t.Error("expected auto detect false")
	}
	if cfg.TrustMode != "strict" {
		t.Errorf("expected trust mode strict, got %s", cfg.TrustMode)
	}
	if len(cfg.PublicKeys) != 2 {
		t.Errorf("expected 2 public keys, got %d", len(cfg.PublicKeys))
	}
}

func TestOutputConfig_Fields(t *testing.T) {
	cfg := OutputConfig{
		Name:           "loki-output",
		Type:           "loki",
		URL:            "http://loki:3100",
		BatchSize:      200,
		Timeout:        30 * time.Second,
		Retries:        5,
		Headers:        map[string]string{"X-Custom": "value"},
		Config:         map[string]interface{}{"index": "logs"},
		InitialBackoff: 500 * time.Millisecond,
		MaxBackoff:     10 * time.Second,
		MaxFailures:    10,
		Cooldown:       1 * time.Minute,
	}

	if cfg.Name != "loki-output" {
		t.Errorf("expected name loki-output, got %s", cfg.Name)
	}
	if cfg.Type != "loki" {
		t.Errorf("expected type loki, got %s", cfg.Type)
	}
	if cfg.Retries != 5 {
		t.Errorf("expected retries 5, got %d", cfg.Retries)
	}
	if cfg.MaxFailures != 10 {
		t.Errorf("expected max failures 10, got %d", cfg.MaxFailures)
	}
}

func TestMetricsConfig_Fields(t *testing.T) {
	cfg := MetricsConfig{
		Enabled: false,
		Port:    9091,
		Path:    "/custom-metrics",
	}

	if cfg.Enabled {
		t.Error("expected enabled false")
	}
	if cfg.Port != 9091 {
		t.Errorf("expected port 9091, got %d", cfg.Port)
	}
	if cfg.Path != "/custom-metrics" {
		t.Errorf("expected path /custom-metrics, got %s", cfg.Path)
	}
}

func TestLoggingConfig_Fields(t *testing.T) {
	cfg := LoggingConfig{
		Level:      "debug",
		Format:     "text",
		Output:     "stderr",
		Structured: false,
	}

	if cfg.Level != "debug" {
		t.Errorf("expected level debug, got %s", cfg.Level)
	}
	if cfg.Format != "text" {
		t.Errorf("expected format text, got %s", cfg.Format)
	}
	if cfg.Structured {
		t.Error("expected structured false")
	}
}
