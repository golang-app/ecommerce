package domain_test

import (
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/payments/domain"
)

func TestProviderConstants(t *testing.T) {
	if domain.ProviderStripe != "stripe" {
		t.Errorf("ProviderStripe = %q, want %q", domain.ProviderStripe, "stripe")
	}
	if domain.ProviderFake != "fake" {
		t.Errorf("ProviderFake = %q, want %q", domain.ProviderFake, "fake")
	}
}

func TestProviderConfig(t *testing.T) {
	now := time.Now().UTC()
	initialConfig := map[string]string{
		"fail_card_ending_in": "0000",
		"publishable_key":     "pk_test_123",
	}

	cfg := domain.NewProviderConfig("stripe", true, initialConfig, now)

	if cfg.ID() != "stripe" {
		t.Errorf("ID() = %q, want stripe", cfg.ID())
	}
	if !cfg.IsEnabled() {
		t.Error("IsEnabled() = false, want true")
	}
	if cfg.ConfigValue("fail_card_ending_in") != "0000" {
		t.Errorf("ConfigValue(fail_card_ending_in) = %q, want 0000", cfg.ConfigValue("fail_card_ending_in"))
	}
	if cfg.ConfigValue("publishable_key") != "pk_test_123" {
		t.Errorf("ConfigValue(publishable_key) = %q, want pk_test_123", cfg.ConfigValue("publishable_key"))
	}
	if cfg.ConfigValue("missing") != "" {
		t.Errorf("ConfigValue(missing) = %q, want empty", cfg.ConfigValue("missing"))
	}
	if !cfg.UpdatedAt().Equal(now) {
		t.Errorf("UpdatedAt() = %v, want %v", cfg.UpdatedAt(), now)
	}

	// Verify defensive copy on NewProviderConfig
	initialConfig["fail_card_ending_in"] = "9999"
	if cfg.ConfigValue("fail_card_ending_in") != "0000" {
		t.Errorf("ConfigValue changed when original map was modified; got %q, want 0000", cfg.ConfigValue("fail_card_ending_in"))
	}

	// Verify defensive copy on Config()
	exportedConfig := cfg.Config()
	exportedConfig["fail_card_ending_in"] = "8888"
	if cfg.ConfigValue("fail_card_ending_in") != "0000" {
		t.Errorf("ConfigValue changed when Config() return map was modified; got %q, want 0000", cfg.ConfigValue("fail_card_ending_in"))
	}

	// WithEnabled
	disabled := cfg.WithEnabled(false)
	if disabled.IsEnabled() {
		t.Error("WithEnabled(false) IsEnabled() = true, want false")
	}
	if !cfg.IsEnabled() {
		t.Error("original cfg IsEnabled() was mutated")
	}

	// WithConfig
	later := now.Add(time.Minute)
	newMap := map[string]string{"foo": "bar"}
	updated := disabled.WithConfig(newMap, later)
	if updated.IsEnabled() {
		t.Error("updated IsEnabled() = true, want false")
	}
	if updated.ConfigValue("foo") != "bar" {
		t.Errorf("ConfigValue(foo) = %q, want bar", updated.ConfigValue("foo"))
	}
	if !updated.UpdatedAt().Equal(later) {
		t.Errorf("UpdatedAt() = %v, want %v", updated.UpdatedAt(), later)
	}
	// Verify defensive copy in WithConfig
	newMap["foo"] = "baz"
	if updated.ConfigValue("foo") != "bar" {
		t.Errorf("ConfigValue changed when WithConfig input map was modified; got %q, want bar", updated.ConfigValue("foo"))
	}
}
