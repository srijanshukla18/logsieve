package profiles

import (
	"testing"
	"time"

	"github.com/logsieve/logsieve/pkg/ingestion"
)

func TestParseProfile(t *testing.T) {
	yaml := `
apiVersion: logsieve.io/v1
kind: Profile
metadata:
  name: test-profile
  version: "1.0.0"
  description: Test profile
  tags:
    - test
  images:
    - test-image:*
spec:
  fingerprints:
    - pattern: "test pattern .*"
      action: template
  contextTriggers:
    - pattern: "ERROR.*"
      before: 5
      after: 3
  sampling:
    - pattern: "debug.*"
      rate: 0.1
  transforms:
    - field: message
      regex: "secret=\\w+"
      replace: "secret=REDACTED"
  routing:
    rules:
      - name: errors-to-loki
        pattern: "ERROR"
        output: loki
`

	profile, err := ParseProfile([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseProfile failed: %v", err)
	}

	if profile.APIVersion != "logsieve.io/v1" {
		t.Errorf("unexpected api version: %s", profile.APIVersion)
	}
	if profile.Kind != "Profile" {
		t.Errorf("unexpected kind: %s", profile.Kind)
	}
	if profile.Metadata.Name != "test-profile" {
		t.Errorf("unexpected name: %s", profile.Metadata.Name)
	}
	if profile.Metadata.Version != "1.0.0" {
		t.Errorf("unexpected version: %s", profile.Metadata.Version)
	}
	if len(profile.Spec.Fingerprints) != 1 {
		t.Errorf("expected 1 fingerprint rule, got %d", len(profile.Spec.Fingerprints))
	}
	if len(profile.Spec.ContextTriggers) != 1 {
		t.Errorf("expected 1 context trigger, got %d", len(profile.Spec.ContextTriggers))
	}
	if len(profile.Spec.Sampling) != 1 {
		t.Errorf("expected 1 sampling rule, got %d", len(profile.Spec.Sampling))
	}
	if len(profile.Spec.Transforms) != 1 {
		t.Errorf("expected 1 transform, got %d", len(profile.Spec.Transforms))
	}
	if profile.Spec.Routing == nil {
		t.Error("expected routing config")
	}
}

func TestParseProfile_InvalidYAML(t *testing.T) {
	yaml := `
invalid: yaml: content:
`
	_, err := ParseProfile([]byte(yaml))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestParseProfile_InvalidRegex(t *testing.T) {
	yaml := `
apiVersion: logsieve.io/v1
kind: Profile
metadata:
  name: invalid-regex
  version: "1.0.0"
spec:
  fingerprints:
    - pattern: "[invalid regex"
      action: template
`
	_, err := ParseProfile([]byte(yaml))
	if err == nil {
		t.Error("expected error for invalid regex pattern")
	}
}

func TestFingerprintRule_Matches(t *testing.T) {
	rule := FingerprintRule{
		Pattern: "error.*occurred",
		Action:  "template",
	}

	matched, err := rule.Matches("error: something occurred")
	if err != nil {
		t.Fatalf("Matches failed: %v", err)
	}
	if !matched {
		t.Error("expected match")
	}

	matched, err = rule.Matches("info: all good")
	if err != nil {
		t.Fatalf("Matches failed: %v", err)
	}
	if matched {
		t.Error("expected no match")
	}
}

func TestFingerprintRule_Validate(t *testing.T) {
	tests := []struct {
		name    string
		rule    FingerprintRule
		wantErr bool
	}{
		{
			name:    "valid rule",
			rule:    FingerprintRule{Pattern: ".*", Action: "template"},
			wantErr: false,
		},
		{
			name:    "empty pattern",
			rule:    FingerprintRule{Pattern: "", Action: "template"},
			wantErr: true,
		},
		{
			name:    "invalid action",
			rule:    FingerprintRule{Pattern: ".*", Action: "invalid"},
			wantErr: true,
		},
		{
			name:    "invalid pattern",
			rule:    FingerprintRule{Pattern: "[invalid", Action: "template"},
			wantErr: true,
		},
		{
			name:    "drop action",
			rule:    FingerprintRule{Pattern: ".*", Action: "drop"},
			wantErr: false,
		},
		{
			name:    "keep action",
			rule:    FingerprintRule{Pattern: ".*", Action: "keep"},
			wantErr: false,
		},
		{
			name:    "empty action defaults to template",
			rule:    FingerprintRule{Pattern: ".*", Action: ""},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rule.Validate()
			if tc.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestContextTrigger_Matches(t *testing.T) {
	trigger := ContextTrigger{
		Pattern: "ERROR|FATAL",
		Before:  5,
		After:   3,
	}

	matched, err := trigger.Matches("ERROR: something failed")
	if err != nil {
		t.Fatalf("Matches failed: %v", err)
	}
	if !matched {
		t.Error("expected match")
	}
}

func TestSamplingRule_Matches(t *testing.T) {
	rule := SamplingRule{
		Pattern: "health.*check",
		Rate:    0.1,
	}

	matched, err := rule.Matches("health check passed")
	if err != nil {
		t.Fatalf("Matches failed: %v", err)
	}
	if !matched {
		t.Error("expected match")
	}
}

func TestSamplingRule_Validate(t *testing.T) {
	tests := []struct {
		name    string
		rule    SamplingRule
		wantErr bool
	}{
		{
			name:    "valid rule",
			rule:    SamplingRule{Pattern: ".*", Rate: 0.5},
			wantErr: false,
		},
		{
			name:    "empty pattern",
			rule:    SamplingRule{Pattern: "", Rate: 0.5},
			wantErr: true,
		},
		{
			name:    "rate below 0",
			rule:    SamplingRule{Pattern: ".*", Rate: -0.1},
			wantErr: true,
		},
		{
			name:    "rate above 1",
			rule:    SamplingRule{Pattern: ".*", Rate: 1.5},
			wantErr: true,
		},
		{
			name:    "rate at 0",
			rule:    SamplingRule{Pattern: ".*", Rate: 0},
			wantErr: false,
		},
		{
			name:    "rate at 1",
			rule:    SamplingRule{Pattern: ".*", Rate: 1.0},
			wantErr: false,
		},
		{
			name:    "invalid pattern",
			rule:    SamplingRule{Pattern: "[invalid", Rate: 0.5},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rule.Validate()
			if tc.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestTransform_Apply(t *testing.T) {
	transform := Transform{
		Field:   "message",
		Regex:   "password=\\w+",
		Replace: "password=REDACTED",
	}

	entry := &ingestion.LogEntry{
		Message: "user login with password=secret123",
	}

	err := transform.Apply(entry)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	expected := "user login with password=REDACTED"
	if entry.Message != expected {
		t.Errorf("expected '%s', got '%s'", expected, entry.Message)
	}
}

func TestTransform_Apply_UnsupportedField(t *testing.T) {
	transform := Transform{
		Field:   "level",
		Regex:   ".*",
		Replace: "INFO",
	}

	entry := &ingestion.LogEntry{
		Message: "test",
		Level:   "ERROR",
	}

	err := transform.Apply(entry)
	if err == nil {
		t.Error("expected error for unsupported field")
	}
}

func TestTransform_Validate(t *testing.T) {
	tests := []struct {
		name    string
		trans   Transform
		wantErr bool
	}{
		{
			name:    "valid transform",
			trans:   Transform{Field: "message", Regex: ".*", Replace: "new"},
			wantErr: false,
		},
		{
			name:    "empty field",
			trans:   Transform{Field: "", Regex: ".*", Replace: "new"},
			wantErr: true,
		},
		{
			name:    "empty regex",
			trans:   Transform{Field: "message", Regex: "", Replace: "new"},
			wantErr: true,
		},
		{
			name:    "unsupported field",
			trans:   Transform{Field: "level", Regex: ".*", Replace: "new"},
			wantErr: true,
		},
		{
			name:    "invalid regex",
			trans:   Transform{Field: "message", Regex: "[invalid", Replace: "new"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.trans.Validate()
			if tc.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestRoutingRule_Matches(t *testing.T) {
	rule := RoutingRule{
		Name:    "errors",
		Pattern: "ERROR|FATAL",
		Output:  "loki",
	}

	matched, err := rule.Matches("ERROR: something happened")
	if err != nil {
		t.Fatalf("Matches failed: %v", err)
	}
	if !matched {
		t.Error("expected match")
	}
}

func TestProfileMetadata_Fields(t *testing.T) {
	meta := ProfileMetadata{
		Name:        "test",
		Version:     "1.0.0",
		Author:      "author",
		Description: "description",
		Tags:        []string{"tag1", "tag2"},
		Images:      []string{"image1", "image2"},
		Signature:   "sig",
	}

	if meta.Name != "test" {
		t.Errorf("unexpected name: %s", meta.Name)
	}
	if meta.Version != "1.0.0" {
		t.Errorf("unexpected version: %s", meta.Version)
	}
	if len(meta.Tags) != 2 {
		t.Errorf("unexpected tags length: %d", len(meta.Tags))
	}
}

func TestProfile_Structure(t *testing.T) {
	profile := Profile{
		APIVersion: "v1",
		Kind:       "Profile",
		Metadata: ProfileMetadata{
			Name:    "test",
			Version: "1.0.0",
		},
		Spec: ProfileSpec{
			Fingerprints:    []FingerprintRule{},
			ContextTriggers: []ContextTrigger{},
			Sampling:        []SamplingRule{},
			Transforms:      []Transform{},
			Routing:         &RoutingConfig{},
		},
	}

	if profile.APIVersion != "v1" {
		t.Errorf("unexpected api version: %s", profile.APIVersion)
	}
	if profile.Kind != "Profile" {
		t.Errorf("unexpected kind: %s", profile.Kind)
	}
}

func TestCompilePatterns_Fingerprints(t *testing.T) {
	profile := &Profile{
		Spec: ProfileSpec{
			Fingerprints: []FingerprintRule{
				{Pattern: "test.*", Action: "template"},
			},
		},
	}

	err := compilePatterns(profile)
	if err != nil {
		t.Fatalf("compilePatterns failed: %v", err)
	}

	if profile.Spec.Fingerprints[0].compiled == nil {
		t.Error("fingerprint pattern not compiled")
	}
}

func TestCompilePatterns_AllTypes(t *testing.T) {
	profile := &Profile{
		Spec: ProfileSpec{
			Fingerprints: []FingerprintRule{
				{Pattern: "test1.*", Action: "template"},
			},
			ContextTriggers: []ContextTrigger{
				{Pattern: "test2.*", Before: 5, After: 3},
			},
			Sampling: []SamplingRule{
				{Pattern: "test3.*", Rate: 0.1},
			},
			Transforms: []Transform{
				{Field: "message", Regex: "test4.*", Replace: "new"},
			},
			Routing: &RoutingConfig{
				Rules: []RoutingRule{
					{Name: "test", Pattern: "test5.*", Output: "out"},
				},
			},
		},
	}

	err := compilePatterns(profile)
	if err != nil {
		t.Fatalf("compilePatterns failed: %v", err)
	}
}

func TestFingerprintRule_CashedMatch(t *testing.T) {
	rule := FingerprintRule{
		Pattern: "test.*",
		Action:  "template",
	}

	// First call compiles
	rule.Matches("test message")

	// Second call uses cached regex
	matched, err := rule.Matches("test another")
	if err != nil {
		t.Fatalf("Matches failed: %v", err)
	}
	if !matched {
		t.Error("expected match")
	}
}

func TestFingerprintRule_Fields(t *testing.T) {
	rule := FingerprintRule{
		Pattern:  "test.*",
		Action:   "drop",
		Unless:   "important",
		Preserve: []string{"timestamp", "level"},
	}

	if rule.Pattern != "test.*" {
		t.Errorf("unexpected pattern: %s", rule.Pattern)
	}
	if rule.Action != "drop" {
		t.Errorf("unexpected action: %s", rule.Action)
	}
	if rule.Unless != "important" {
		t.Errorf("unexpected unless: %s", rule.Unless)
	}
	if len(rule.Preserve) != 2 {
		t.Errorf("unexpected preserve length: %d", len(rule.Preserve))
	}
}

func TestContextTrigger_Fields(t *testing.T) {
	trigger := ContextTrigger{
		Pattern: "ERROR.*",
		Before:  10,
		After:   5,
	}

	if trigger.Pattern != "ERROR.*" {
		t.Errorf("unexpected pattern: %s", trigger.Pattern)
	}
	if trigger.Before != 10 {
		t.Errorf("unexpected before: %d", trigger.Before)
	}
	if trigger.After != 5 {
		t.Errorf("unexpected after: %d", trigger.After)
	}
}

func TestRoutingConfig_Fields(t *testing.T) {
	config := RoutingConfig{
		Rules: []RoutingRule{
			{Name: "rule1", Pattern: ".*", Output: "loki"},
			{Name: "rule2", Pattern: "error", Output: "es"},
		},
	}

	if len(config.Rules) != 2 {
		t.Errorf("unexpected rules length: %d", len(config.Rules))
	}
}

func TestTransform_ApplyMultiple(t *testing.T) {
	transform := Transform{
		Field:   "message",
		Regex:   "secret",
		Replace: "***",
	}

	entry := &ingestion.LogEntry{
		Message:   "secret data and more secret stuff",
		Timestamp: time.Now(),
	}

	err := transform.Apply(entry)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	expected := "*** data and more *** stuff"
	if entry.Message != expected {
		t.Errorf("expected '%s', got '%s'", expected, entry.Message)
	}
}
