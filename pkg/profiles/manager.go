package profiles

import (
    "fmt"
    "sync"
    "crypto/ed25519"
    "encoding/base64"
    "encoding/hex"
    "path/filepath"
    "os"
    "strings"

    "github.com/rs/zerolog"

	"github.com/logsieve/logsieve/pkg/config"
    "github.com/logsieve/logsieve/pkg/ingestion"
    "github.com/logsieve/logsieve/pkg/metrics"
)

type Manager struct {
    config    config.ProfilesConfig
    logger    zerolog.Logger
    profiles  map[string]*Profile
    detector  *Detector
    metrics   *metrics.Registry
    mu        sync.RWMutex
    trustMode string
    pubkeys   [][]byte
}

func NewManager(cfg config.ProfilesConfig, metrics *metrics.Registry, logger zerolog.Logger) *Manager {
    m := &Manager{
        config:   cfg,
        logger:   logger.With().Str("component", "profiles").Logger(),
        profiles: make(map[string]*Profile),
        detector: NewDetector(logger),
        metrics:  metrics,
        trustMode: cfg.TrustMode,
    }
    for _, pk := range cfg.PublicKeys {
        // keys may be base64 or hex; try base64 first
        if b, err := base64.StdEncoding.DecodeString(pk); err == nil {
            m.pubkeys = append(m.pubkeys, b)
            continue
        }
        if hb, err := hex.DecodeString(pk); err == nil {
            m.pubkeys = append(m.pubkeys, hb)
        }
    }
    return m
}

func (m *Manager) LoadProfiles() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.loadBuiltinProfiles(); err != nil {
		m.logger.Error().Err(err).Msg("Failed to load builtin profiles")
	}

	if err := m.loadLocalProfiles(); err != nil {
		m.logger.Error().Err(err).Msg("Failed to load local profiles")
	}

	m.logger.Info().Int("count", len(m.profiles)).Msg("Loaded profiles")
	return nil
}

func (m *Manager) loadBuiltinProfiles() error {
	builtinProfiles := map[string]*Profile{
		"generic": {
			Metadata: ProfileMetadata{
				Name:        "generic",
				Version:     "1.0.0",
				Description: "Generic fallback profile",
				Tags:        []string{"generic", "fallback"},
				Images:      []string{"*"},
			},
			Spec: ProfileSpec{
				Fingerprints: []FingerprintRule{
					{
						Pattern: ".*",
						Action:  "template",
					},
				},
			},
		},
		"nginx": {
			Metadata: ProfileMetadata{
				Name:        "nginx",
				Version:     "1.0.0",
				Description: "Nginx web server logs",
				Tags:        []string{"nginx", "web", "http"},
				Images:      []string{"nginx:*", "nginx"},
			},
			Spec: ProfileSpec{
				Fingerprints: []FingerprintRule{
					{
						Pattern: `\d+\.\d+\.\d+\.\d+ - - \[.*?\] ".*?" \d+ \d+`,
						Action:  "template",
						Preserve: []string{"method", "status", "size"},
					},
					{
						Pattern: "access log",
						Action:  "drop",
					},
				},
				Sampling: []SamplingRule{
					{
						Pattern: "GET /health",
						Rate:    0.01,
					},
				},
			},
		},
	}

	for name, profile := range builtinProfiles {
		m.profiles[name] = profile
	}

	return nil
}

func (m *Manager) loadLocalProfiles() error {
    if m.config.LocalPath == "" {
        return nil
    }
    entries, err := os.ReadDir(m.config.LocalPath)
    if err != nil {
        return nil
    }
    for _, e := range entries {
        if e.IsDir() { continue }
        if !strings.HasSuffix(e.Name(), ".yaml") && !strings.HasSuffix(e.Name(), ".yml") { continue }
        path := filepath.Join(m.config.LocalPath, e.Name())
        b, err := os.ReadFile(path)
        if err != nil { continue }
        prof, err := ParseProfile(b)
        if err != nil { continue }
        if err := m.verifyProfile(prof); err != nil {
            if m.trustMode == "strict" {
                m.logger.Warn().Str("file", path).Msg("Rejected unsigned/invalid profile in strict mode")
                continue
            }
            m.logger.Warn().Str("file", path).Msg("Profile failed verification; accepting due to relaxed mode")
        }
        _ = m.AddProfile(prof)
    }
    return nil
}

func (m *Manager) GetProfile(name string) (*Profile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if profile, exists := m.profiles[name]; exists {
		return profile, nil
	}

	return nil, fmt.Errorf("profile not found: %s", name)
}

func (m *Manager) DetectProfile(entry *ingestion.LogEntry) string {
	if !m.config.AutoDetect {
		return m.config.DefaultProfile
	}

	if entry.Labels != nil {
		if profile, ok := entry.Labels["profile"]; ok && profile != "auto" {
			return profile
		}
	}

	detectedProfile := m.detector.Detect(entry)
	if detectedProfile != "" {
		return detectedProfile
	}

	return m.config.DefaultProfile
}

func (m *Manager) ProcessWithProfile(entry *ingestion.LogEntry, profileName string) (*ProcessedEntry, error) {
	profile, err := m.GetProfile(profileName)
	if err != nil {
		m.logger.Warn().Str("profile", profileName).Msg("Profile not found, using generic")
		profile, _ = m.GetProfile("generic")
	}

	return m.processEntry(entry, profile)
}

func (m *Manager) processEntry(entry *ingestion.LogEntry, profile *Profile) (*ProcessedEntry, error) {
	result := &ProcessedEntry{
		Entry:    entry,
		Profile:  profile.Metadata.Name,
		Actions:  []string{},
		Modified: false,
	}

    matched := false
    for _, rule := range profile.Spec.Fingerprints {
        if matched, err := rule.Matches(entry.Message); err != nil {
            m.logger.Error().Err(err).Str("pattern", rule.Pattern).Msg("Pattern match error")
            continue
        } else if matched {
            matched = true
            result.Actions = append(result.Actions, rule.Action)
            
            switch rule.Action {
            case "drop":
                result.Drop = true
                return result, nil
            case "template":
                result.Template = true
            }
            
            break
        }
    }
    result.Matched = matched
    if !matched && m.metrics != nil {
        m.metrics.ProfileUnknownPatternsTotal.WithLabelValues(profile.Metadata.Name).Inc()
    }

	for _, rule := range profile.Spec.Sampling {
		if matched, err := rule.Matches(entry.Message); err != nil {
			m.logger.Error().Err(err).Str("pattern", rule.Pattern).Msg("Sampling pattern match error")
			continue
		} else if matched {
			result.Sample = true
			result.SampleRate = rule.Rate
			break
		}
	}

	for _, transform := range profile.Spec.Transforms {
		if err := transform.Apply(entry); err != nil {
			m.logger.Error().Err(err).Msg("Transform error")
		} else {
			result.Modified = true
		}
	}

    return result, nil
}

func (m *Manager) ListProfiles() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.profiles))
	for name := range m.profiles {
		names = append(names, name)
	}

	return names
}

func (m *Manager) AddProfile(profile *Profile) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    if err := m.validateProfile(profile); err != nil {
        return fmt.Errorf("profile validation failed: %w", err)
    }

    if err := m.verifyProfile(profile); err != nil {
        if m.trustMode == "strict" {
            return fmt.Errorf("profile signature invalid (strict mode): %w", err)
        }
        m.logger.Warn().Str("name", profile.Metadata.Name).Msg("Profile verification failed; accepting due to relaxed/offline mode")
    }

    m.profiles[profile.Metadata.Name] = profile
    m.logger.Info().Str("name", profile.Metadata.Name).Msg("Added profile")
    
    return nil
}

func (m *Manager) RemoveProfile(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.profiles[name]; !exists {
		return fmt.Errorf("profile not found: %s", name)
	}

	delete(m.profiles, name)
	m.logger.Info().Str("name", name).Msg("Removed profile")
	
	return nil
}

func (m *Manager) validateProfile(profile *Profile) error {
	if profile.Metadata.Name == "" {
		return fmt.Errorf("profile name is required")
	}

	if profile.Metadata.Version == "" {
		return fmt.Errorf("profile version is required")
	}

	for i, rule := range profile.Spec.Fingerprints {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("fingerprint rule %d: %w", i, err)
		}
	}

	for i, rule := range profile.Spec.Sampling {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("sampling rule %d: %w", i, err)
		}
	}

	for i, transform := range profile.Spec.Transforms {
		if err := transform.Validate(); err != nil {
			return fmt.Errorf("transform %d: %w", i, err)
		}
	}

	return nil
}

// verifyProfile validates signature per configured trust mode.
// It skips verification for built-in profiles (Images contains "*").
func (m *Manager) verifyProfile(profile *Profile) error {
    if m.trustMode == "offline" {
        return nil
    }
    // Treat built-ins as trusted
    if len(profile.Metadata.Images) == 1 && profile.Metadata.Images[0] == "*" {
        return nil
    }
    if len(m.pubkeys) == 0 {
        if m.trustMode == "strict" {
            return fmt.Errorf("no public keys configured for strict mode")
        }
        return nil
    }
    sigB64 := strings.TrimSpace(profile.Metadata.Signature)
    if sigB64 == "" {
        return fmt.Errorf("missing signature")
    }
    sig, err := base64.StdEncoding.DecodeString(sigB64)
    if err != nil {
        return fmt.Errorf("invalid signature encoding: %w", err)
    }
    // Build signable bytes from core fields excluding signature
    signable, err := m.signableBytes(profile)
    if err != nil {
        return err
    }
    for _, pk := range m.pubkeys {
        if len(pk) == ed25519.PublicKeySize && ed25519.Verify(pk, signable, sig) {
            return nil
        }
    }
    return fmt.Errorf("signature verification failed with configured keys")
}

func (m *Manager) signableBytes(p *Profile) ([]byte, error) {
    // Keep it simple: concatenate fields with separators in a stable order.
    // For stronger guarantees we could use a canonical JSON/YAML encoder.
    var b strings.Builder
    b.WriteString(p.APIVersion)
    b.WriteString("\n")
    b.WriteString(p.Kind)
    b.WriteString("\n")
    b.WriteString(p.Metadata.Name)
    b.WriteString("\n")
    b.WriteString(p.Metadata.Version)
    b.WriteString("\n")
    b.WriteString(strings.Join(p.Metadata.Images, ","))
    b.WriteString("\n")
    // Include spec fingerprints and sampling regex to bind behavior
    for _, r := range p.Spec.Fingerprints {
        b.WriteString(r.Pattern)
        b.WriteString("|")
        b.WriteString(r.Action)
        b.WriteString("\n")
    }
    for _, r := range p.Spec.Sampling {
        b.WriteString(r.Pattern)
        b.WriteString("|")
        b.WriteString(fmt.Sprintf("%f", r.Rate))
        b.WriteString("\n")
    }
    return []byte(b.String()), nil
}

func (m *Manager) GetStats() ManagerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return ManagerStats{
		ProfileCount: len(m.profiles),
		Profiles:     m.ListProfiles(),
	}
}

type ProcessedEntry struct {
    Entry      *ingestion.LogEntry
    Profile    string
    Actions    []string
    Drop       bool
    Template   bool
    Sample     bool
    SampleRate float64
    Modified   bool
    Matched    bool
}

type ManagerStats struct {
	ProfileCount int      `json:"profile_count"`
	Profiles     []string `json:"profiles"`
}
