package config_test

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/digitaldrywood/heft"
	"github.com/digitaldrywood/heft/config"
)

func TestEmbeddedDefaultsMatchCode(t *testing.T) {
	var doc struct {
		Linters struct {
			Settings struct {
				Custom map[string]struct {
					Settings any `yaml:"settings"`
				} `yaml:"custom"`
			} `yaml:"settings"`
		} `yaml:"linters"`
	}
	if err := yaml.Unmarshal(config.DefaultGolangCI, &doc); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(doc.Linters.Settings.Custom["heft"].Settings)
	if err != nil {
		t.Fatal(err)
	}
	var s config.Settings
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(s.Limits, heft.DefaultLimits()) {
		t.Errorf("embedded limits %v differ from heft.DefaultLimits %v", s.Limits, heft.DefaultLimits())
	}
	if s.BannedNames == nil || !slices.Equal(*s.BannedNames, config.DefaultBannedNames) {
		t.Errorf("embedded banned names differ from config.DefaultBannedNames")
	}
	if s.Baseline != heft.DefaultBaselinePath {
		t.Errorf("embedded baseline %q", s.Baseline)
	}
	if _, err := config.New(s); err != nil {
		t.Errorf("embedded settings invalid: %v", err)
	}
	var gcl map[string]any
	if err := yaml.Unmarshal(config.DefaultCustomGCL, &gcl); err != nil || gcl["plugins"] == nil {
		t.Errorf("embedded .custom-gcl.yml invalid: %v", err)
	}
}
