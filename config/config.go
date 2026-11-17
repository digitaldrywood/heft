package config

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/digitaldrywood/heft"
)

var DefaultBannedNames = []string{"util", "utils", "common", "helper", "helpers", "models", "misc"}

type Settings struct {
	Limits      heft.Limits `json:"limits"`
	Exclude     []string    `json:"exclude"`
	ExcludePkgs []string    `json:"exclude-packages"`
	Overrides   []Override  `json:"overrides"`
	BannedNames *[]string   `json:"banned-names"`
	Baseline    string      `json:"baseline"`
}

type Override struct {
	Packages []string    `json:"packages"`
	Limits   heft.Limits `json:"limits"`
}

type Config struct {
	settings Settings
	limits   heft.Limits
	banned   []string
}

func New(s Settings) (*Config, error) {
	if err := validLimits(s.Limits); err != nil {
		return nil, err
	}
	for _, g := range slices.Concat(s.Exclude, s.ExcludePkgs) {
		if !doublestar.ValidatePattern(g) {
			return nil, fmt.Errorf("heft: invalid exclude glob %q", g)
		}
	}
	for _, o := range s.Overrides {
		if err := validLimits(o.Limits); err != nil {
			return nil, err
		}
		for _, g := range o.Packages {
			if !doublestar.ValidatePattern(g) {
				return nil, fmt.Errorf("heft: invalid override glob %q", g)
			}
		}
	}
	banned := DefaultBannedNames
	if s.BannedNames != nil {
		banned = *s.BannedNames
	}
	if s.Baseline == "" {
		s.Baseline = heft.DefaultBaselinePath
	}
	return &Config{settings: s, limits: heft.DefaultLimits().Merge(s.Limits), banned: banned}, nil
}

func validLimits(l heft.Limits) error {
	for k := range l {
		if !k.Valid() {
			return fmt.Errorf("heft: unknown limit %q (valid: %v)", k, heft.AllMetrics)
		}
	}
	return nil
}

func (c *Config) Settings() Settings { return c.settings }

func (c *Config) BaselinePath() string { return c.settings.Baseline }

func (c *Config) LimitsFor(pkg string) heft.Limits {
	out := c.limits
	for _, o := range c.settings.Overrides {
		if slices.ContainsFunc(o.Packages, func(g string) bool { return match(g, pkg) }) {
			out = out.Merge(o.Limits)
		}
	}
	return out
}

func (c *Config) Excluded(file string) bool {
	file = filepath.ToSlash(file)
	return slices.ContainsFunc(c.settings.Exclude, func(g string) bool { return match(g, file) })
}

func (c *Config) PackageExcluded(pkg string) bool {
	pkg = strings.TrimSuffix(pkg, "_test")
	return slices.ContainsFunc(c.settings.ExcludePkgs, func(g string) bool { return match(g, pkg) })
}

func (c *Config) Banned(name string) bool { return slices.Contains(c.banned, name) }

func match(glob, s string) bool {
	ok, _ := doublestar.Match(glob, s)
	return ok
}
