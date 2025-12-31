package profiles

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/logsieve/logsieve/pkg/ingestion"
	"gopkg.in/yaml.v3"
)

type Profile struct {
	APIVersion string          `yaml:"apiVersion"`
	Kind       string          `yaml:"kind"`
	Metadata   ProfileMetadata `yaml:"metadata"`
	Spec       ProfileSpec     `yaml:"spec"`
}

type ProfileMetadata struct {
    Name        string   `yaml:"name"`
    Version     string   `yaml:"version"`
    Author      string   `yaml:"author,omitempty"`
    Description string   `yaml:"description,omitempty"`
    Tags        []string `yaml:"tags,omitempty"`
    Images      []string `yaml:"images,omitempty"`
    Signature   string   `yaml:"signature,omitempty"`
}

type ProfileSpec struct {
	Fingerprints    []FingerprintRule `yaml:"fingerprints,omitempty"`
	ContextTriggers []ContextTrigger  `yaml:"contextTriggers,omitempty"`
	Sampling        []SamplingRule    `yaml:"sampling,omitempty"`
	Transforms      []Transform       `yaml:"transforms,omitempty"`
	Routing         *RoutingConfig    `yaml:"routing,omitempty"`
}

type FingerprintRule struct {
	Pattern  string   `yaml:"pattern"`
	Action   string   `yaml:"action"`   // "drop", "template", "keep"
	Unless   string   `yaml:"unless,omitempty"`
	Preserve []string `yaml:"preserve,omitempty"`
	compiled *regexp.Regexp
}

type ContextTrigger struct {
	Pattern string `yaml:"pattern"`
	Before  int    `yaml:"before"`
	After   int    `yaml:"after"`
	compiled *regexp.Regexp
}

type SamplingRule struct {
	Pattern string  `yaml:"pattern"`
	Rate    float64 `yaml:"rate"` // 0.0 to 1.0
	compiled *regexp.Regexp
}

type Transform struct {
	Field   string `yaml:"field"`
	Regex   string `yaml:"regex"`
	Replace string `yaml:"replace"`
	compiled *regexp.Regexp
}

type RoutingConfig struct {
	Rules []RoutingRule `yaml:"rules,omitempty"`
}

type RoutingRule struct {
	Name    string `yaml:"name"`
	Pattern string `yaml:"pattern"`
	Output  string `yaml:"output"`
	compiled *regexp.Regexp
}

func ParseProfile(data []byte) (*Profile, error) {
	var profile Profile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if err := compilePatterns(&profile); err != nil {
		return nil, fmt.Errorf("failed to compile patterns: %w", err)
	}

	return &profile, nil
}

func compilePatterns(profile *Profile) error {
	for i := range profile.Spec.Fingerprints {
		compiled, err := regexp.Compile(profile.Spec.Fingerprints[i].Pattern)
		if err != nil {
			return fmt.Errorf("fingerprint pattern %d: %w", i, err)
		}
		profile.Spec.Fingerprints[i].compiled = compiled
	}

	for i := range profile.Spec.ContextTriggers {
		compiled, err := regexp.Compile(profile.Spec.ContextTriggers[i].Pattern)
		if err != nil {
			return fmt.Errorf("context trigger pattern %d: %w", i, err)
		}
		profile.Spec.ContextTriggers[i].compiled = compiled
	}

	for i := range profile.Spec.Sampling {
		compiled, err := regexp.Compile(profile.Spec.Sampling[i].Pattern)
		if err != nil {
			return fmt.Errorf("sampling pattern %d: %w", i, err)
		}
		profile.Spec.Sampling[i].compiled = compiled
	}

	for i := range profile.Spec.Transforms {
		compiled, err := regexp.Compile(profile.Spec.Transforms[i].Regex)
		if err != nil {
			return fmt.Errorf("transform regex %d: %w", i, err)
		}
		profile.Spec.Transforms[i].compiled = compiled
	}

	if profile.Spec.Routing != nil {
		for i := range profile.Spec.Routing.Rules {
			compiled, err := regexp.Compile(profile.Spec.Routing.Rules[i].Pattern)
			if err != nil {
				return fmt.Errorf("routing pattern %d: %w", i, err)
			}
			profile.Spec.Routing.Rules[i].compiled = compiled
		}
	}

	return nil
}

func (r *FingerprintRule) Matches(message string) (bool, error) {
	if r.compiled == nil {
		compiled, err := regexp.Compile(r.Pattern)
		if err != nil {
			return false, err
		}
		r.compiled = compiled
	}

	return r.compiled.MatchString(message), nil
}

func (r *FingerprintRule) Validate() error {
	if r.Pattern == "" {
		return fmt.Errorf("pattern is required")
	}

	if r.Action == "" {
		r.Action = "template"
	}

	validActions := map[string]bool{
		"drop":     true,
		"template": true,
		"keep":     true,
	}

	if !validActions[r.Action] {
		return fmt.Errorf("invalid action: %s", r.Action)
	}

	_, err := regexp.Compile(r.Pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
	}

	return nil
}

func (t *ContextTrigger) Matches(message string) (bool, error) {
	if t.compiled == nil {
		compiled, err := regexp.Compile(t.Pattern)
		if err != nil {
			return false, err
		}
		t.compiled = compiled
	}

	return t.compiled.MatchString(message), nil
}

func (s *SamplingRule) Matches(message string) (bool, error) {
	if s.compiled == nil {
		compiled, err := regexp.Compile(s.Pattern)
		if err != nil {
			return false, err
		}
		s.compiled = compiled
	}

	return s.compiled.MatchString(message), nil
}

func (s *SamplingRule) Validate() error {
	if s.Pattern == "" {
		return fmt.Errorf("pattern is required")
	}

	if s.Rate < 0.0 || s.Rate > 1.0 {
		return fmt.Errorf("rate must be between 0.0 and 1.0")
	}

	_, err := regexp.Compile(s.Pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
	}

	return nil
}

func (t *Transform) Apply(entry *ingestion.LogEntry) error {
	if t.compiled == nil {
		compiled, err := regexp.Compile(t.Regex)
		if err != nil {
			return err
		}
		t.compiled = compiled
	}

	switch strings.ToLower(t.Field) {
	case "message":
		entry.Message = t.compiled.ReplaceAllString(entry.Message, t.Replace)
	default:
		return fmt.Errorf("unsupported field: %s", t.Field)
	}

	return nil
}

func (t *Transform) Validate() error {
	if t.Field == "" {
		return fmt.Errorf("field is required")
	}

	if t.Regex == "" {
		return fmt.Errorf("regex is required")
	}

	supportedFields := map[string]bool{
		"message": true,
	}

	if !supportedFields[strings.ToLower(t.Field)] {
		return fmt.Errorf("unsupported field: %s", t.Field)
	}

	_, err := regexp.Compile(t.Regex)
	if err != nil {
		return fmt.Errorf("invalid regex: %w", err)
	}

	return nil
}

func (r *RoutingRule) Matches(message string) (bool, error) {
	if r.compiled == nil {
		compiled, err := regexp.Compile(r.Pattern)
		if err != nil {
			return false, err
		}
		r.compiled = compiled
	}

	return r.compiled.MatchString(message), nil
}
