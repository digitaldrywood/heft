package heft

import (
	"fmt"
	"slices"
	"strings"
)

type Metric string

const (
	Files     Metric = "files"
	LOC       Metric = "loc"
	TestFiles Metric = "test-files"
	TestFuncs Metric = "test-funcs"
	Exports   Metric = "exports"
	Imports   Metric = "imports"
)

var AllMetrics = []Metric{Files, LOC, TestFiles, TestFuncs, Exports, Imports}

var metricLabels = map[Metric]string{
	Files:     "non-test files",
	LOC:       "lines of code",
	TestFiles: "test files",
	TestFuncs: "test functions",
	Exports:   "exported identifiers",
	Imports:   "imports",
}

func (m Metric) Valid() bool { return slices.Contains(AllMetrics, m) }

func (m Metric) Label() string { return metricLabels[m] }

type Metrics map[Metric]int

type Limits map[Metric]int

func DefaultLimits() Limits {
	return Limits{
		Files:     40,
		LOC:       10000,
		TestFiles: 40,
		TestFuncs: 200,
		Exports:   0,
		Imports:   0,
	}
}

func (l Limits) Merge(o Limits) Limits {
	out := Limits{}
	for k, v := range l {
		out[k] = v
	}
	for k, v := range o {
		out[k] = v
	}
	return out
}

type Violation struct {
	Metric   Metric
	Value    int
	Limit    int
	Baseline int
}

func (v Violation) Allowed() int { return max(v.Limit, v.Baseline) }

func (v Violation) PercentOver() int {
	return (v.Value - v.Limit) * 100 / v.Limit
}

func (v Violation) clause() string {
	target := fmt.Sprintf("target: %d or fewer, %d to remove", v.Limit, v.Value-v.Limit)
	if v.Baseline > 0 {
		return fmt.Sprintf("%d %s, %d over its baseline of %d and %d%% over the limit of %d (%s)",
			v.Value, v.Metric.Label(), v.Value-v.Baseline, v.Baseline, v.PercentOver(), v.Limit, target)
	}
	return fmt.Sprintf("%d %s, %d%% over the limit of %d (%s)", v.Value, v.Metric.Label(), v.PercentOver(), v.Limit, target)
}

func (v Violation) Test() bool { return v.Metric.test() }

func Message(pkg string, vs []Violation, m Metrics, limits Limits) string {
	clauses := make([]string, len(vs))
	for i, v := range vs {
		clauses[i] = v.clause()
	}
	msg := fmt.Sprintf("package %s has %s.", pkg, strings.Join(clauses, "; "))
	if w := Within(m, limits, vs, vs[0].Test()); w != "" {
		msg += " " + w
	}
	return msg + " " + splitAdvice
}

func Within(m Metrics, limits Limits, over []Violation, test bool) string {
	var parts []string
	for _, metric := range AllMetrics {
		limit := limits[metric]
		if limit <= 0 || metric.test() != test || slices.ContainsFunc(over, func(v Violation) bool { return v.Metric == metric }) {
			continue
		}
		parts = append(parts, fmt.Sprintf("%d of %d %s (%d%%)", m[metric], limit, metric.Label(), m[metric]*100/limit))
	}
	if len(parts) == 0 {
		return ""
	}
	return "Within limits: " + strings.Join(parts, ", ") + "."
}

func (m Metric) test() bool { return m == TestFiles || m == TestFuncs }

func Partition(vs []Violation) (code, test []Violation) {
	for _, v := range vs {
		if v.Test() {
			test = append(test, v)
		} else {
			code = append(code, v)
		}
	}
	return code, test
}

const splitAdvice = "Split it by domain or topic (orders, billing, users), not by technical layer (handlers, queries, types), so each change re-runs fewer cached tests."

func Check(m Metrics, limits Limits, baseline Metrics) []Violation {
	var out []Violation
	for _, metric := range AllMetrics {
		limit := limits[metric]
		if limit <= 0 {
			continue
		}
		v := Violation{Metric: metric, Value: m[metric], Limit: limit, Baseline: baseline[metric]}
		if v.Value > v.Allowed() {
			out = append(out, v)
		}
	}
	return out
}

func NameMessage(name string) string {
	return fmt.Sprintf("package name %q says nothing about what the package provides. Move each piece next to the domain type it serves, or name the package after what it does.", name)
}
