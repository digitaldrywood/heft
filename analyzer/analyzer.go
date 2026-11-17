package analyzer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/digitaldrywood/heft"
	"github.com/digitaldrywood/heft/config"
	"github.com/digitaldrywood/heft/gomod"
)

const Doc = "heft reports Go packages that are too large for fast, cache-friendly tests"

const RulesURL = "https://github.com/digitaldrywood/heft/blob/main/docs/rules.md"

func New(cfg *config.Config, baseline *heft.Baseline) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "heft",
		Doc:  Doc,
		URL:  RulesURL,
		Run:  func(pass *analysis.Pass) (any, error) { return nil, run(pass, cfg, baseline) },
	}
}

func run(pass *analysis.Pass, cfg *config.Config, baseline *heft.Baseline) error {
	sources, err := collect(pass, cfg)
	if err != nil || len(sources) == 0 {
		return err
	}
	path, name, err := identity(pass, sources)
	if err != nil || cfg.PackageExcluded(path) {
		return err
	}

	if !strings.HasSuffix(name, "_test") && cfg.Banned(name) {
		pass.Report(analysis.Diagnostic{
			Pos:      anchor(sources, false),
			Category: "names",
			Message:  heft.NameMessage(name),
			URL:      RulesURL + "#names",
		})
	}
	m, limits := heft.Measure(sources), cfg.LimitsFor(path)
	code, test := heft.Partition(heft.Check(m, limits, baseline.For(path)))
	if len(code) > 0 {
		msg := heft.Message(name, code, m, limits)
		if s := heft.PlanSplit(sources).Summary(4); s != "" {
			msg += " " + s
		}
		report(pass, anchor(sources, false), code[0].Metric, msg)
	}
	if len(test) > 0 {
		report(pass, anchor(sources, true), test[0].Metric, heft.Message(name, test, m, limits))
	}
	return nil
}

func report(pass *analysis.Pass, pos token.Pos, metric heft.Metric, msg string) {
	pass.Report(analysis.Diagnostic{Pos: pos, Category: string(metric), Message: msg, URL: RulesURL + "#" + string(metric)})
}

func identity(pass *analysis.Pass, sources []heft.Source) (path, name string, err error) {
	if pass.Pkg != nil {
		return pass.Pkg.Path(), pass.Pkg.Name(), nil
	}
	name = sources[0].File.Name.Name
	path, err = gomod.ImportPath(filepath.Dir(sources[0].Name))
	if strings.HasSuffix(name, "_test") {
		path += "_test"
	}
	return path, name, err
}

func anchor(sources []heft.Source, test bool) token.Pos {
	for _, s := range sources {
		if heft.IsTestFile(s.Name) == test {
			return s.File.Package
		}
	}
	return sources[0].File.Package
}

func collect(pass *analysis.Pass, cfg *config.Config) ([]heft.Source, error) {
	var out []heft.Source
	for _, f := range pass.Files {
		name := pass.Fset.File(f.Pos()).Name()
		if !strings.HasSuffix(name, ".go") || ast.IsGenerated(f) || cfg.Excluded(name) {
			continue
		}
		read := pass.ReadFile
		if read == nil {
			read = os.ReadFile
		}
		src, err := read(name)
		if err != nil {
			return nil, err
		}
		out = append(out, heft.Source{Name: name, File: f, Src: src})
	}
	slices.SortFunc(out, func(a, b heft.Source) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func CacheKey(cfg *config.Config, baseline *heft.Baseline) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "heft_" + fingerprint(cfg, baseline),
		Doc:  "changes name when heft settings or the baseline change, so golangci-lint drops cached heft results",
		Run:  func(*analysis.Pass) (any, error) { return nil, nil },
	}
}

func fingerprint(cfg *config.Config, baseline *heft.Baseline) string {
	h := sha256.New()
	_ = json.NewEncoder(h).Encode(cfg.Settings())
	_ = json.NewEncoder(h).Encode(baseline)
	return hex.EncodeToString(h.Sum(nil))[:12]
}
