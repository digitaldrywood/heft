package config_test

import (
	"testing"

	"github.com/digitaldrywood/heft"
	"github.com/digitaldrywood/heft/config"
)

func TestLimitsFor(t *testing.T) {
	c, err := config.New(config.Settings{
		Limits: heft.Limits{heft.Files: 30},
		Overrides: []config.Override{
			{Packages: []string{"example.com/app/internal/**"}, Limits: heft.Limits{heft.Files: 60}},
			{Packages: []string{"example.com/app/internal/legacy"}, Limits: heft.Limits{heft.Files: 0}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]int{
		"example.com/app/domain":          30,
		"example.com/app/internal/db":     60,
		"example.com/app/internal/legacy": 0,
	}
	for pkg, want := range cases {
		if got := c.LimitsFor(pkg)[heft.Files]; got != want {
			t.Errorf("%s files limit = %d, want %d", pkg, got, want)
		}
	}
	if got := c.LimitsFor("x")[heft.LOC]; got != heft.DefaultLimits()[heft.LOC] {
		t.Errorf("loc default lost: %d", got)
	}
}

func TestExcludedAndBanned(t *testing.T) {
	c, err := config.New(config.Settings{Exclude: []string{"**/db/sqlc/**", "**/*_templ.go"}})
	if err != nil {
		t.Fatal(err)
	}
	if !c.Excluded("/repo/internal/db/sqlc/models.go") || !c.Excluded("/repo/web/page_templ.go") || c.Excluded("/repo/web/page.go") {
		t.Error("exclude globs mismatched")
	}
	if !c.Banned("util") || c.Banned("orders") {
		t.Error("default banned names mismatched")
	}
}

func TestInvalid(t *testing.T) {
	if _, err := config.New(config.Settings{Limits: heft.Limits{"lines": 1}}); err == nil {
		t.Error("unknown metric accepted")
	}
	if _, err := config.New(config.Settings{Exclude: []string{"[bad"}}); err == nil {
		t.Error("bad glob accepted")
	}
}

func TestPackageExcluded(t *testing.T) {
	c, err := config.New(config.Settings{ExcludePkgs: []string{"example.com/app/templates/**"}})
	if err != nil {
		t.Fatal(err)
	}
	for pkg, want := range map[string]bool{
		"example.com/app/templates/pages":      true,
		"example.com/app/templates/pages_test": true,
		"example.com/app/internal/store":       false,
	} {
		if got := c.PackageExcluded(pkg); got != want {
			t.Errorf("PackageExcluded(%q) = %v, want %v", pkg, got, want)
		}
	}
}
