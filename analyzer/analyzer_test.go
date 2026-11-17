package analyzer_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/digitaldrywood/heft"
	"github.com/digitaldrywood/heft/analyzer"
	"github.com/digitaldrywood/heft/config"
)

func TestAnalyzer(t *testing.T) {
	cfg, err := config.New(config.Settings{
		Limits:      heft.Limits{heft.Files: 2, heft.LOC: 3, heft.TestFuncs: 2},
		Exclude:     []string{"**/mock_*.go"},
		ExcludePkgs: []string{"skipped/**", "skipped"},
		Overrides:   []config.Override{{Packages: []string{"legacy"}, Limits: heft.Limits{heft.Files: 5}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	baseline := heft.NewBaseline()
	baseline.Packages["baselined"] = heft.Metrics{heft.Files: 3, heft.LOC: 4}
	baseline.Packages["grew"] = heft.Metrics{heft.Files: 3, heft.LOC: 4}

	a := analyzer.New(cfg, baseline)
	analysistest.Run(t, analysistest.TestData(), a,
		"toomany", "small", "util", "withtests", "baselined", "grew", "legacy", "gen", "excluded", "loc", "both", "skipped")
}
