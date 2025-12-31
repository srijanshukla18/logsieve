package profiles

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
	"github.com/logsieve/logsieve/pkg/ingestion"
	"github.com/logsieve/logsieve/pkg/metrics"
)

func newTestManager() *Manager {
	cfg := config.ProfilesConfig{
		AutoDetect:     true,
		DefaultProfile: "generic",
		TrustMode:      "relaxed",
	}
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()
	return NewManager(cfg, metricsRegistry, logger)
}

func TestNewManager(t *testing.T) {
	m := newTestManager()
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.detector == nil {
		t.Error("expected non-nil detector")
	}
}

func TestManager_LoadProfiles(t *testing.T) {
	m := newTestManager()

	err := m.LoadProfiles()
	if err != nil {
		t.Fatalf("LoadProfiles failed: %v", err)
	}

	profiles := m.ListProfiles()
	if len(profiles) < 2 {
		t.Error("expected at least generic and nginx builtin profiles")
	}

	foundGeneric := false
	foundNginx := false
	for _, name := range profiles {
		if name == "generic" {
			foundGeneric = true
		}
		if name == "nginx" {
			foundNginx = true
		}
	}
	if !foundGeneric {
		t.Error("expected generic profile")
	}
	if !foundNginx {
		t.Error("expected nginx profile")
	}
}

func TestManager_GetProfile(t *testing.T) {
	m := newTestManager()
	m.LoadProfiles()

	profile, err := m.GetProfile("generic")
	if err != nil {
		t.Fatalf("GetProfile failed: %v", err)
	}
	if profile.Metadata.Name != "generic" {
		t.Errorf("unexpected profile name: %s", profile.Metadata.Name)
	}
}

func TestManager_GetProfile_NotFound(t *testing.T) {
	m := newTestManager()
	m.LoadProfiles()

	_, err := m.GetProfile("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent profile")
	}
}

func TestManager_DetectProfile(t *testing.T) {
	m := newTestManager()
	m.LoadProfiles()

	entry := &ingestion.LogEntry{
		ContainerName: "nginx-proxy",
		Message:       `192.168.1.1 - - [01/Jan/2024:12:00:00 +0000] "GET /api HTTP/1.1" 200`,
	}

	profile := m.DetectProfile(entry)
	if profile != "nginx" {
		t.Errorf("expected nginx profile, got %s", profile)
	}
}

func TestManager_DetectProfile_FromLabel(t *testing.T) {
	m := newTestManager()
	m.LoadProfiles()

	entry := &ingestion.LogEntry{
		Message: "some log",
		Labels:  map[string]string{"profile": "custom-profile"},
	}

	profile := m.DetectProfile(entry)
	if profile != "custom-profile" {
		t.Errorf("expected custom-profile from label, got %s", profile)
	}
}

func TestManager_DetectProfile_AutoLabel(t *testing.T) {
	m := newTestManager()
	m.LoadProfiles()

	entry := &ingestion.LogEntry{
		Message: "some log",
		Labels:  map[string]string{"profile": "auto"},
	}

	// With auto label, should use detection or default
	profile := m.DetectProfile(entry)
	if profile == "" {
		t.Error("profile should not be empty")
	}
}

func TestManager_DetectProfile_AutoDetectDisabled(t *testing.T) {
	cfg := config.ProfilesConfig{
		AutoDetect:     false,
		DefaultProfile: "generic",
	}
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()
	m := NewManager(cfg, metricsRegistry, logger)
	m.LoadProfiles()

	entry := &ingestion.LogEntry{
		ContainerName: "nginx-proxy",
		Message:       "nginx log",
	}

	profile := m.DetectProfile(entry)
	if profile != "generic" {
		t.Errorf("expected default profile when auto detect disabled, got %s", profile)
	}
}

func TestManager_ProcessWithProfile(t *testing.T) {
	m := newTestManager()
	m.LoadProfiles()

	entry := &ingestion.LogEntry{
		Message: "test log message",
	}

	result, err := m.ProcessWithProfile(entry, "generic")
	if err != nil {
		t.Fatalf("ProcessWithProfile failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Entry != entry {
		t.Error("entry mismatch")
	}
	if result.Profile != "generic" {
		t.Errorf("expected profile generic, got %s", result.Profile)
	}
}

func TestManager_ProcessWithProfile_NonexistentFallback(t *testing.T) {
	m := newTestManager()
	m.LoadProfiles()

	entry := &ingestion.LogEntry{
		Message: "test log",
	}

	result, err := m.ProcessWithProfile(entry, "nonexistent")
	if err != nil {
		t.Fatalf("ProcessWithProfile failed: %v", err)
	}
	if result.Profile != "generic" {
		t.Errorf("expected fallback to generic, got %s", result.Profile)
	}
}

func TestManager_AddProfile(t *testing.T) {
	m := newTestManager()
	m.LoadProfiles()

	profile := &Profile{
		Metadata: ProfileMetadata{
			Name:    "custom",
			Version: "1.0.0",
		},
		Spec: ProfileSpec{
			Fingerprints: []FingerprintRule{
				{Pattern: ".*", Action: "template"},
			},
		},
	}

	err := m.AddProfile(profile)
	if err != nil {
		t.Fatalf("AddProfile failed: %v", err)
	}

	retrieved, err := m.GetProfile("custom")
	if err != nil {
		t.Fatalf("GetProfile failed: %v", err)
	}
	if retrieved.Metadata.Name != "custom" {
		t.Error("profile not added correctly")
	}
}

func TestManager_AddProfile_InvalidName(t *testing.T) {
	m := newTestManager()

	profile := &Profile{
		Metadata: ProfileMetadata{
			Name:    "",
			Version: "1.0.0",
		},
	}

	err := m.AddProfile(profile)
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestManager_AddProfile_InvalidVersion(t *testing.T) {
	m := newTestManager()

	profile := &Profile{
		Metadata: ProfileMetadata{
			Name:    "test",
			Version: "",
		},
	}

	err := m.AddProfile(profile)
	if err == nil {
		t.Error("expected error for empty version")
	}
}

func TestManager_RemoveProfile(t *testing.T) {
	m := newTestManager()
	m.LoadProfiles()

	// Add and then remove
	profile := &Profile{
		Metadata: ProfileMetadata{
			Name:    "to-remove",
			Version: "1.0.0",
		},
		Spec: ProfileSpec{},
	}
	m.AddProfile(profile)

	err := m.RemoveProfile("to-remove")
	if err != nil {
		t.Fatalf("RemoveProfile failed: %v", err)
	}

	_, err = m.GetProfile("to-remove")
	if err == nil {
		t.Error("expected error after removal")
	}
}

func TestManager_RemoveProfile_NotFound(t *testing.T) {
	m := newTestManager()

	err := m.RemoveProfile("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent profile")
	}
}

func TestManager_ListProfiles(t *testing.T) {
	m := newTestManager()
	m.LoadProfiles()

	profiles := m.ListProfiles()
	if len(profiles) == 0 {
		t.Error("expected some profiles")
	}
}

func TestManager_GetStats(t *testing.T) {
	m := newTestManager()
	m.LoadProfiles()

	stats := m.GetStats()
	if stats.ProfileCount == 0 {
		t.Error("expected some profiles")
	}
	if len(stats.Profiles) != stats.ProfileCount {
		t.Error("profile count mismatch")
	}
}

func TestManager_LoadLocalProfiles(t *testing.T) {
	tmpDir := t.TempDir()

	profileContent := `
apiVersion: logsieve.io/v1
kind: Profile
metadata:
  name: local-test
  version: "1.0.0"
spec:
  fingerprints:
    - pattern: ".*"
      action: template
`
	profilePath := filepath.Join(tmpDir, "test.yaml")
	if err := os.WriteFile(profilePath, []byte(profileContent), 0644); err != nil {
		t.Fatalf("failed to write profile: %v", err)
	}

	cfg := config.ProfilesConfig{
		AutoDetect:     true,
		DefaultProfile: "generic",
		LocalPath:      tmpDir,
		TrustMode:      "offline",
	}
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()
	m := NewManager(cfg, metricsRegistry, logger)

	err := m.LoadProfiles()
	if err != nil {
		t.Fatalf("LoadProfiles failed: %v", err)
	}

	_, err = m.GetProfile("local-test")
	if err != nil {
		t.Error("local profile should be loaded")
	}
}

func TestProcessedEntry_Fields(t *testing.T) {
	entry := &ingestion.LogEntry{Message: "test"}
	result := ProcessedEntry{
		Entry:      entry,
		Profile:    "test-profile",
		Actions:    []string{"template"},
		Drop:       false,
		Template:   true,
		Sample:     true,
		SampleRate: 0.5,
		Modified:   true,
		Matched:    true,
	}

	if result.Entry != entry {
		t.Error("entry mismatch")
	}
	if result.Profile != "test-profile" {
		t.Errorf("unexpected profile: %s", result.Profile)
	}
	if len(result.Actions) != 1 {
		t.Errorf("unexpected actions length: %d", len(result.Actions))
	}
	if result.Drop {
		t.Error("unexpected drop")
	}
	if !result.Template {
		t.Error("expected template true")
	}
	if !result.Sample {
		t.Error("expected sample true")
	}
	if result.SampleRate != 0.5 {
		t.Errorf("unexpected sample rate: %f", result.SampleRate)
	}
}

func TestManagerStats_Fields(t *testing.T) {
	stats := ManagerStats{
		ProfileCount: 5,
		Profiles:     []string{"a", "b", "c", "d", "e"},
	}

	if stats.ProfileCount != 5 {
		t.Errorf("unexpected count: %d", stats.ProfileCount)
	}
	if len(stats.Profiles) != 5 {
		t.Errorf("unexpected profiles length: %d", len(stats.Profiles))
	}
}

func TestManager_ProcessEntry_Drop(t *testing.T) {
	m := newTestManager()

	profile := &Profile{
		Metadata: ProfileMetadata{
			Name:    "drop-test",
			Version: "1.0.0",
		},
		Spec: ProfileSpec{
			Fingerprints: []FingerprintRule{
				{Pattern: "drop this", Action: "drop"},
			},
		},
	}
	compilePatterns(profile)
	m.AddProfile(profile)

	entry := &ingestion.LogEntry{Message: "drop this message"}

	result, err := m.ProcessWithProfile(entry, "drop-test")
	if err != nil {
		t.Fatalf("ProcessWithProfile failed: %v", err)
	}

	if !result.Drop {
		t.Error("expected drop to be true")
	}
}

func TestManager_ProcessEntry_Sampling(t *testing.T) {
	m := newTestManager()

	profile := &Profile{
		Metadata: ProfileMetadata{
			Name:    "sample-test",
			Version: "1.0.0",
		},
		Spec: ProfileSpec{
			Sampling: []SamplingRule{
				{Pattern: "sample this", Rate: 0.5},
			},
		},
	}
	compilePatterns(profile)
	m.AddProfile(profile)

	entry := &ingestion.LogEntry{Message: "sample this message"}

	result, err := m.ProcessWithProfile(entry, "sample-test")
	if err != nil {
		t.Fatalf("ProcessWithProfile failed: %v", err)
	}

	if !result.Sample {
		t.Error("expected sample to be true")
	}
	if result.SampleRate != 0.5 {
		t.Errorf("expected sample rate 0.5, got %f", result.SampleRate)
	}
}

func TestManager_ProcessEntry_Transform(t *testing.T) {
	m := newTestManager()

	profile := &Profile{
		Metadata: ProfileMetadata{
			Name:    "transform-test",
			Version: "1.0.0",
		},
		Spec: ProfileSpec{
			Transforms: []Transform{
				{Field: "message", Regex: "secret", Replace: "***"},
			},
		},
	}
	compilePatterns(profile)
	m.AddProfile(profile)

	entry := &ingestion.LogEntry{Message: "my secret data"}

	result, err := m.ProcessWithProfile(entry, "transform-test")
	if err != nil {
		t.Fatalf("ProcessWithProfile failed: %v", err)
	}

	if !result.Modified {
		t.Error("expected modified to be true")
	}
	if entry.Message != "my *** data" {
		t.Errorf("expected transformed message, got: %s", entry.Message)
	}
}

func TestManager_StrictMode(t *testing.T) {
	cfg := config.ProfilesConfig{
		TrustMode: "strict",
	}
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()
	m := NewManager(cfg, metricsRegistry, logger)

	// Add profile without signature should fail in strict mode
	profile := &Profile{
		Metadata: ProfileMetadata{
			Name:    "unsigned",
			Version: "1.0.0",
			Images:  []string{"app:*"}, // Not builtin
		},
		Spec: ProfileSpec{},
	}

	err := m.AddProfile(profile)
	if err == nil {
		t.Error("expected error in strict mode without signature")
	}
}

func TestManager_OfflineMode(t *testing.T) {
	cfg := config.ProfilesConfig{
		TrustMode: "offline",
	}
	metricsRegistry := metrics.NewRegistry()
	logger := zerolog.Nop()
	m := NewManager(cfg, metricsRegistry, logger)

	// Add profile without signature should succeed in offline mode
	profile := &Profile{
		Metadata: ProfileMetadata{
			Name:    "unsigned",
			Version: "1.0.0",
		},
		Spec: ProfileSpec{},
	}

	err := m.AddProfile(profile)
	if err != nil {
		t.Errorf("offline mode should accept unsigned profiles: %v", err)
	}
}
