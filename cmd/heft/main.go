package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/digitaldrywood/heft"
	"github.com/digitaldrywood/heft/config"
	"github.com/digitaldrywood/heft/repo"
)

const usage = `heft measures Go package size and test cost across a repository.

Usage:
  heft init     [-print] [-force]    write default .golangci.yml and .custom-gcl.yml
  heft check    [flags] [packages]   enforce limits and the baseline (exit 1 on violations); use in CI
  heft suggest  [flags] [packages]   show how each oversized package could be split
  heft report   [flags] [packages]   rank packages by size, fan-in and test blast radius
  heft baseline write   [flags] [packages]   record every oversized package
  heft baseline tighten [flags] [packages]   lower the baseline after a split (never raises)

Packages default to ./... . Settings come from the heft entry in .golangci.yml.
`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "heft:", err)
		os.Exit(1)
	}
}

var errFailed = errors.New("threshold exceeded")

func run(args []string, w io.Writer) error {
	if len(args) == 0 {
		_, err := fmt.Fprint(w, usage)
		return err
	}
	switch args[0] {
	case "report":
		return report(args[1:], w)
	case "init":
		return initConfig(args[1:], w)
	case "check":
		return check(args[1:], w)
	case "suggest":
		return suggest(args[1:], w)
	case "baseline":
		if len(args) < 2 {
			return errors.New("baseline needs write or tighten")
		}
		return baseline(args[1], args[2:], w)
	case "help", "-h", "--help":
		_, err := fmt.Fprint(w, usage)
		return err
	}
	return fmt.Errorf("unknown command %q", args[0])
}

type common struct {
	fs      *flag.FlagSet
	config  *string
	dir     *string
	cfg     *config.Config
	pkgArgs []string
}

func newCommon(name string) *common {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	return &common{
		fs:     fs,
		config: fs.String("config", ".golangci.yml", "golangci-lint config holding heft settings"),
		dir:    fs.String("C", ".", "run in this directory"),
	}
}

func (c *common) parse(args []string) error {
	if err := c.fs.Parse(args); err != nil {
		return err
	}
	c.pkgArgs = c.fs.Args()
	if len(c.pkgArgs) == 0 {
		c.pkgArgs = []string{"./..."}
	}
	if err := os.Chdir(*c.dir); err != nil {
		return err
	}
	s, err := loadSettings(*c.config)
	if err != nil {
		return err
	}
	c.cfg, err = config.New(s)
	return err
}

func loadSettings(path string) (config.Settings, error) {
	var s config.Settings
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	var doc struct {
		Linters struct {
			Settings struct {
				Custom map[string]struct {
					Settings any `yaml:"settings"`
				} `yaml:"custom"`
			} `yaml:"settings"`
		} `yaml:"linters"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return s, fmt.Errorf("parse %s: %w", path, err)
	}
	raw, err := json.Marshal(doc.Linters.Settings.Custom["heft"].Settings)
	if err != nil {
		return s, err
	}
	if string(raw) == "null" {
		return s, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return s, dec.Decode(&s)
}

func report(args []string, w io.Writer) error {
	c := newCommon("report")
	asJSON := c.fs.Bool("json", false, "print JSON")
	testJSON := c.fs.String("test-json", "", "go test -json output to read test times from")
	top := c.fs.Int("top", 25, "rows to print (0 for all)")
	failTest := c.fs.Duration("fail-test-time", 0, "fail if any package's tests take longer")
	failBlast := c.fs.Duration("fail-blast", 0, "fail if any package's blast radius is longer")
	if err := c.parse(args); err != nil {
		return err
	}
	pkgs, err := repo.Load(".", c.pkgArgs, c.cfg)
	if err != nil {
		return err
	}
	if *testJSON != "" {
		f, err := os.Open(*testJSON)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		if err := repo.ApplyTestJSON(pkgs, f); err != nil {
			return err
		}
	}
	slices.SortStableFunc(pkgs, func(a, b *repo.Package) int {
		return cmp.Or(
			cmp.Compare(b.Blast, a.Blast),
			cmp.Compare(b.Metrics[heft.LOC]*max(1, b.Dependents), a.Metrics[heft.LOC]*max(1, a.Dependents)),
		)
	})
	if *asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(pkgs); err != nil {
			return err
		}
	} else if err := printTable(w, pkgs, *top); err != nil {
		return err
	}
	for _, p := range pkgs {
		if (*failTest > 0 && p.TestTime > *failTest) || (*failBlast > 0 && p.Blast > *failBlast) {
			fmt.Fprintf(os.Stderr, "heft: %s tests %s, blast radius %s\n", p.Path, p.TestTime.Round(time.Millisecond), p.Blast.Round(time.Millisecond))
			err = errFailed
		}
	}
	return err
}

func printTable(w io.Writer, pkgs []*repo.Package, top int) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
	_, _ = fmt.Fprintln(tw, "files\tloc\ttest-files\ttest-funcs\texports\timports\timporters\tdependents\ttest-time\tblast\t package")
	for i, p := range pkgs {
		if top > 0 && i >= top {
			break
		}
		m := p.Metrics
		_, _ = fmt.Fprintf(tw, "%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%s\t%s\t %s\n",
			m[heft.Files], m[heft.LOC], m[heft.TestFiles], m[heft.TestFuncs], m[heft.Exports], m[heft.Imports],
			p.Importers, p.Dependents, p.TestTime.Round(time.Millisecond), p.Blast.Round(time.Millisecond), p.Path)
	}
	return tw.Flush()
}

func baseline(sub string, args []string, w io.Writer) error {
	c := newCommon("baseline " + sub)
	out := c.fs.String("o", "", "baseline file (default: the configured baseline path)")
	check := c.fs.Bool("check", false, "tighten only: fail instead of writing if the baseline would change")
	if err := c.parse(args); err != nil {
		return err
	}
	path := *out
	if path == "" {
		path = c.cfg.BaselinePath()
	}
	pkgs, err := repo.Load(".", c.pkgArgs, c.cfg)
	if err != nil {
		return err
	}
	switch sub {
	case "write":
		b := heft.NewBaseline()
		for _, p := range pkgs {
			b.Record(p.Path, heft.Check(p.Metrics, c.cfg.LimitsFor(p.Path), nil))
		}
		_, _ = fmt.Fprintf(w, "recorded %d oversized packages in %s\n", len(b.Packages), path)
		return b.Write(path)
	case "tighten":
		old, err := heft.ReadBaseline(path)
		if err != nil {
			return err
		}
		current := map[string]heft.Metrics{}
		for _, p := range pkgs {
			current[p.Path] = p.Metrics
		}
		next := old.Tighten(current, c.cfg.LimitsFor)
		if old.Equal(next) {
			_, err := fmt.Fprintf(w, "%s is already tight\n", path)
			return err
		}
		if *check {
			return fmt.Errorf("%s is looser than the code; run heft baseline tighten", path)
		}
		_, _ = fmt.Fprintf(w, "tightened %s: %d packages remain (was %d)\n", path, len(next.Packages), len(old.Packages))
		return next.Write(path)
	}
	return fmt.Errorf("unknown baseline command %q", sub)
}

func check(args []string, w io.Writer) error {
	c := newCommon("check")
	if err := c.parse(args); err != nil {
		return err
	}
	b, err := heft.ReadBaseline(c.cfg.BaselinePath())
	if err != nil {
		return err
	}
	pkgs, err := repo.Load(".", c.pkgArgs, c.cfg)
	if err != nil {
		return err
	}
	failed := 0
	report := func(file, msg string) {
		failed++
		_, _ = fmt.Fprintf(w, "%s:1:1: %s\n", file, msg)
	}
	for _, p := range pkgs {
		if !strings.HasSuffix(p.Name, "_test") && c.cfg.Banned(p.Name) {
			report(p.FirstFile(false), heft.NameMessage(p.Name))
		}
		limits := c.cfg.LimitsFor(p.Path)
		code, test := heft.Partition(heft.Check(p.Metrics, limits, b.For(p.Path)))
		if len(code) > 0 {
			msg := heft.Message(p.Name, code, p.Metrics, limits)
			if s := p.Plan().Summary(4); s != "" {
				msg += " " + s
			}
			report(p.FirstFile(false), msg)
		}
		if len(test) > 0 {
			report(p.FirstFile(true), heft.Message(p.Name, test, p.Metrics, limits))
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d violations: %w", failed, errFailed)
	}
	return nil
}

func suggest(args []string, w io.Writer) error {
	c := newCommon("suggest")
	all := c.fs.Bool("all", false, "include packages that are within their limits")
	if err := c.parse(args); err != nil {
		return err
	}
	pkgs, err := repo.Load(".", c.pkgArgs, c.cfg)
	if err != nil {
		return err
	}
	for _, p := range pkgs {
		limits := c.cfg.LimitsFor(p.Path)
		code, _ := heft.Partition(heft.Check(p.Metrics, limits, nil))
		if len(code) == 0 && !*all {
			continue
		}
		plan := p.Plan()
		_, _ = fmt.Fprintf(w, "%s (%d files, %d lines)\n", p.Path, p.Metrics[heft.Files], p.Metrics[heft.LOC])
		if len(plan.Groups) <= 1 && (len(plan.Groups) == 0 || len(plan.Groups[0].Topics) == 0) {
			_, _ = fmt.Fprintf(w, "  no clean split: files share unexported identifiers: %s\n\n", strings.Join(plan.Coupling, ", "))
			continue
		}
		for _, g := range plan.Groups {
			role := ""
			if g.UsedBy > 0 && g.DependsOn == 0 {
				role = ", keep at the root"
			}
			_, _ = fmt.Fprintf(w, "  %s: %d files, %d lines, around %s, used by %d groups, uses %d%s\n",
				g.Name, len(g.Files), g.LOC, cmp.Or(g.Anchor, "-"), g.UsedBy, g.DependsOn, role)
			if len(g.Topics) == 0 {
				_, _ = fmt.Fprintf(w, "    %s\n", strings.Join(g.Files, " "))
				continue
			}
			_, _ = fmt.Fprintf(w, "    methods of %s span these files; topics ignoring that receiver:\n", g.Hub)
			for _, t := range g.Topics {
				_, _ = fmt.Fprintf(w, "    %s (%d files, %d lines): %s\n", t.Name, len(t.Files), t.LOC, strings.Join(t.Files, " "))
			}
		}
		if plan.Small > 0 {
			_, _ = fmt.Fprintf(w, "  plus %d small single-file groups\n", plan.Small)
		}
		_, _ = fmt.Fprintln(w)
	}
	return nil
}

func initConfig(args []string, w io.Writer) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	print := fs.Bool("print", false, "print the default .golangci.yml instead of writing files")
	force := fs.Bool("force", false, "overwrite existing files")
	dir := fs.String("C", ".", "write into this directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *print {
		_, err := w.Write(config.DefaultGolangCI)
		return err
	}
	files := []struct {
		name string
		data []byte
	}{
		{".golangci.yml", config.DefaultGolangCI},
		{".custom-gcl.yml", config.DefaultCustomGCL},
	}
	for _, f := range files {
		path := filepath.Join(*dir, f.name)
		if _, err := os.Stat(path); err == nil && !*force {
			_, _ = fmt.Fprintf(w, "%s exists, left unchanged. Merge the heft block from `heft init -print`, or rerun with -force.\n", path)
			continue
		}
		if err := os.WriteFile(path, f.data, 0o644); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(w, "wrote %s\n", path)
	}
	return nil
}
