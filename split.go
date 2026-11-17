package heft

import (
	"cmp"
	"fmt"
	"go/ast"
	"path/filepath"
	"slices"
	"strings"
)

type Group struct {
	Name      string
	Anchor    string
	Files     []string
	LOC       int
	UsedBy    int
	DependsOn int
	Hub       string
	Topics    []Group
	Coupling  []string
	idxs      []int
}

type SplitPlan struct {
	Groups   []Group
	Small    int
	Coupling []string
}

const (
	hubMinFiles = 10
	hubShare    = 0.5
	smallShare  = 0.01
)

func small(g Group, total int) bool {
	return len(g.Files) == 1 && float64(g.LOC) < smallShare*float64(total)
}

type splitter struct {
	files   []Source
	loc     []int
	owner   map[string]int
	methods map[string]int
	recv    []map[string]bool
	refs    [][]string
}

func PlanSplit(sources []Source) SplitPlan {
	var files []Source
	for _, s := range sources {
		if !IsTestFile(s.Name) {
			files = append(files, s)
		}
	}
	if len(files) == 0 {
		return SplitPlan{}
	}
	sp := newSplitter(files)
	all := make([]int, len(files))
	for i := range all {
		all[i] = i
	}
	groups, shared := sp.cluster(all, "")
	total := 0
	for _, g := range groups {
		total += g.LOC
	}

	plan := SplitPlan{}
	for _, g := range groups {
		if len(groups) > 1 && g.UsedBy == 0 && small(g, total) {
			plan.Small++
			continue
		}
		if hub := sp.hubType(g); hub != "" {
			topics, topicShared := sp.cluster(g.idxs, hub)
			g.Hub = hub
			if len(topics) > 1 {
				g.Topics = mergeSmall(topics)
				if big := topics[0]; 2*len(big.Files) > len(g.Files) {
					g.Coupling = topShared(only(topicShared, big.idxs), 3)
				}
			} else {
				shared = topicShared
			}
		}
		plan.Groups = append(plan.Groups, g)
	}
	if len(plan.Groups) == 1 && len(plan.Groups[0].Topics) == 0 {
		plan.Coupling = topShared(shared, 5)
	}
	return plan
}

func only(shared map[string]map[int]struct{}, idxs []int) map[string]map[int]struct{} {
	keep := map[int]bool{}
	for _, i := range idxs {
		keep[i] = true
	}
	out := map[string]map[int]struct{}{}
	for name, users := range shared {
		for i := range users {
			if keep[i] {
				if out[name] == nil {
					out[name] = map[int]struct{}{}
				}
				out[name][i] = struct{}{}
			}
		}
	}
	return out
}

func files(n int) string {
	if n == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", n)
}

func newSplitter(files []Source) *splitter {
	sp := &splitter{
		files:   files,
		loc:     make([]int, len(files)),
		owner:   map[string]int{},
		methods: map[string]int{},
		recv:    make([]map[string]bool, len(files)),
		refs:    make([][]string, len(files)),
	}
	for i, s := range files {
		sp.loc[i] = CodeLines(s.Src)
		for _, name := range declaredNames(s.File) {
			sp.owner[name] = i
		}
		sp.recv[i] = map[string]bool{}
		for _, d := range s.File.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv != nil {
				if t := receiverType(fn); t != "" {
					sp.methods[t]++
					sp.recv[i][t] = true
				}
			}
		}
		forEachRef(s.File, func(name string) { sp.refs[i] = append(sp.refs[i], name) })
	}
	return sp
}

func (sp *splitter) cluster(idxs []int, hub string) ([]Group, map[string]map[int]struct{}) {
	pos := map[int]int{}
	for k, i := range idxs {
		pos[i] = k
	}
	uf := newUnionFind(len(idxs))
	exported := map[[2]int]struct{}{}
	shared := map[string]map[int]struct{}{}
	for k, i := range idxs {
		for t := range sp.recv[i] {
			if t == hub {
				continue
			}
			if j, ok := pos[sp.owner[t]]; ok {
				uf.union(k, j)
			}
		}
		for _, name := range sp.refs[i] {
			o, ok := sp.owner[name]
			if !ok || o == i || name == hub {
				continue
			}
			j, ok := pos[o]
			if !ok {
				continue
			}
			if ast.IsExported(name) {
				exported[[2]int{k, j}] = struct{}{}
				continue
			}
			uf.union(k, j)
			if shared[name] == nil {
				shared[name] = map[int]struct{}{}
			}
			shared[name][i] = struct{}{}
		}
	}

	comp := map[int]int{}
	var count int
	for k := range idxs {
		r := uf.find(k)
		if _, ok := comp[r]; !ok {
			comp[r] = count
			count++
		}
	}
	edges := make([]map[int]struct{}, count)
	for c := range edges {
		edges[c] = map[int]struct{}{}
	}
	for e := range exported {
		a, b := comp[uf.find(e[0])], comp[uf.find(e[1])]
		if a != b {
			edges[a][b] = struct{}{}
		}
	}
	scc := stronglyConnected(edges)

	byID := map[int]*Group{}
	for k, i := range idxs {
		id := scc[comp[uf.find(k)]]
		if byID[id] == nil {
			byID[id] = &Group{}
		}
		g := byID[id]
		g.Files = append(g.Files, filepath.Base(sp.files[i].Name))
		g.LOC += sp.loc[i]
		g.idxs = append(g.idxs, i)
	}
	links := map[[2]int]struct{}{}
	for a, out := range edges {
		for b := range out {
			if scc[a] != scc[b] {
				links[[2]int{scc[a], scc[b]}] = struct{}{}
			}
		}
	}
	for l := range links {
		byID[l[0]].DependsOn++
		byID[l[1]].UsedBy++
	}

	groups := make([]Group, 0, len(byID))
	for _, g := range byID {
		slices.Sort(g.Files)
		if hub == "" {
			g.Anchor = sp.anchorType(g.idxs)
		}
		g.Name = suggestName(g)
		groups = append(groups, *g)
	}
	slices.SortFunc(groups, func(a, b Group) int {
		return cmp.Or(cmp.Compare(b.LOC, a.LOC), strings.Compare(a.Name, b.Name))
	})
	return groups, shared
}

func (sp *splitter) hubType(g Group) string {
	if len(g.idxs) < hubMinFiles {
		return ""
	}
	counts := map[string]int{}
	for _, i := range g.idxs {
		for t := range sp.recv[i] {
			counts[t]++
		}
	}
	best, n := "", 0
	for t, c := range counts {
		if c > n || (c == n && t < best) {
			best, n = t, c
		}
	}
	if float64(n) < hubShare*float64(len(g.idxs)) {
		return ""
	}
	return best
}

func (sp *splitter) anchorType(idxs []int) string {
	best, bestScore := "", -1
	for _, i := range idxs {
		for _, d := range sp.files[i].File.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range g.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || !ts.Name.IsExported() {
					continue
				}
				if score := sp.methods[ts.Name.Name]; score > bestScore {
					best, bestScore = ts.Name.Name, score
				}
			}
		}
	}
	return best
}

func mergeSmall(topics []Group) []Group {
	total := 0
	byName := map[string]*Group{}
	var order []string
	for _, t := range topics {
		total += t.LOC
		name := commonStem(t.Files)
		g, ok := byName[name]
		if !ok {
			g = &Group{Name: name}
			byName[name] = g
			order = append(order, name)
		}
		g.Files = append(g.Files, t.Files...)
		g.LOC += t.LOC
	}
	var out []Group
	rest := Group{Name: "small files"}
	for _, name := range order {
		g := byName[name]
		slices.Sort(g.Files)
		if small(*g, total) {
			rest.Files = append(rest.Files, g.Files...)
			rest.LOC += g.LOC
			continue
		}
		out = append(out, *g)
	}
	slices.SortFunc(out, func(a, b Group) int {
		return cmp.Or(cmp.Compare(b.LOC, a.LOC), strings.Compare(a.Name, b.Name))
	})
	if len(rest.Files) > 0 {
		slices.Sort(rest.Files)
		out = append(out, rest)
	}
	return out
}

func (p SplitPlan) Summary(max int) string {
	switch {
	case len(p.Groups) == 0:
		return ""
	case len(p.Groups) == 1 && len(p.Groups[0].Topics) == 0:
		if len(p.Coupling) == 0 {
			return ""
		}
		return fmt.Sprintf("No clean split yet: every file is tied together through unexported identifiers, most widely shared: %s. Export them or move them behind a small type first, then rerun.", strings.Join(p.Coupling, ", "))
	}
	var parts []string
	for i, g := range p.Groups {
		if i == max {
			parts = append(parts, fmt.Sprintf("%d more", len(p.Groups)-max))
			break
		}
		parts = append(parts, g.describe(max))
	}
	if p.Small > 0 {
		parts = append(parts, fmt.Sprintf("%s small enough to go wherever they are used", files(p.Small)))
	}
	lead := fmt.Sprintf("Found %d groups of files that share no unexported code and can move apart without an import cycle: ", len(p.Groups))
	if len(p.Groups) == 1 {
		lead = ""
	}
	return lead + strings.Join(parts, "; ") + ". Run heft suggest for the file lists."
}

func (g Group) describe(max int) string {
	desc := fmt.Sprintf("%s (%s, %d lines", g.Name, files(len(g.Files)), g.LOC)
	if g.Anchor != "" && g.Hub == "" {
		desc += ", around " + g.Anchor
	}
	if g.UsedBy > 0 && g.DependsOn == 0 {
		desc += ", used by others: keep at the root"
	}
	desc += ")"
	if len(g.Topics) == 0 {
		return desc
	}
	var topics []string
	for i, t := range g.Topics {
		if i == max {
			topics = append(topics, fmt.Sprintf("%d more", len(g.Topics)-max))
			break
		}
		topics = append(topics, fmt.Sprintf("%s (%s)", t.Name, files(len(t.Files))))
	}
	out := fmt.Sprintf("%s is one type, %s, with methods spread over %d files. Ignoring that shared receiver, its files fall into %d topics: %s. Give each topic its own package and its own small %s holding only what that topic needs",
		desc, g.Hub, len(g.Files), len(g.Topics), strings.Join(topics, ", "), g.Hub)
	if len(g.Coupling) > 0 {
		out += fmt.Sprintf(". %s is still held together by %s; export or move those to split it further", g.Topics[0].Name, strings.Join(g.Coupling, ", "))
	}
	return out
}

func declaredNames(f *ast.File) []string {
	var out []string
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil && d.Name.Name != "init" && d.Name.Name != "_" {
				out = append(out, d.Name.Name)
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					out = append(out, s.Name.Name)
				case *ast.ValueSpec:
					for _, n := range s.Names {
						if n.Name != "_" {
							out = append(out, n.Name)
						}
					}
				}
			}
		}
	}
	return out
}

func forEachRef(f *ast.File, visit func(string)) {
	var walk func(ast.Node) bool
	walk = func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			ast.Inspect(n.X, walk)
			return false
		case *ast.Ident:
			visit(n.Name)
		}
		return true
	}
	for _, d := range f.Decls {
		ast.Inspect(d, walk)
	}
}

func receiverType(fn *ast.FuncDecl) string {
	if len(fn.Recv.List) == 0 {
		return ""
	}
	t := fn.Recv.List[0].Type
	for {
		switch x := t.(type) {
		case *ast.StarExpr:
			t = x.X
		case *ast.IndexExpr:
			t = x.X
		case *ast.IndexListExpr:
			t = x.X
		case *ast.Ident:
			return x.Name
		default:
			return ""
		}
	}
}

func suggestName(g *Group) string {
	if g.Anchor != "" {
		return strings.ToLower(g.Anchor)
	}
	return commonStem(g.Files)
}

func commonStem(files []string) string {
	counts := map[string]int{}
	for _, f := range files {
		counts[topicWord(f)]++
	}
	best, n := "", 0
	for s, c := range counts {
		if c > n || (c == n && s < best) {
			best, n = s, c
		}
	}
	return best
}

var verbs = map[string]bool{
	"list": true, "get": true, "set": true, "create": true, "update": true, "delete": true,
	"new": true, "add": true, "remove": true, "find": true, "load": true, "save": true,
	"parse": true, "handle": true, "admin": true, "api": true,
}

func topicWord(file string) string {
	words := strings.FieldsFunc(strings.ToLower(strings.TrimSuffix(file, ".go")), func(r rune) bool { return r == '_' || r == '.' })
	for _, w := range words {
		if !verbs[w] {
			return w
		}
	}
	if len(words) > 0 {
		return words[0]
	}
	return file
}

func topShared(shared map[string]map[int]struct{}, n int) []string {
	names := make([]string, 0, len(shared))
	for name := range shared {
		names = append(names, name)
	}
	slices.SortFunc(names, func(a, b string) int {
		return cmp.Or(cmp.Compare(len(shared[b]), len(shared[a])), strings.Compare(a, b))
	})
	if len(names) > n {
		names = names[:n]
	}
	for i, name := range names {
		names[i] = fmt.Sprintf("%s (%s)", name, files(len(shared[name])))
	}
	return names
}

type unionFind []int

func newUnionFind(n int) unionFind {
	u := make(unionFind, n)
	for i := range u {
		u[i] = i
	}
	return u
}

func (u unionFind) find(i int) int {
	for u[i] != i {
		u[i] = u[u[i]]
		i = u[i]
	}
	return i
}

func (u unionFind) union(a, b int) { u[u.find(a)] = u.find(b) }

func stronglyConnected(edges []map[int]struct{}) []int {
	n := len(edges)
	index, low, comp := make([]int, n), make([]int, n), make([]int, n)
	onStack := make([]bool, n)
	for i := range index {
		index[i] = -1
	}
	var stack []int
	next, count := 0, 0
	var visit func(int)
	visit = func(v int) {
		index[v], low[v] = next, next
		next++
		stack = append(stack, v)
		onStack[v] = true
		for w := range edges[v] {
			if index[w] < 0 {
				visit(w)
				low[v] = min(low[v], low[w])
			} else if onStack[w] {
				low[v] = min(low[v], index[w])
			}
		}
		if low[v] == index[v] {
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				comp[w] = count
				if w == v {
					break
				}
			}
			count++
		}
	}
	for v := range edges {
		if index[v] < 0 {
			visit(v)
		}
	}
	return comp
}
