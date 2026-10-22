package store

// ModelConfig holds per-feature AI model settings.
type ModelConfig struct {
	DevcontainerGen string `json:"devcontainerGen,omitempty"` // default: opus
	Summarizer      string `json:"summarizer,omitempty"`      // default: haiku
	IdeaEnrich      string `json:"ideaEnrich,omitempty"`      // default: sonnet
	Locale          string `json:"locale,omitempty"`          // AI output language (e.g. "zh-TW", "en"); empty = auto-detect
}

// EpicGitConfig holds settings for epic git remote sync.
type EpicGitConfig struct {
	RemoteURL       string `json:"remoteUrl,omitempty"`
	SyncEnabled     bool   `json:"syncEnabled,omitempty"`
	SyncIntervalMin int    `json:"syncIntervalMin,omitempty"` // default: 5
}

// FirewallConfig holds firewall settings for DevContainers.
type FirewallConfig struct {
	Mode    string   `json:"mode"`    // "open" | "whitelist"
	Domains []string `json:"domains"` // whitelist domain list
}

// DefaultFirewallConfig returns the default firewall configuration.
func DefaultFirewallConfig() FirewallConfig {
	return FirewallConfig{
		Mode: "open",
		Domains: []string{
			"api.anthropic.com", "claude.ai", "statsig.anthropic.com", "sentry.io",
			"registry.npmjs.org", "npmjs.com",
			"github.com", "api.github.com", "raw.githubusercontent.com", "objects.githubusercontent.com",
			"proxy.golang.org", "sum.golang.org", "storage.googleapis.com",
			"pypi.org", "files.pythonhosted.org",
			"crates.io", "static.crates.io",
			"registry-1.docker.io", "auth.docker.io", "production.cloudflare.docker.com",
		},
	}
}

// RecoveryConfig holds settings for session recovery retention.
type RecoveryConfig struct {
	RetentionDays int `json:"retentionDays"` // default: 7
}

// DigestConfig holds settings for daily digest webhook delivery.
type DigestConfig struct {
	WebhookURL string `json:"webhookUrl,omitempty"` // Discord webhook URL
	Enabled    bool   `json:"enabled"`              // scheduler on/off
	SendHour   int    `json:"sendHour"`             // UTC hour to send (0-23)
	Timezone   string `json:"timezone,omitempty"`   // IANA timezone (e.g. "Asia/Taipei"); empty defaults to UTC
}

// GetModelConfig returns a copy of the current model configuration.
func (s *Store) GetModelConfig() ModelConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.modelConfig
}

// SetModelConfig updates the model configuration and saves to disk.
func (s *Store) SetModelConfig(cfg ModelConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modelConfig = cfg
	return s.Save()
}

// GetEpicGitConfig returns a copy of the epic git configuration.
func (s *Store) GetEpicGitConfig() EpicGitConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.epicGitConfig
}

// SetEpicGitConfig updates the epic git configuration and saves to disk.
func (s *Store) SetEpicGitConfig(cfg EpicGitConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.epicGitConfig = cfg
	return s.Save()
}

// GetFirewallConfig returns the firewall configuration.
// If not set, returns the default config.
func (s *Store) GetFirewallConfig() FirewallConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.firewallConfig == nil {
		return DefaultFirewallConfig()
	}
	cfg := *s.firewallConfig
	domains := make([]string, len(cfg.Domains))
	copy(domains, cfg.Domains)
	cfg.Domains = domains
	return cfg
}

// SetFirewallConfig updates the firewall configuration and saves to disk.
func (s *Store) SetFirewallConfig(cfg FirewallConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.firewallConfig = &cfg
	return s.Save()
}

// GetDigestConfig returns the digest configuration.
// Default: enabled=false, sendHour=1 (UTC 01:00 = TW 09:00).
func (s *Store) GetDigestConfig() DigestConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.digestConfig == nil {
		return DigestConfig{SendHour: 1}
	}
	return *s.digestConfig
}

// SetDigestConfig updates the digest configuration and saves to disk.
func (s *Store) SetDigestConfig(cfg DigestConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.digestConfig = &cfg
	return s.Save()
}

// GetRecoveryConfig returns the recovery configuration.
// Default: retentionDays=7.
func (s *Store) GetRecoveryConfig() RecoveryConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.recoveryConfig == nil {
		return RecoveryConfig{RetentionDays: 7}
	}
	return *s.recoveryConfig
}

// SetRecoveryConfig updates the recovery configuration and saves to disk.
func (s *Store) SetRecoveryConfig(cfg RecoveryConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recoveryConfig = &cfg
	return s.Save()
}
