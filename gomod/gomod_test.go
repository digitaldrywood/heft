package gomod_test

import (
	"testing"

	"github.com/digitaldrywood/heft/gomod"
)

func TestImportPath(t *testing.T) {
	cases := map[string]string{
		"..":          "github.com/digitaldrywood/heft",
		".":           "github.com/digitaldrywood/heft/gomod",
		"../analyzer": "github.com/digitaldrywood/heft/analyzer",
	}
	for dir, want := range cases {
		got, err := gomod.ImportPath(dir)
		if err != nil || got != want {
			t.Errorf("ImportPath(%q) = %q, %v; want %q", dir, got, err, want)
		}
	}
}
