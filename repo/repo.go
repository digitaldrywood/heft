package repo

import (
	"bufio"
	"encoding/json"
	"go/ast"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"

	"github.com/digitaldrywood/heft"
	"github.com/digitaldrywood/heft/config"
)

type Package struct {
	Path       string        `json:"path"`
	Name       string        `json:"name"`
	Metrics    heft.Metrics  `json:"metrics"`
	Importers  int           `json:"importers"`
	Dependents int           `json:"dependents"`
	TestTime   time.Duration `json:"test_time_ns"`
	Blast      time.Duration `json:"blast_ns"`

	dependents []string
	sources    []heft.Source
}

func (p *Package) Plan() heft.SplitPlan { return heft.PlanSplit(p.sources) }

func (p *Package) FirstFile(test bool) string {
	for _, s := range p.sources {
		if heft.IsTestFile(s.Name) == test {
			return s.Name
		}
	}
	if len(p.sources) > 0 {
		return p.sources[0].Name
	}
	return ""
}

func Load(dir string, patterns []string, cfg *config.Config) ([]*Package, error) {
	pcfg := &packages.Config{
		Dir:   dir,
		Tests: true,
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedImports,
	}
	loaded, err := packages.Load(pcfg, patterns...)
	if err != nil {
		return nil, err
	}
	byPath := map[string]*Package{}
	importers := map[string]map[string]struct{}{}
	for _, p := range loaded {
		if strings.HasSuffix(p.ID, ".test") {
			continue
		}
		if len(p.Syntax) == 0 || cfg.PackageExcluded(p.PkgPath) {
			continue
		}
		testVariant := strings.Contains(p.ID, " [")
		if !testVariant {
			for imp := range p.Imports {
				if importers[imp] == nil {
					importers[imp] = map[string]struct{}{}
				}
				importers[imp][p.PkgPath] = struct{}{}
			}
		}
		existing, seen := byPath[p.PkgPath]
		if seen && !testVariant {
			continue
		}
		src := sources(p, cfg)
		pkg := &Package{Path: p.PkgPath, Name: p.Name, Metrics: heft.Measure(src), sources: src}
		if seen {
			existing.Metrics, existing.sources = pkg.Metrics, pkg.sources
			continue
		}
		byPath[p.PkgPath] = pkg
	}
	for path, pkg := range byPath {
		for imp := range importers[path] {
			if _, ok := byPath[imp]; ok {
				pkg.Importers++
			}
		}
	}
	for path, pkg := range byPath {
		pkg.dependents = transitive(path, importers, byPath)
		pkg.Dependents = len(pkg.dependents)
	}
	out := make([]*Package, 0, len(byPath))
	for _, p := range byPath {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b *Package) int { return strings.Compare(a.Path, b.Path) })
	return out, nil
}

func sources(p *packages.Package, cfg *config.Config) []heft.Source {
	var out []heft.Source
	for i, f := range p.Syntax {
		if i >= len(p.CompiledGoFiles) {
			break
		}
		name := p.CompiledGoFiles[i]
		if !strings.HasSuffix(name, ".go") || ast.IsGenerated(f) || cfg.Excluded(name) {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		out = append(out, heft.Source{Name: name, File: f, Src: src})
	}
	slices.SortFunc(out, func(a, b heft.Source) int { return strings.Compare(a.Name, b.Name) })
	return out
}

func transitive(root string, importers map[string]map[string]struct{}, known map[string]*Package) []string {
	seen := map[string]struct{}{root: {}}
	queue := []string{root}
	var out []string
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for imp := range importers[cur] {
			if _, ok := seen[imp]; ok {
				continue
			}
			seen[imp] = struct{}{}
			if _, ok := known[imp]; ok {
				out = append(out, imp)
			}
			queue = append(queue, imp)
		}
	}
	return out
}

type testEvent struct {
	Action  string
	Package string
	Test    string
	Elapsed float64
}

func ApplyTestJSON(pkgs []*Package, r io.Reader) error {
	times := map[string]time.Duration{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		var e testEvent
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.Test != "" {
			continue
		}
		switch e.Action {
		case "pass", "fail", "skip":
			times[e.Package] = time.Duration(e.Elapsed * float64(time.Second))
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	for _, p := range pkgs {
		p.TestTime = times[p.Path]
	}
	for _, p := range pkgs {
		p.Blast = p.TestTime
		for _, d := range p.dependents {
			p.Blast += times[d]
		}
	}
	return nil
}
