package heft

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
)

const DefaultBaselinePath = ".heft-baseline.json"

type Baseline struct {
	Version  int                `json:"version"`
	Packages map[string]Metrics `json:"packages"`
}

func NewBaseline() *Baseline { return &Baseline{Version: 1, Packages: map[string]Metrics{}} }

func ReadBaseline(path string) (*Baseline, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return NewBaseline(), nil
	}
	if err != nil {
		return nil, err
	}
	b := NewBaseline()
	if err := json.Unmarshal(data, b); err != nil {
		return nil, fmt.Errorf("parse baseline %s: %w", path, err)
	}
	if b.Packages == nil {
		b.Packages = map[string]Metrics{}
	}
	return b, nil
}

func (b *Baseline) Write(path string) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (b *Baseline) For(pkg string) Metrics {
	if b == nil {
		return nil
	}
	return b.Packages[pkg]
}

func (b *Baseline) Record(pkg string, violations []Violation) {
	if len(violations) == 0 {
		return
	}
	m := Metrics{}
	for _, v := range violations {
		m[v.Metric] = v.Value
	}
	b.Packages[pkg] = m
}

func (b *Baseline) Tighten(current map[string]Metrics, limits func(pkg string) Limits) *Baseline {
	out := NewBaseline()
	for pkg, recorded := range b.Packages {
		now, ok := current[pkg]
		if !ok {
			continue
		}
		lim := limits(pkg)
		m := Metrics{}
		for metric, old := range recorded {
			value := min(old, now[metric])
			if l := lim[metric]; l > 0 && value > l {
				m[metric] = value
			}
		}
		if len(m) > 0 {
			out.Packages[pkg] = m
		}
	}
	return out
}

func (b *Baseline) Equal(o *Baseline) bool {
	return maps.EqualFunc(b.Packages, o.Packages, func(x, y Metrics) bool { return maps.Equal(x, y) })
}
