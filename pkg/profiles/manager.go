package profiles

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

	httpClient *http.Client
	cachePath  string
}

func NewManager(cfg config.ProfilesConfig, metrics *metrics.Registry, logger zerolog.Logger) *Manager {
	m := &Manager{
		config:    cfg,
		logger:    logger.With().Str("component", "profiles").Logger(),
		profiles:  make(map[string]*Profile),
		detector:  NewDetector(logger),
		metrics:   metrics,
		trustMode: cfg.TrustMode,
		cachePath: cfg.CachePath,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	for _, pk := range cfg.PublicKeys {
		if b, err := base64.StdEncoding.DecodeString(pk); err == nil {
			m.pubkeys = append(m.pubkeys, b)
			continue
		}
		if hb, err := hex.DecodeString(pk); err == nil {
			m.pubkeys = append(m.pubkeys, hb)
		}
	}

	if m.cachePath != "" {
		if err := os.MkdirAll(m.cachePath, 0755); err != nil {
			m.logger.Warn().Err(err).Str("path", m.cachePath).Msg("Failed to create cache directory")
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

	if err := m.loadCachedProfiles(); err != nil {
		m.logger.Warn().Err(err).Msg("Failed to load cached profiles")
	}

	if m.trustMode != "offline" && m.config.HubURL != "" {
		if err := m.syncFromHub(); err != nil {
			m.logger.Warn().Err(err).Msg("Failed to sync from hub, using cached/local profiles")
		}
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
						Pattern:  `\d+\.\d+\.\d+\.\d+ - - \[.*?\] ".*?" \d+ \d+`,
						Action:   "template",
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

	return m.loadProfilesFromDir(m.config.LocalPath, "local")
}

func (m *Manager) loadCachedProfiles() error {
	if m.cachePath == "" {
		return nil
	}

	return m.loadProfilesFromDir(m.cachePath, "cache")
}

func (m *Manager) loadProfilesFromDir(dir, source string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	loaded := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".yaml") && !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}

		path := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			m.logger.Warn().Err(err).Str("file", path).Msg("Failed to read profile file")
			continue
		}

		prof, err := ParseProfile(b)
		if err != nil {
			m.logger.Warn().Err(err).Str("file", path).Msg("Failed to parse profile")
			continue
		}

		if err := m.verifyProfile(prof); err != nil {
			if m.trustMode == "strict" {
				m.logger.Warn().Str("file", path).Msg("Rejected unsigned/invalid profile in strict mode")
				continue
			}
			m.logger.Warn().Str("file", path).Msg("Profile failed verification; accepting due to relaxed/offline mode")
		}

		m.profiles[prof.Metadata.Name] = prof
		loaded++
	}

	if loaded > 0 {
		m.logger.Debug().Int("count", loaded).Str("source", source).Str("dir", dir).Msg("Loaded profiles from directory")
	}

	return nil
}

func (m *Manager) syncFromHub() error {
	if m.config.HubURL == "" {
		return nil
	}

	indexURL := strings.TrimSuffix(m.config.HubURL, "/") + "/profiles/index.json"

	resp, err := m.httpClient.Get(indexURL)
	if err != nil {
		return fmt.Errorf("failed to fetch profile index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hub returned status %d", resp.StatusCode)
	}

	m.logger.Debug().Str("url", indexURL).Msg("Synced profile index from hub")
	return nil
}

func (m *Manager) DownloadProfile(name string) (*Profile, error) {
	if m.config.HubURL == "" {
		return nil, fmt.Errorf("hub URL not configured")
	}

	if cached, err := m.loadCachedProfile(name); err == nil && cached != nil {
		m.logger.Debug().Str("profile", name).Msg("Using cached profile")
		return cached, nil
	}

	profileURL := strings.TrimSuffix(m.config.HubURL, "/") + "/profiles/" + name + ".yaml"

	resp, err := m.httpClient.Get(profileURL)
	if err != nil {
		if cached, cacheErr := m.loadCachedProfile(name); cacheErr == nil && cached != nil {
			m.logger.Warn().Err(err).Str("profile", name).Msg("Hub unreachable, using cached profile")
			return cached, nil
		}
		return nil, fmt.Errorf("failed to download profile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if cached, cacheErr := m.loadCachedProfile(name); cacheErr == nil && cached != nil {
			m.logger.Warn().Int("status", resp.StatusCode).Str("profile", name).Msg("Hub returned error, using cached profile")
			return cached, nil
		}
		return nil, fmt.Errorf("hub returned status %d for profile %s", resp.StatusCode, name)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read profile data: %w", err)
	}

	profile, err := ParseProfile(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse profile: %w", err)
	}

	if err := m.verifyProfile(profile); err != nil {
		if m.trustMode == "strict" {
			return nil, fmt.Errorf("profile verification failed: %w", err)
		}
		m.logger.Warn().Str("profile", name).Msg("Downloaded profile failed verification; accepting due to relaxed mode")
	}

	if err := m.cacheProfile(name, data); err != nil {
		m.logger.Warn().Err(err).Str("profile", name).Msg("Failed to cache profile")
	}

	return profile, nil
}

func (m *Manager) loadCachedProfile(name string) (*Profile, error) {
	if m.cachePath == "" {
		return nil, fmt.Errorf("cache path not configured")
	}

	cachePath := filepath.Join(m.cachePath, name+".yaml")
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}

	return ParseProfile(data)
}

func (m *Manager) cacheProfile(name string, data []byte) error {
	if m.cachePath == "" {
		return nil
	}

	if err := os.MkdirAll(m.cachePath, 0755); err != nil {
		return err
	}

	cachePath := filepath.Join(m.cachePath, name+".yaml")
	return os.WriteFile(cachePath, data, 0644)
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
		Entry:   entry,
		Profile: profile.Metadata.Name,
		Actions: []string{},
	}

	matched := false
	for _, rule := range profile.Spec.Fingerprints {
		if ruleMatched, err := rule.Matches(entry.Message); err != nil {
			m.logger.Error().Err(err).Str("pattern", rule.Pattern).Msg("Pattern match error")
			continue
		} else if ruleMatched {
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

func (m *Manager) verifyProfile(profile *Profile) error {
	if m.trustMode == "offline" {
		return nil
	}

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

func (m *Manager) ClearCache() error {
	if m.cachePath == "" {
		return nil
	}

	entries, err := os.ReadDir(m.cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml") {
			path := filepath.Join(m.cachePath, e.Name())
			if err := os.Remove(path); err != nil {
				m.logger.Warn().Err(err).Str("file", path).Msg("Failed to remove cached profile")
			}
		}
	}

	m.logger.Info().Str("path", m.cachePath).Msg("Cleared profile cache")
	return nil
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
