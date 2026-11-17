# heft rules

Every heft diagnostic links to a section of this page. Each section says what the rule measures, why it matters, and how to fix it.

Why size matters at all: `go test` caches results per package, keyed on the package's files and everything it imports. Change one line in a package with 300 files and Go re-runs every test in it, then every test in every package that imports it. Small packages keep that blast radius small.

## How to split a package

`heft suggest ./path/to/pkg` prints the file groups heft found. They come from three passes:

1. Files are joined when they share unexported identifiers (a helper function, a private type, a package variable), or when one holds methods on a type declared in another. Those files can't be separated without exporting something, so they move together.
2. Groups that use each other's exported identifiers in both directions are merged, since putting them in separate packages would create an import cycle. Groups that are used by others but use nothing themselves are marked "keep at the root". They are your domain types.
3. If one type's methods cover at least half of a large group (the usual `Handler`, `Store`, `Service`, `DB`), heft ignores that receiver and groups the files again, naming each topic after the files' shared name prefix, skipping verbs like list and get.

Turn each topic into its own package:

- Domain types stay at the root of the module or of the area. Subpackages import the root, never the other way around (Ben Johnson, Standard Package Layout).
- One subpackage per dependency it wraps: `postgres`, `stripe`, `http`. Inside a large area, one package per topic: `orders`, `inventory`, `counter`.
- No imports between sibling packages. If `orders` needs something from `inventory`, move the shared type up to the root or pass an interface down.
- Split by what the code is about, not by what kind of code it is. `orders` is good; `handlers`, `queries`, `types` and `models` repeat the problem one level down (Bill Kennedy, package-oriented design).
- Replace a god object with small ones. Instead of one `Store` with 261 files of methods, give `orders.Store` and `inventory.Store` just the database handle and settings they use.
- Tests move with the code they test. The test-file and test-function limits usually clear up once the non-test split is done.

When heft says "No clean split yet", every file is tied to the others through the unexported identifiers it lists. Deal with those first: export them, move them into a small helper type in their own package, or, when only a few callers share one, copy it to each caller. Then run `heft suggest` again.

After each move, run `go build ./... && go test ./...`, then `heft baseline tighten` so the baseline locks in the smaller size.

## files

Non-test `.go` files in the package, excluding generated files and `exclude` globs. Default limit: 40. In the well-structured repos used to calibrate heft, no package went over 36. A package past this point almost always holds several topics. Split it as described above.

## loc

Lines that contain code: blank lines and comment-only lines aren't counted. Default limit: 10,000. File count can hide a few enormous files, and this limit catches them. Split the same way as `files`. If a single file is the culprit, split that file by topic first.

## test-files

`_test.go` files in the package. External test packages (`foo_test`) are counted separately from in-package tests. Default limit: 40. Every one of these files compiles into one test binary and re-runs whenever anything in the package changes. Split the package under test, and the tests follow.

## test-funcs

Functions named `Test*`, `Benchmark*`, `Fuzz*` and `Example*` in the test files (subtests aren't counted). Default limit: 200. This tracks how much work one cache miss costs. If the package itself is small but its tests are huge, look for integration tests that belong in their own package (for example `foo/footest` or a separate `_integration` package behind a build tag).

## exports

Exported top-level identifiers. Off by default. A wide exported surface usually means several APIs share one package. Set a limit if you want to cap it.

## imports

Distinct import paths in non-test files. Off by default. A package that imports 70 other packages is probably wiring many topics together. It is also rebuilt and retested whenever any of those 70 change.

## names

Package names such as `util`, `utils`, `common`, `helper`, `helpers`, `models` and `misc` say nothing about what the package provides, and they attract unrelated code until they become the biggest package in the repo (Go Blog, Package names; Dave Cheney, Practical Go). Move each function next to the type it works on, or into a package named after what it does (`money`, `slug`, `retry`). Configure the list with `banned-names`.

## References

- https://www.gobeyond.dev/standard-package-layout/
- https://www.ardanlabs.com/blog/2017/02/package-oriented-design.html
- https://dave.cheney.net/practical-go/presentations/qcon-china.html
- https://go.dev/blog/package-names
