package plugin

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"

	"github.com/digitaldrywood/heft"
	"github.com/digitaldrywood/heft/analyzer"
	"github.com/digitaldrywood/heft/config"
)

func init() { register.Plugin("heft", New) }

type Plugin struct {
	cfg      *config.Config
	baseline *heft.Baseline
}

func New(raw any) (register.LinterPlugin, error) {
	s, err := register.DecodeSettings[config.Settings](raw)
	if err != nil {
		return nil, err
	}
	cfg, err := config.New(s)
	if err != nil {
		return nil, err
	}
	b, err := heft.ReadBaseline(cfg.BaselinePath())
	if err != nil {
		return nil, err
	}
	return &Plugin{cfg: cfg, baseline: b}, nil
}

func (p *Plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{analyzer.New(p.cfg, p.baseline), analyzer.CacheKey(p.cfg, p.baseline)}, nil
}

func (p *Plugin) GetLoadMode() string { return register.LoadModeSyntax }
