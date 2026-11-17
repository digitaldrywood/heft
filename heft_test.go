package heft_test

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/digitaldrywood/heft"
)

func TestCodeLines(t *testing.T) {
	src := "package p\n\n// c\n/* a\nb */\nvar x = `one\ntwo`\n\nfunc f() {\n}\n"
	if got := heft.CodeLines([]byte(src)); got != 4 {
		t.Fatalf("CodeLines = %d, want 4", got)
	}
}

func TestMeasure(t *testing.T) {
	fset := token.NewFileSet()
	parse := func(name, src string) heft.Source {
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		return heft.Source{Name: name, File: f, Src: []byte(src)}
	}
	m := heft.Measure([]heft.Source{
		parse("a.go", "package p\nimport (\"fmt\"; \"os\")\nconst A, b = 1, 2\ntype T struct{}\nfunc (T) M() {}\nfunc F() { fmt.Println(os.Args) }\n"),
		parse("b.go", "package p\nimport \"fmt\"\nvar _ = fmt.Sprint\n"),
		parse("a_test.go", "package p\nfunc TestX() {}\nfunc Testx() {}\nfunc Example() {}\nfunc FuzzY() {}\nfunc helper() {}\n"),
	})
	want := heft.Metrics{heft.Files: 2, heft.LOC: 9, heft.TestFiles: 1, heft.TestFuncs: 3, heft.Exports: 3, heft.Imports: 2}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %d, want %d", k, m[k], v)
		}
	}
}

func TestCheckBaseline(t *testing.T) {
	limits := heft.Limits{heft.Files: 10, heft.LOC: 0}
	m := heft.Metrics{heft.Files: 15, heft.LOC: 1_000_000}
	if v := heft.Check(m, limits, heft.Metrics{heft.Files: 15}); len(v) != 0 {
		t.Fatalf("at baseline: got %v", v)
	}
	v := heft.Check(m, limits, heft.Metrics{heft.Files: 12})
	if len(v) != 1 || v[0].Allowed() != 12 {
		t.Fatalf("over baseline: got %v", v)
	}
}

func TestTighten(t *testing.T) {
	b := heft.NewBaseline()
	b.Packages["big"] = heft.Metrics{heft.Files: 100, heft.LOC: 20000}
	b.Packages["split"] = heft.Metrics{heft.Files: 60}
	b.Packages["gone"] = heft.Metrics{heft.Files: 60}
	current := map[string]heft.Metrics{
		"big":   {heft.Files: 80, heft.LOC: 25000},
		"split": {heft.Files: 20},
	}
	got := b.Tighten(current, func(string) heft.Limits { return heft.DefaultLimits() })
	if len(got.Packages) != 1 {
		t.Fatalf("packages = %v", got.Packages)
	}
	big := got.Packages["big"]
	if big[heft.Files] != 80 || big[heft.LOC] != 20000 {
		t.Fatalf("big = %v, want files lowered to 80 and loc kept at 20000", big)
	}
}

func TestMessageTargets(t *testing.T) {
	limits := heft.Limits{heft.Files: 40, heft.LOC: 10000, heft.TestFiles: 40}
	m := heft.Metrics{heft.Files: 60, heft.LOC: 8000, heft.TestFiles: 10}
	vs := heft.Check(m, limits, nil)
	got := heft.Message("store", vs, m, limits)
	for _, want := range []string{
		"60 non-test files, 50% over the limit of 40 (target: 40 or fewer, 20 to remove).",
		"Within limits: 8000 of 10000 lines of code (80%).",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("message %q missing %q", got, want)
		}
	}
	if strings.Contains(got, "of 40 test files") {
		t.Errorf("code message should not mention test limits: %q", got)
	}
}
