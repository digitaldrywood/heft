# heft

heft is a golangci-lint plugin that keeps Go packages small enough for fast, cache-friendly tests, and tells you where to split the ones that aren't.

`go test` caches results per package, keyed on the package's files and everything it imports. When a package has 270 files and 900 tests, a one-line change re-runs all of them, plus the tests of every package that imports it. heft finds those packages, says how far over the limits they are, and proposes a split worked out from the package's own code.

```text
internal/store/abandoned_carts.go:1:1: package store has 273 non-test files, 582% over the limit of 40
(target: 40 or fewer, 233 to remove); 54027 lines of code, 440% over the limit of 10000 (target: 10000 or
fewer, 44027 to remove). Split it by domain or topic (orders, billing, users), not by technical layer
(handlers, queries, types), so each change re-runs fewer cached tests. store (261 files, 53419 lines) is
one type, Store, with methods spread over 261 files. Ignoring that shared receiver, its files fall into 32
topics: counter (50 files), order (23 files), inventory (24 files), product (24 files), 28 more. Give each
topic its own package and its own small Store holding only what that topic needs. Run heft suggest for the
file lists. (heft)
```

(golangci-lint prints this as a single line; it is wrapped here for reading.)

## Quick start

```sh
go install github.com/digitaldrywood/heft/cmd/heft@latest
heft init              # writes .golangci.yml and .custom-gcl.yml with the defaults
golangci-lint custom   # builds ./custom-gcl, golangci-lint with heft built in
./custom-gcl run ./...
```

`heft init` never overwrites an existing file unless you pass `-force`. If the repo already has a `.golangci.yml`, run `heft init -print` and merge the heft block into it.

Adding heft to a large existing repo? Record today's sizes first, so only growth fails:

```sh
heft baseline write    # writes .heft-baseline.json; commit it
./custom-gcl run ./... # passes; any package that grows past its recorded size fails
```

## What it checks

| limit key | measures | default |
|---|---|---|
| `files` | non-test `.go` files | 40 |
| `loc` | lines containing code (no blank or comment-only lines) in non-test files | 10000 |
| `test-files` | `_test.go` files | 40 |
| `test-funcs` | `Test*`, `Benchmark*`, `Fuzz*` and `Example*` functions | 200 |
| `exports` | exported top-level identifiers | off |
| `imports` | distinct imports in non-test files | off |
| `banned-names` | package names `util`, `utils`, `common`, `helper`, `helpers`, `models`, `misc` | on |

Every message gives each exceeded limit as a percentage over, the target, and how much has to go, plus where the package stands on the limits it still meets. That gives a person or an AI agent a concrete goal for the split. Each diagnostic links to a section of docs/rules.md, which explains what the limit measures, why it matters and how to fix it.

Generated files (`// Code generated ... DO NOT EDIT.`) are always skipped. External test packages (`foo_test`) are measured as their own package.

The defaults came from running heft against real repositories. In well-structured repos the largest packages stayed around 27 to 36 files and under 8,400 lines. In repos where test runs had become slow, the worst packages had 300 to 470 files and 75k to 113k lines. Set a limit to `0` to turn that check off.

## Configuration

The `heft` block of `.golangci.yml`, with every setting:

```yaml
linters:
  enable: [heft]
  settings:
    custom:
      heft:
        type: module
        settings:
          limits: {files: 40, loc: 10000, test-files: 40, test-funcs: 200, exports: 0, imports: 0}
          exclude: ["**/db/sqlc/**", "**/*_templ.go", "**/mocks/**"]
          exclude-packages: ["example.com/app/web/templates/**"]
          overrides:
            - packages: ["example.com/app/internal/legacy/**"]
              limits: {files: 120}
          banned-names: [util, utils, common, helper, helpers, models, misc]
          baseline: .heft-baseline.json
```

- `exclude` skips individual files. The globs match absolute paths, so start them with `**/`.
- `exclude-packages` skips whole packages, including their `_test` packages, everywhere heft runs. Use it for generated packages such as templ pages, or for code you've decided not to split. The globs match import paths.
- `overrides` changes limits for matching import paths. When more than one override matches, later entries win, key by key.
- `baseline` is the path of the committed baseline file, relative to where golangci-lint runs.

All globs use doublestar syntax. The defaults live in config/defaults in this repo and are embedded in the binary, and a test fails if they drift from the limits in the code. The examples directory has a config for a new repo, one for an existing repo with a baseline, and a GitHub Actions workflow.

## Commands

The `heft` command does what one package pass inside golangci-lint can't: it reads the whole repo.

`heft init [-print] [-force]` writes the default `.golangci.yml` and `.custom-gcl.yml`.

`heft check [packages]` applies the same limits and baseline as the linter and exits 1 on any violation. Run it in CI. golangci-lint's `new-from-rev` drops issues on lines that haven't changed, and heft anchors each issue on a `package` line, so growth from editing existing files would slip past it.

`heft suggest [packages]` prints the full split plan for every oversized package:

```text
example.com/app/internal/store (273 files, 54027 lines)
  store: 261 files, 53419 lines, around Store, used by 1 groups, uses 11
    methods of Store span these files; topics ignoring that receiver:
    counter (50 files, 16050 lines): cart_recovery_pricing.go checkout.go checkout_availability.go ...
    order (23 files, 6270 lines): imported_payments.go intercompany_cancellations.go ...
    inventory (24 files, 5484 lines): catalog_health.go catalog_search.go inventory.go ...
```

`heft baseline write` records every package currently over a limit. `heft baseline tighten` lowers recorded sizes after a split and drops packages that are now within the limits; it never raises anything. `heft baseline tighten -check` fails if the committed baseline is looser than the code, so CI catches a split that forgot to tighten. To raise a baseline, edit the JSON by hand so the change shows up in review.

`heft report [-test-json file] [-json] [-top n] [-fail-test-time d] [-fail-blast d]` ranks packages by what a change costs:

```sh
go test -json ./... > test.json
heft report -test-json test.json -top 5
```

```text
files  loc  test-files  test-funcs  importers  dependents  test-time      blast  package
    1   56           1           4          2          39     4.058s  1m50.413s  example.com/app/pkg/shortcode
    8  490           6          10         34          38     4.992s  1m46.355s  example.com/app/pkg/database
```

`importers` counts the packages that import this one directly, and `dependents` counts those that depend on it at any depth. `blast` is this package's test time plus that of all its dependents: roughly what a one-line change here costs in a full test run. Split the packages at the top of this list first.

Every command takes `-config` (default `.golangci.yml`) and `-C dir`. Packages default to `./...`.

## How the split plan works

heft works out the plan from the package's own syntax:

1. It joins files that share unexported identifiers, or where one holds methods on a type declared in another. Those files can't be separated without exporting something.
2. It merges groups that use each other's exported identifiers in both directions, since splitting them would create an import cycle. A group that others use, but which uses none of them, is marked "keep at the root": those are your domain types.
3. When one type's methods cover most of the files (the usual `Handler`, `Store`, `Service` or `DB`), it ignores that receiver and groups the files by topic, named after the files' shared prefix.
4. When everything is still tied together, it names the unexported helpers that hold the files together, so you know what to untangle first.

docs/rules.md turns the plan into steps, based on Ben Johnson's Standard Package Layout, Bill Kennedy's package-oriented design, Dave Cheney's Practical Go and the Go blog post on package names. In short: put domain types at the root, give each dependency you wrap its own subpackage, have imports point only down or inward, and avoid imports between sibling packages.

## How it runs

heft uses golangci-lint's syntax load mode, so it needs no type information, and it works out import paths from the nearest `go.mod`. Editing the heft settings or the baseline file invalidates golangci-lint's cache. heft combines all of a package's violations into one diagnostic per anchor line, because golangci-lint keeps only one issue per line.

## Development

```sh
go test ./...
golangci-lint run ./...
```

The analyzer tests use `analysistest`, with fixture packages in analyzer/testdata. docs/design.md records the design decisions and the calibration data.
