# heft design

## Goal

Keep Go packages small enough that `go test` caching works for you: a one-line change should re-run a small package and its few importers, not thousands of tests. Target: a full unit-test run of any repo in under 10 minutes.

## Shape

Two deliverables in one module, `github.com/digitaldrywood/heft`:

1. `heft` analyzer, a golangci-lint module plugin (`register.Plugin("heft", New)`), load mode `LoadModeSyntax`; golangci-lint leaves `pass.Pkg` nil in that mode, so the import path comes from the nearest go.mod (`gomod` package). Measures what one package pass can see.
2. `cmd/heft`, a companion CLI for whole-repo facts and baseline management. It reuses the same metric code (`metrics` package) by loading packages with `golang.org/x/tools/go/packages`, so the CLI and analyzer never disagree on a number.

Layout (domain at root, one subpackage per dependency):

```
heft.go            Metrics, Limits, Violation, Baseline types (no deps)
metrics/           compute Metrics from []*ast.File
config/            settings decode, glob matching, limit resolution
baseline/          read/write/tighten .heft-baseline.json
analyzer/          analysis.Analyzer + golangci-lint register glue
repo/              go list + go test -json (fan-in, test time)
cmd/heft/          CLI wiring only
```

## Per-package metrics (analyzer)

| metric | key | default limit |
|---|---|---|
| non-test files | `files` | 40 |
| non-test lines of code (non-blank, non-comment) | `loc` | 10000 |
| test files | `test-files` | 40 |
| test functions (Test*, Benchmark*, Fuzz*, Example*) | `test-funcs` | 200 |
| exported top-level identifiers | `exports` | off |
| distinct imports (non-test files) | `imports` | off |
| banned package names: util, utils, common, helpers, helper, models, misc, shared | `names` | on |

Defaults come from the calibration runs below.

Test packages: golangci-lint (tests: true) gives the analyzer the in-package test variant (`foo` + `foo_test.go` files) and the external `foo_test` package as separate passes. heft reports test metrics on the in-package variant and attributes the `foo_test` package to `foo`'s import path with its own counts, so `test-funcs` applies to in-package and external tests separately. Open question 3 covers whether to sum them instead. Non-test metrics are only reported from one variant to avoid duplicate diagnostics.

Generated files (`// Code generated ... DO NOT EDIT.`) are skipped automatically, plus anything matching `exclude` globs.

## Configuration (.golangci.yml)

```yaml
linters:
  enable: [heft]
  settings:
    custom:
      heft:
        type: module
        settings:
          limits: {files: 40, loc: 10000, test-funcs: 200}
          exclude: ["**/db/sqlc/**", "**/*_templ.go", "**/mocks/**"]
          overrides:
            - packages: ["*/internal/legacy/**"]
              limits: {files: 120}
          banned-names: [util, common, helpers, models]
          baseline: .heft-baseline.json
```

Globs match the import path (packages) or the file path relative to the module root (exclude), doublestar syntax. Last matching override wins per key. A limit of 0 disables that metric.

## Ratchet

`.heft-baseline.json`, committed, keyed by import path:

```json
{"version":1,"packages":{"example.com/app/internal/database":{"files":304,"loc":61000,"test-funcs":2210}}}
```

Rule: for a package in the baseline, a metric fails only if it exceeds max(limit, baseline). Packages not in the baseline get plain limits, so new giant packages are blocked from day one.

- `heft baseline write ./...` records every package currently over a limit.
- `heft baseline tighten ./...` lowers recorded values to current sizes, drops packages now under limits, never raises anything. Run after a split; CI can run `heft baseline tighten --check` to fail when the committed baseline is looser than reality, so shrinkage gets locked in.
- Raising a baseline is a deliberate hand edit, visible in review.

## Diagnostics

One diagnostic per exceeded metric, e.g.:

```
heft: package database has 312 non-test files (limit 50, baseline 304, +8). Split by domain (database/billing, database/users), not by layer (database/queries, database/types).
heft: package name "util" says nothing about what it provides; move each function next to the domain type it serves.
```

Position: package clause of the first file in the package.

## new-from-rev

Diagnostics anchor on the package clause of the first file (first test file for test metrics). Under new-from-rev, growth by editing existing files is usually filtered out; the ratchet is the incremental mechanism, so CI should run heft without new-from-rev.

## Companion CLI (repo-wide)

- `heft report ./...` per package: metrics, reverse-import fan-in (from `go list -deps -json`, counted within the main module), and transitive dependents.
- `heft report --test-json out.json` ingests `go test -json` output and adds per-package elapsed time.
- Ranks packages by "blast radius" = test time of the package plus test time of all transitive dependents. That number is what a one-line change costs, and it is the best single sort key for what to split first.
- `--fail-test-time 60s`, `--fail-blast 5m` thresholds for CI. Text and JSON output.

## Design principles behind the suggestions

Standard Package Layout (Johnson), package-oriented design (Kennedy), Practical Go (Cheney), Go blog "Package names": domain types at the root, one subpackage per wrapped dependency, imports point down or inward, no sibling-to-sibling imports, names describe what a package provides. heft v1 only enforces size and names; import-direction rules are left to depguard/go-arch-lint, and the diagnostics link a short doc page explaining the split strategy.

## Prior work

- spaghetti-cutter `size`: standalone tool, per-package size, no ratchet, not a golangci-lint linter.
- depguard, go-arch-lint: import rules, not size.
- heft differs by living inside golangci-lint, ratcheting, and tying size to test-cache cost.

## Calibration (2026-11-16, read-only `heft report`)

| repo | pkgs | files p95 / max | loc p95 / max | test-funcs p95 / max |
|---|---|---|---|---|
| gopherguides/corp (healthy) | 101 | 8 / 27 | 2579 / 8369 | 27 / 58 |
| gopherguides/learn (healthy) | 14 | 36 / 36 | 2845 / 2845 | 19 / 19 |
| detent | 485 | 21 / 449 | 4656 / 96067 | 69 / 1401 |
| pyroapex | 128 | 38 / 302 | 5862 / 75156 | 194 / 2001 |
| parable apps/api | 609 | 26 / 469 | 6361 / 113325 | 128 / 2337 |

Healthy repos stay under 40 files, 10k LOC and 200 test functions per package; the problem repos' giants are 7 to 12 times over. `exports` and `imports` tripped healthy packages (icon sets, admin wiring), so they are measured but off by default.

## Caching

golangci-lint keys its issue cache on analyzer names plus package contents. Editing the baseline or the heft settings would otherwise serve stale results, so the plugin also registers a no-op analyzer named `heft_<hash of settings and baseline>`.

## Testing

`analysistest` with testdata packages per metric, override, exclude, banned name, baseline pass/fail; unit tests for glob resolution and baseline tighten; a golden test for CLI report output.
