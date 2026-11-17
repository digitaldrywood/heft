package heft

import (
	"go/ast"
	"go/scanner"
	"go/token"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Source struct {
	Name string
	File *ast.File
	Src  []byte
}

func IsTestFile(name string) bool { return strings.HasSuffix(name, "_test.go") }

func Measure(sources []Source) Metrics {
	m := Metrics{}
	imports := map[string]struct{}{}
	for _, s := range sources {
		if IsTestFile(s.Name) {
			m[TestFiles]++
			m[TestFuncs] += countTestFuncs(s.File)
			continue
		}
		m[Files]++
		m[LOC] += CodeLines(s.Src)
		m[Exports] += countExports(s.File)
		for _, imp := range s.File.Imports {
			imports[imp.Path.Value] = struct{}{}
		}
	}
	m[Imports] = len(imports)
	return m
}

func CodeLines(src []byte) int {
	fset := token.NewFileSet()
	f := fset.AddFile("", -1, len(src))
	var s scanner.Scanner
	s.Init(f, src, nil, 0)
	count, last := 0, 0
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			return count
		}
		if tok == token.SEMICOLON && lit == "\n" {
			continue
		}
		if line := f.Line(pos); line != last {
			count++
			last = line
		}
	}
}

var testPrefixes = []string{"Test", "Benchmark", "Fuzz", "Example"}

func countTestFuncs(f *ast.File) int {
	n := 0
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv != nil {
			continue
		}
		for _, p := range testPrefixes {
			if isTestName(fn.Name.Name, p) {
				n++
				break
			}
		}
	}
	return n
}

func isTestName(name, prefix string) bool {
	rest, ok := strings.CutPrefix(name, prefix)
	if !ok {
		return false
	}
	if rest == "" {
		return true
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return !unicode.IsLower(r)
}

func countExports(f *ast.File) int {
	n := 0
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil && d.Name.IsExported() {
				n++
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Name.IsExported() {
						n++
					}
				case *ast.ValueSpec:
					for _, name := range s.Names {
						if name.IsExported() {
							n++
						}
					}
				}
			}
		}
	}
	return n
}
