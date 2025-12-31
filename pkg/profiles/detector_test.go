package profiles

import (
	"testing"

	"github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/ingestion"
)

func TestNewDetector(t *testing.T) {
	logger := zerolog.Nop()
	d := NewDetector(logger)
	if d == nil {
		t.Fatal("expected non-nil detector")
	}

	rules := d.GetRules()
	if len(rules) == 0 {
		t.Error("expected default rules")
	}
}

func TestDetector_Detect_Nginx(t *testing.T) {
	logger := zerolog.Nop()
	d := NewDetector(logger)

	entry := &ingestion.LogEntry{
		ContainerName: "nginx-proxy",
		Message:       `192.168.1.1 - - [01/Jan/2024:12:00:00 +0000] "GET /api/users HTTP/1.1" 200 1234`,
	}

	profile := d.Detect(entry)
	if profile != "nginx" {
		t.Errorf("expected nginx profile, got %s", profile)
	}
}

func TestDetector_Detect_NginxByImage(t *testing.T) {
	logger := zerolog.Nop()
	d := NewDetector(logger)

	entry := &ingestion.LogEntry{
		ContainerName: "my-app",
		Labels:        map[string]string{"image": "nginx:1.21"},
		Message:       "some log message",
	}

	profile := d.Detect(entry)
	if profile != "nginx" {
		t.Errorf("expected nginx profile by image, got %s", profile)
	}
}

func TestDetector_Detect_Postgres(t *testing.T) {
	logger := zerolog.Nop()
	d := NewDetector(logger)

	entry := &ingestion.LogEntry{
		ContainerName: "postgres-db",
		Message:       "LOG: database system is ready to accept connections",
	}

	profile := d.Detect(entry)
	if profile != "postgres" {
		t.Errorf("expected postgres profile, got %s", profile)
	}
}

func TestDetector_Detect_JavaSpring(t *testing.T) {
	logger := zerolog.Nop()
	d := NewDetector(logger)

	entry := &ingestion.LogEntry{
		ContainerName: "my-java-app",
		Labels:        map[string]string{"image": "openjdk:17"},
		Message:       "2024-01-01 12:00:00.000  INFO org.springframework.boot.SpringApplication: Application started",
	}

	profile := d.Detect(entry)
	if profile != "java-spring" {
		t.Errorf("expected java-spring profile, got %s", profile)
	}
}

func TestDetector_Detect_Redis(t *testing.T) {
	logger := zerolog.Nop()
	d := NewDetector(logger)

	entry := &ingestion.LogEntry{
		ContainerName: "redis-cache",
		Message:       "Ready to accept connections",
	}

	profile := d.Detect(entry)
	if profile != "redis" {
		t.Errorf("expected redis profile, got %s", profile)
	}
}

func TestDetector_Detect_MySQL(t *testing.T) {
	logger := zerolog.Nop()
	d := NewDetector(logger)

	entry := &ingestion.LogEntry{
		ContainerName: "mysql-db",
		Message:       "mysqld: ready for connections",
	}

	profile := d.Detect(entry)
	if profile != "mysql" {
		t.Errorf("expected mysql profile, got %s", profile)
	}
}

func TestDetector_Detect_NoMatch(t *testing.T) {
	logger := zerolog.Nop()
	d := NewDetector(logger)

	entry := &ingestion.LogEntry{
		ContainerName: "my-custom-app",
		Message:       "this is a custom log message that doesn't match any pattern",
	}

	profile := d.Detect(entry)
	if profile != "" {
		t.Errorf("expected empty profile for no match, got %s", profile)
	}
}

func TestDetector_AddRule(t *testing.T) {
	logger := zerolog.Nop()
	d := NewDetector(logger)

	initialCount := len(d.GetRules())

	newRule := DetectionRule{
		ProfileName:   "custom",
		ImagePatterns: []string{"custom-image"},
		LogPatterns:   []string{"CUSTOM_LOG"},
		Priority:      15,
	}

	d.AddRule(newRule)

	if len(d.GetRules()) != initialCount+1 {
		t.Error("rule was not added")
	}
}

func TestDetector_CheckImagePatterns(t *testing.T) {
	logger := zerolog.Nop()
	d := NewDetector(logger)

	entry := &ingestion.LogEntry{
		ContainerName: "nginx-web",
		Labels:        map[string]string{"image": "nginx:latest"},
	}

	patterns := []string{"nginx", "nginx:"}
	score := d.checkImagePatterns(entry, patterns)
	if score == 0 {
		t.Error("expected non-zero score for matching image")
	}
}

func TestDetector_CheckImagePatterns_ContainerImage(t *testing.T) {
	logger := zerolog.Nop()
	d := NewDetector(logger)

	entry := &ingestion.LogEntry{
		ContainerName: "my-app",
		Labels:        map[string]string{"container_image": "nginx:1.21"},
	}

	patterns := []string{"nginx"}
	score := d.checkImagePatterns(entry, patterns)
	if score == 0 {
		t.Error("expected non-zero score for container_image label")
	}
}

func TestDetector_CheckImagePatterns_NoMatch(t *testing.T) {
	logger := zerolog.Nop()
	d := NewDetector(logger)

	entry := &ingestion.LogEntry{
		ContainerName: "my-custom-app",
	}

	patterns := []string{"nginx"}
	score := d.checkImagePatterns(entry, patterns)
	if score != 0 {
		t.Error("expected zero score for no match")
	}
}

func TestDetector_CheckLogPatterns(t *testing.T) {
	logger := zerolog.Nop()
	d := NewDetector(logger)

	entry := &ingestion.LogEntry{
		Message: "ERROR: Something went wrong",
	}

	patterns := []string{"ERROR", "WARNING"}
	score := d.checkLogPatterns(entry, patterns)
	if score == 0 {
		t.Error("expected non-zero score for matching log pattern")
	}
}

func TestDetector_CheckLogPatterns_Multiple(t *testing.T) {
	logger := zerolog.Nop()
	d := NewDetector(logger)

	entry := &ingestion.LogEntry{
		Message: "ERROR: Connection to database system failed",
	}

	patterns := []string{"ERROR", "database system"}
	score := d.checkLogPatterns(entry, patterns)
	if score < 4 { // 2 matches * 2 points each
		t.Errorf("expected score >= 4 for multiple matches, got %d", score)
	}
}

func TestDetector_CheckLogPatterns_NoMatch(t *testing.T) {
	logger := zerolog.Nop()
	d := NewDetector(logger)

	entry := &ingestion.LogEntry{
		Message: "Just a normal log message",
	}

	patterns := []string{"ERROR", "FATAL"}
	score := d.checkLogPatterns(entry, patterns)
	if score != 0 {
		t.Error("expected zero score for no match")
	}
}

func TestDetector_CalculateScore(t *testing.T) {
	logger := zerolog.Nop()
	d := NewDetector(logger)

	entry := &ingestion.LogEntry{
		ContainerName: "nginx-proxy",
		Message:       `192.168.1.1 - - [01/Jan/2024:12:00:00 +0000] "GET /api HTTP/1.1" 200`,
	}

	rule := DetectionRule{
		ProfileName:   "nginx",
		ImagePatterns: []string{"nginx"},
		LogPatterns:   []string{"GET", "POST"},
		Priority:      10,
	}

	score := d.calculateScore(entry, rule)
	if score == 0 {
		t.Error("expected non-zero score")
	}
}

func TestDetectionRule_Fields(t *testing.T) {
	rule := DetectionRule{
		ProfileName:   "test",
		ImagePatterns: []string{"image1", "image2"},
		LogPatterns:   []string{"pattern1", "pattern2"},
		Priority:      5,
	}

	if rule.ProfileName != "test" {
		t.Errorf("unexpected profile name: %s", rule.ProfileName)
	}
	if len(rule.ImagePatterns) != 2 {
		t.Errorf("unexpected image patterns length: %d", len(rule.ImagePatterns))
	}
	if len(rule.LogPatterns) != 2 {
		t.Errorf("unexpected log patterns length: %d", len(rule.LogPatterns))
	}
	if rule.Priority != 5 {
		t.Errorf("unexpected priority: %d", rule.Priority)
	}
}

func TestDetector_GetRules(t *testing.T) {
	logger := zerolog.Nop()
	d := NewDetector(logger)

	rules := d.GetRules()
	if len(rules) < 5 {
		t.Error("expected at least 5 default rules (nginx, postgres, java-spring, redis, mysql)")
	}

	profileNames := make(map[string]bool)
	for _, rule := range rules {
		profileNames[rule.ProfileName] = true
	}

	expectedProfiles := []string{"nginx", "postgres", "java-spring", "redis", "mysql"}
	for _, expected := range expectedProfiles {
		if !profileNames[expected] {
			t.Errorf("expected %s in default rules", expected)
		}
	}
}
