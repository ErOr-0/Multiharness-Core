package config

import (
	"strings"
	"testing"

	decisionadapter "multiharness-core/internal/adapter/decision/openrouter"
)

// Switching decision.provider switches the model and endpoint defaults, while
// explicit values still win. Configurations from before Laya support keep Jev.
func TestDecisionProviderDefaults(t *testing.T) {
	d := Defaults().Decision
	if d.Provider != "jev" || d.Model != "" || d.Endpoint != "" {
		t.Fatalf("defaults must leave provider-specific fields blank: %+v", d)
	}
	jev := d.Effective()
	if jev.Model != "typesafe/jev-1.13" || jev.Endpoint != "https://openrouter.ai/api/alpha/decisions" || d.ProviderName() != "Jev" || d.KeyVariable() != "OPENROUTER_API_KEY" || !d.RequiresKey() {
		t.Fatalf("jev defaults: %+v", jev)
	}
	legacy := Decision{Enabled: true, Model: "typesafe/jev-1.13", Endpoint: "https://openrouter.ai/api/alpha/decisions", Timeout: d.Timeout, ConfidenceThreshold: d.ConfidenceThreshold}
	if legacy.Effective().Provider != "jev" || legacy.validate() != nil {
		t.Fatal("blank provider must remain jev")
	}
	// A file saved by an earlier release spells out Jev's defaults. Switching
	// the provider must swap them for Laya's defaults, not reject the switch.
	legacy.Provider = "laya"
	switched := legacy.Effective()
	if err := legacy.validate(); err != nil || switched.Model != "laya" || switched.Endpoint != "http://127.0.0.1:8765/v1/systemone" {
		t.Fatalf("jev defaults were not swapped: %+v %v", switched, err)
	}
	// And back again, including a config that saved Laya's defaults.
	back := Decision{Enabled: true, Provider: "jev", Model: "laya", Endpoint: "http://127.0.0.1:8765/v1/systemone", Timeout: d.Timeout, ConfidenceThreshold: d.ConfidenceThreshold}
	if e := back.Effective(); back.validate() != nil || e.Model != "typesafe/jev-1.13" || e.Endpoint != "https://openrouter.ai/api/alpha/decisions" {
		t.Fatalf("laya defaults were not swapped: %+v", e)
	}
	d.Provider = decisionadapter.ProviderLaya
	laya := d.Effective()
	if laya.Model != "laya" || laya.Endpoint != "http://127.0.0.1:8765/v1/systemone" || d.ProviderName() != "Laya" || d.KeyVariable() != "LAYA_API_KEY" || d.RequiresKey() {
		t.Fatalf("laya defaults: %+v", laya)
	}
	d.Model, d.Endpoint = "laya-multilingual", "http://laya.lan:8080/v1/systemone"
	if e := d.Effective(); e.Model != "laya-multilingual" || e.Endpoint != "http://laya.lan:8080/v1/systemone" {
		t.Fatalf("explicit values overridden: %+v", e)
	}
	adapter := d.Adapter("k")
	if adapter.Provider != "laya" || adapter.Model != "laya-multilingual" || adapter.APIKey != "k" {
		t.Fatalf("adapter config: %+v", adapter)
	}
}

func TestDecisionValidationRejectsJevSettingsOnLaya(t *testing.T) {
	base := Defaults().Decision
	base.Enabled = true
	base.Provider = "laya"
	if err := base.validate(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Decision)
		want   string
	}{
		{"jev model", func(d *Decision) { d.Model = "typesafe/jev-2.0" }, "hosted Jev model"},
		{"openrouter endpoint", func(d *Decision) { d.Endpoint = "https://openrouter.ai/api/v1/decisions" }, "not a self-hosted Laya server"},
		{"malformed endpoint", func(d *Decision) { d.Endpoint = "://nope" }, "not a self-hosted Laya server"},
		{"unknown provider", func(d *Decision) { d.Provider = "other" }, "jev or laya"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := base
			tc.mutate(&d)
			if err := d.validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatal(err)
			}
			d.Enabled = false
			if err := d.validate(); err != nil {
				t.Fatal("disabled router must not validate provider settings", err)
			}
		})
	}
	// Jev keeps accepting custom endpoints; only Laya has the OpenRouter guard.
	jev := Defaults().Decision
	jev.Enabled = true
	jev.Endpoint = "https://custom.invalid/decisions"
	if err := jev.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestDecisionProviderOptionApplies(t *testing.T) {
	cfg := Defaults()
	cfg.Mode = "team"
	cfg.Decision.Enabled = true
	var option Option
	for _, candidate := range Options() {
		if candidate.Name == "decision-provider" {
			option = candidate
		}
	}
	if option.Path != "decision.provider" {
		t.Fatal("decision-provider option missing")
	}
	if err := apply(&cfg, option, "laya"); err != nil || cfg.Decision.Provider != "laya" {
		t.Fatal(err, cfg.Decision)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := apply(&cfg, option, "other"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("unknown provider accepted")
	}
}
