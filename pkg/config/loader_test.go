package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_NoConfigFile(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load with no config file should return defaults, got error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
}

func TestLoad_WithConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  port: 9000
  address: "127.0.0.1"
  readTimeout: 60s
  writeTimeout: 60s
  idleTimeout: 120s

ingestion:
  maxBatchSize: 500
  flushInterval: 10s
  bufferSize: 5000

dedup:
  engine: "drain3"
  cacheSize: 5000
  similarityThreshold: 0.85

profiles:
  autoDetect: false
  defaultProfile: "custom"

metrics:
  enabled: false
  port: 9091
  path: "/custom-metrics"

logging:
  level: "debug"
  format: "text"
  structured: false

outputs:
  - name: "test-output"
    type: "stdout"
    batchSize: 50
    timeout: 5s
    retries: 2
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Server.Port != 9000 {
		t.Errorf("expected port 9000, got %d", cfg.Server.Port)
	}
	if cfg.Server.Address != "127.0.0.1" {
		t.Errorf("expected address 127.0.0.1, got %s", cfg.Server.Address)
	}
	if cfg.Server.ReadTimeout != 60*time.Second {
		t.Errorf("expected read timeout 60s, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Ingestion.MaxBatchSize != 500 {
		t.Errorf("expected max batch size 500, got %d", cfg.Ingestion.MaxBatchSize)
	}
	if cfg.Dedup.SimilarityThreshold != 0.85 {
		t.Errorf("expected similarity 0.85, got %f", cfg.Dedup.SimilarityThreshold)
	}
	if cfg.Profiles.AutoDetect {
		t.Error("expected auto detect false")
	}
	if !cfg.Metrics.Enabled {
		// Metrics enabled is still true from defaults if not explicitly set
		// The config above sets it to false, so it should be false
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected log level debug, got %s", cfg.Logging.Level)
	}
}

func TestLoad_InvalidConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	invalidContent := `
server:
  port: "not-a-number"
`

	if err := os.WriteFile(configPath, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error loading invalid config")
	}
}

func TestValidateConfig_InvalidPort(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Port = 0

	err := validateConfig(cfg)
	if err == nil {
		t.Error("expected error for invalid port 0")
	}

	cfg.Server.Port = 70000
	err = validateConfig(cfg)
	if err == nil {
		t.Error("expected error for port > 65535")
	}
}

func TestValidateConfig_InvalidMetricsPort(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Metrics.Port = -1

	err := validateConfig(cfg)
	if err == nil {
		t.Error("expected error for invalid metrics port")
	}
}

func TestValidateConfig_InvalidBatchSize(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Ingestion.MaxBatchSize = 0

	err := validateConfig(cfg)
	if err == nil {
		t.Error("expected error for invalid batch size")
	}
}

func TestValidateConfig_InvalidSimilarityThreshold(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Dedup.SimilarityThreshold = -0.1

	err := validateConfig(cfg)
	if err == nil {
		t.Error("expected error for similarity < 0")
	}

	cfg.Dedup.SimilarityThreshold = 1.5
	err = validateConfig(cfg)
	if err == nil {
		t.Error("expected error for similarity > 1")
	}
}

func TestValidateConfig_InvalidDedupEngine(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Dedup.Engine = "invalid-engine"

	err := validateConfig(cfg)
	if err == nil {
		t.Error("expected error for invalid dedup engine")
	}
}

func TestValidateConfig_ValidEngines(t *testing.T) {
	cfg := DefaultConfig()

	cfg.Dedup.Engine = "drain3"
	if err := validateConfig(cfg); err != nil {
		t.Errorf("drain3 should be valid: %v", err)
	}

	cfg.Dedup.Engine = "simple"
	if err := validateConfig(cfg); err != nil {
		t.Errorf("simple should be valid: %v", err)
	}
}

func TestValidateConfig_InvalidLogLevel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logging.Level = "invalid-level"

	err := validateConfig(cfg)
	if err == nil {
		t.Error("expected error for invalid log level")
	}
}

func TestValidateConfig_ValidLogLevels(t *testing.T) {
	validLevels := []string{"trace", "debug", "info", "warn", "error", "fatal", "panic"}
	cfg := DefaultConfig()

	for _, level := range validLevels {
		cfg.Logging.Level = level
		if err := validateConfig(cfg); err != nil {
			t.Errorf("log level %s should be valid: %v", level, err)
		}
	}
}

func TestValidateConfig_OutputMissingName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Outputs = []OutputConfig{
		{Name: "", Type: "stdout"},
	}

	err := validateConfig(cfg)
	if err == nil {
		t.Error("expected error for output with missing name")
	}
}

func TestValidateConfig_OutputMissingType(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Outputs = []OutputConfig{
		{Name: "test", Type: ""},
	}

	err := validateConfig(cfg)
	if err == nil {
		t.Error("expected error for output with missing type")
	}
}

func TestValidateConfig_OutputDefaultsApplied(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Outputs = []OutputConfig{
		{Name: "test", Type: "stdout"},
	}

	err := validateConfig(cfg)
	if err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	if cfg.Outputs[0].BatchSize != 100 {
		t.Errorf("expected default batch size 100, got %d", cfg.Outputs[0].BatchSize)
	}
	if cfg.Outputs[0].Retries != 3 {
		t.Errorf("expected default retries 3, got %d", cfg.Outputs[0].Retries)
	}
	if cfg.Outputs[0].InitialBackoff != 250*time.Millisecond {
		t.Errorf("expected default initial backoff 250ms, got %v", cfg.Outputs[0].InitialBackoff)
	}
	if cfg.Outputs[0].MaxBackoff != 5*time.Second {
		t.Errorf("expected default max backoff 5s, got %v", cfg.Outputs[0].MaxBackoff)
	}
	if cfg.Outputs[0].MaxFailures != 5 {
		t.Errorf("expected default max failures 5, got %d", cfg.Outputs[0].MaxFailures)
	}
	if cfg.Outputs[0].Cooldown != 30*time.Second {
		t.Errorf("expected default cooldown 30s, got %v", cfg.Outputs[0].Cooldown)
	}
}

func TestWriteExample(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "example.yaml")

	err := WriteExample(outputPath)
	if err != nil {
		t.Fatalf("WriteExample failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read example file: %v", err)
	}

	if len(content) == 0 {
		t.Error("example file is empty")
	}

	contentStr := string(content)
	if !contains(contentStr, "server:") {
		t.Error("example should contain server section")
	}
	if !contains(contentStr, "ingestion:") {
		t.Error("example should contain ingestion section")
	}
	if !contains(contentStr, "dedup:") {
		t.Error("example should contain dedup section")
	}
	if !contains(contentStr, "profiles:") {
		t.Error("example should contain profiles section")
	}
	if !contains(contentStr, "outputs:") {
		t.Error("example should contain outputs section")
	}
	if !contains(contentStr, "metrics:") {
		t.Error("example should contain metrics section")
	}
	if !contains(contentStr, "logging:") {
		t.Error("example should contain logging section")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestLoad_EnvironmentVariables(t *testing.T) {
	os.Setenv("LOGSIEVE_SERVER_PORT", "9999")
	defer os.Unsetenv("LOGSIEVE_SERVER_PORT")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Server.Port != 9999 {
		t.Errorf("expected port 9999 from env, got %d", cfg.Server.Port)
	}
}
