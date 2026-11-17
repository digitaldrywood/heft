package repo

import (
	"strings"
	"testing"
	"time"

	"github.com/digitaldrywood/heft/config"
)

func TestApplyTestJSON(t *testing.T) {
	core := &Package{Path: "app/core", dependents: []string{"app/web", "app/api"}}
	web := &Package{Path: "app/web"}
	api := &Package{Path: "app/api"}
	events := `{"Action":"pass","Package":"app/core","Test":"TestX","Elapsed":9}
{"Action":"pass","Package":"app/core","Elapsed":2}
{"Action":"fail","Package":"app/web","Elapsed":3.5}
not json
{"Action":"skip","Package":"app/api","Elapsed":0.5}
`
	if err := ApplyTestJSON([]*Package{core, web, api}, strings.NewReader(events)); err != nil {
		t.Fatal(err)
	}
	if core.TestTime != 2*time.Second {
		t.Errorf("core test time = %s", core.TestTime)
	}
	if core.Blast != 6*time.Second {
		t.Errorf("core blast = %s, want 6s", core.Blast)
	}
	if web.Blast != 3500*time.Millisecond {
		t.Errorf("web blast = %s", web.Blast)
	}
}

func TestLoadSelf(t *testing.T) {
	cfg, err := config.New(config.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	pkgs, err := Load("..", []string{"./..."}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]*Package{}
	for _, p := range pkgs {
		byPath[p.Path] = p
	}
	root, xtest := byPath["github.com/digitaldrywood/heft"], byPath["github.com/digitaldrywood/heft_test"]
	if root == nil || root.Metrics["files"] == 0 || root.Dependents == 0 {
		t.Fatalf("root package = %+v", root)
	}
	if xtest == nil || xtest.Metrics["test-funcs"] == 0 {
		t.Fatalf("external test package = %+v", xtest)
	}
}
