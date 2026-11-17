package gomod

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"

	"golang.org/x/mod/modfile"
)

var modules sync.Map

type module struct {
	dir  string
	path string
}

func ImportPath(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	m, err := find(dir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(m.dir, dir)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return m.path, nil
	}
	return path.Join(m.path, filepath.ToSlash(rel)), nil
}

func find(dir string) (module, error) {
	if m, ok := modules.Load(dir); ok {
		return m.(module), nil
	}
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err == nil {
		m := module{dir: dir, path: modfile.ModulePath(data)}
		if m.path == "" {
			return module{}, fmt.Errorf("%s/go.mod has no module line", dir)
		}
		modules.Store(dir, m)
		return m, nil
	}
	parent := filepath.Dir(dir)
	if parent == dir {
		return module{}, fmt.Errorf("no go.mod above %s", dir)
	}
	m, err := find(parent)
	if err == nil {
		modules.Store(dir, m)
	}
	return m, err
}
