package domain

import "time"

const (
	// ProviderStripe identifies the Stripe payment provider.
	ProviderStripe = "stripe"
	// ProviderFake identifies the Fake Payment Simulator provider.
	ProviderFake = "fake"
)

// ProviderConfig represents configuration for an external payment provider
// (e.g. enabled status, credentials, and provider-specific settings).
type ProviderConfig struct {
	id        string
	enabled   bool
	config    map[string]string
	updatedAt time.Time
}

// NewProviderConfig constructs a ProviderConfig entity with a defensive copy
// of the provided configuration map.
func NewProviderConfig(id string, enabled bool, config map[string]string, updatedAt time.Time) ProviderConfig {
	cfgCopy := make(map[string]string, len(config))
	for k, v := range config {
		cfgCopy[k] = v
	}
	return ProviderConfig{
		id:        id,
		enabled:   enabled,
		config:    cfgCopy,
		updatedAt: updatedAt,
	}
}

// ID returns the unique provider identifier (e.g., "stripe", "fake").
func (p ProviderConfig) ID() string { return p.id }

// IsEnabled reports whether the provider is currently active for processing payments.
func (p ProviderConfig) IsEnabled() bool { return p.enabled }

// Config returns a defensive copy of the provider's configuration map.
func (p ProviderConfig) Config() map[string]string {
	copyMap := make(map[string]string, len(p.config))
	for k, v := range p.config {
		copyMap[k] = v
	}
	return copyMap
}

// ConfigValue returns the configuration value for the given key, or empty string if not set.
func (p ProviderConfig) ConfigValue(key string) string {
	return p.config[key]
}

// UpdatedAt returns the timestamp when this configuration was last modified.
func (p ProviderConfig) UpdatedAt() time.Time { return p.updatedAt }

// WithEnabled returns a copy of the ProviderConfig with the enabled flag updated.
func (p ProviderConfig) WithEnabled(enabled bool) ProviderConfig {
	p.enabled = enabled
	return p
}

// WithConfig returns a copy of the ProviderConfig with new configuration settings
// and an updated modification timestamp.
func (p ProviderConfig) WithConfig(cfg map[string]string, at time.Time) ProviderConfig {
	cfgCopy := make(map[string]string, len(cfg))
	for k, v := range cfg {
		cfgCopy[k] = v
	}
	p.config = cfgCopy
	p.updatedAt = at
	return p
}
