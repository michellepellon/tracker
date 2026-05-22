# Spec Loader Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the data-layer foundation for spec-first workflows in tracker — a `pkg/spec/` interface package and a `pkg/spec/acai/` implementation. Backwards-compatible — no existing tracker code consumes the new package; the abstraction is justified by being the *only* sensible shape for the engine integration in PR3+.

**Architecture:** Three layers, in dependency order:

1. **`pkg/spec/` interface** — `Loader`, `Spec`, `Requirement`, `Registry`. Pure types + a global registry.
2. **`pkg/spec/acai/` impl** — YAML reader for acai's `feature.yaml`, with ACID expansion for ranges/wildcards. Registers itself at init().
3. **CHANGELOG + design doc cross-references.**

**Tech Stack:** Go 1.25+. Test runner via `make test-short` or `make test`. Coverage gate enforces ≥80% on changed files (`make coverage`). Complexity gate is loose on this PR: 24 baseline files already exceed the 500-line limit; new files must stay under 500 lines AND under cyclo 8 / cog 8.

**Reference:** `docs/superpowers/specs/2026-05-22-spec-loader-design.md`.

---

## File Structure

**Created:**
- `pkg/spec/spec.go` — `Loader`, `Spec`, `Requirement`, `Kind` types
- `pkg/spec/registry.go` — process-level loader registry
- `pkg/spec/spec_test.go` — Registry round-trip and type-level tests
- `pkg/spec/acai/loader.go` — acai YAML reader
- `pkg/spec/acai/resolve.go` — ACID pattern expansion (bare/range/wildcard)
- `pkg/spec/acai/loader_test.go` — unit tests against synthetic + real fixtures
- `pkg/spec/acai/testdata/minimal.yaml` — minimal valid spec
- `pkg/spec/acai/testdata/full.yaml` — every feature exercised (sub-reqs, notes, deprecated, constraints)
- `pkg/spec/acai/testdata/cognitoforms-py.yaml` — copy of `/Users/mpellon/dev/cognitoforms.py/features/cognitoforms-py/features.yaml` as a real-world regression fixture
- `pkg/spec/acai/testdata/malformed_no_feature.yaml`, `testdata/malformed_subsub.yaml`, etc. — error-path fixtures

**Modified:**
- `go.mod` — `replace github.com/2389-research/dippin-lang => github.com/michellepellon/dippin-lang <sha>` (already done in T1); adds `gopkg.in/yaml.v3`.
- `go.sum` — auto-updated.
- `CHANGELOG.md` — `## [Unreleased]` entry.

No existing files outside `go.mod` / `go.sum` / `CHANGELOG.md` are touched.

---

## Task 1: `pkg/spec/` interface (failing tests first)

**Files:**
- Create: `pkg/spec/spec_test.go`
- Create: `pkg/spec/spec.go` (after Step 2)
- Create: `pkg/spec/registry.go` (after Step 2)

- [ ] **Step 1: Write failing tests for the registry**

```go
package spec_test

import (
    "testing"

    "github.com/2389-research/tracker/pkg/spec"
)

type fakeLoader struct{ name string }

func (f fakeLoader) Name() string                       { return f.name }
func (f fakeLoader) Load(path string) (spec.Spec, error) { return nil, nil }

func TestRegistry_RegisterAndLookup(t *testing.T) {
    spec.Register(fakeLoader{name: "fake1"})
    l, ok := spec.Lookup("fake1")
    if !ok || l.Name() != "fake1" {
        t.Fatalf("Lookup(fake1) = %v, %v; want fakeLoader, true", l, ok)
    }
}

func TestRegistry_LookupMissing(t *testing.T) {
    _, ok := spec.Lookup("does-not-exist")
    if ok {
        t.Errorf("expected ok=false for missing loader")
    }
}

func TestRegistry_RegisteredList(t *testing.T) {
    spec.Register(fakeLoader{name: "registered-list-a"})
    spec.Register(fakeLoader{name: "registered-list-b"})
    names := spec.Registered()
    found := map[string]bool{}
    for _, n := range names {
        found[n] = true
    }
    if !found["registered-list-a"] || !found["registered-list-b"] {
        t.Errorf("Registered() missing entries: %v", names)
    }
}

func TestRequirement_ZeroValue(t *testing.T) {
    var r spec.Requirement
    if r.ID != "" || r.Kind != spec.KindComponent {
        t.Errorf("zero-value Requirement should be empty / Kind=Component, got %+v", r)
    }
}
```

Run: `go test ./pkg/spec/...` — expected: **build failure** (package doesn't exist).

- [ ] **Step 2: Create `pkg/spec/spec.go` and `pkg/spec/registry.go`**

`spec.go`: defines `Loader`, `Spec`, `Requirement`, `Kind`. Per the design doc.

`registry.go`: process-level map, `Register`, `Lookup`, `Registered`. Mutex-protected (loaders may register concurrently in test goroutines; not a hot path).

- [ ] **Step 3: Re-run tests**

Run: `go test ./pkg/spec/...` — expected: all four pass.

---

## Task 2: `pkg/spec/acai/` loader — minimal happy path

**Files:**
- Create: `pkg/spec/acai/loader_test.go`
- Create: `pkg/spec/acai/testdata/minimal.yaml`
- Create: `pkg/spec/acai/loader.go` (after Step 2)

- [ ] **Step 1: Failing test on minimal fixture**

```yaml
# pkg/spec/acai/testdata/minimal.yaml
feature:
  name: example
  product: example-product

components:
  AUTH:
    requirements:
      1: First auth requirement.
      2: Second auth requirement.
```

```go
package acai_test

import (
    "testing"

    "github.com/2389-research/tracker/pkg/spec"
    _ "github.com/2389-research/tracker/pkg/spec/acai" // register
)

func TestLoad_Minimal(t *testing.T) {
    l, ok := spec.Lookup("acai")
    if !ok {
        t.Fatalf("acai loader not registered")
    }
    s, err := l.Load("testdata/minimal.yaml")
    if err != nil {
        t.Fatalf("Load: %v", err)
    }
    if s.Name() != "example" {
        t.Errorf("Name = %q, want example", s.Name())
    }
    reqs := s.Requirements()
    if len(reqs) != 2 {
        t.Fatalf("Requirements len = %d, want 2", len(reqs))
    }
    if reqs[0].ID != "example.AUTH.1" || reqs[0].Component != "AUTH" || reqs[0].Number != "1" {
        t.Errorf("first req mismatch: %+v", reqs[0])
    }
    if reqs[0].Text != "First auth requirement." {
        t.Errorf("Text = %q", reqs[0].Text)
    }
}
```

- [ ] **Step 2: Implement `loader.go` happy path**

Use `gopkg.in/yaml.v3`. Parse into an intermediate struct, walk `components.<NAME>.requirements.<N>`, build `[]Requirement`. Add `package acai` `init()` that calls `spec.Register(loader{})`.

- [ ] **Step 3: Pass test**

---

## Task 3: Loader — long-form, sub-requirements, notes, deprecated

- [ ] **Step 1: Failing tests against `testdata/full.yaml`**

```yaml
# pkg/spec/acai/testdata/full.yaml
feature:
  name: full
  product: full

components:
  AUTH:
    requirements:
      1: Short-form requirement.
      1-note: A note attached to requirement 1.
      1-1: A sub-requirement of 1.
      2:
        requirement: Long-form requirement.
        deprecated: true

constraints:
  PKG:
    description: Packaging constraints.
    requirements:
      1: Use uv.
```

Cases to assert:
- Notes attach to parent: `reqs["full.AUTH.1"].Notes == ["A note attached to requirement 1."]`
- Sub-requirement parent set: `reqs["full.AUTH.1-1"].Parent == "full.AUTH.1"`
- Deprecated flag round-trips: `reqs["full.AUTH.2"].Deprecated == true`
- Long-form text: `reqs["full.AUTH.2"].Text == "Long-form requirement."`
- Constraint kind: `reqs["full.PKG.1"].Kind == spec.KindConstraint`

- [ ] **Step 2: Extend loader to handle long-form via `yaml.Node` or a `map[string]any` second pass**

- [ ] **Step 3: All cases pass**

---

## Task 4: Loader — error paths

- [ ] **Step 1: Failing tests**

```yaml
# testdata/malformed_no_feature.yaml
components:
  X:
    requirements:
      1: thing
```

```yaml
# testdata/malformed_subsub.yaml
feature:
  name: x
  product: x
components:
  A:
    requirements:
      1-1-1: invalid — too deep
```

Tests assert:
- Missing `feature.name` → non-nil error with message containing "feature.name".
- Sub-sub-requirement → non-nil error with message containing "sub-sub" or referencing `1-1-1`.
- Empty `components` AND `constraints` → non-nil error.
- Non-existent file → non-nil error containing "open".

- [ ] **Step 2: Implement error checks in loader**

- [ ] **Step 3: All error-path tests pass**

---

## Task 5: ACID resolution — bare, wildcard, range

**Files:**
- Create: `pkg/spec/acai/resolve.go`
- Modify: `pkg/spec/acai/loader_test.go` (add `TestResolve_*`)

- [ ] **Step 1: Failing tests**

```go
func TestResolve_Bare(t *testing.T) {
    s := loadFixture(t, "testdata/full.yaml")
    got := s.Resolve("full.AUTH.1")
    if len(got) != 1 || got[0].ID != "full.AUTH.1" { ... }
}

func TestResolve_Wildcard(t *testing.T) {
    s := loadFixture(t, "testdata/full.yaml")
    got := s.Resolve("full.AUTH.*")
    // Includes 1, 1-1, 2 — i.e. every requirement in component AUTH.
    if len(got) != 3 { ... }
}

func TestResolve_Range(t *testing.T) {
    s := loadFixture(t, "testdata/full.yaml")
    got := s.Resolve("full.AUTH.[1-2]")
    // Includes 1 and 2; sub-requirement 1-1 is NOT included by range syntax (ranges
    // are over top-level requirement numbers only, matching how callers read them).
    if len(got) != 2 { ... }
}

func TestResolve_UnknownFeature(t *testing.T) {
    s := loadFixture(t, "testdata/full.yaml")
    if got := s.Resolve("wrongname.AUTH.1"); len(got) != 0 { ... }
}

func TestResolve_GapsInRangeAreSkipped(t *testing.T) {
    // testdata/full.yaml AUTH has 1, 1-1, 2. Range [1-5] returns just 1 and 2.
    s := loadFixture(t, "testdata/full.yaml")
    got := s.Resolve("full.AUTH.[1-5]")
    if len(got) != 2 { ... }
}
```

- [ ] **Step 2: Implement `Resolve` in `resolve.go`**

Split pattern via regex matching dippin's DIP139 shape; dispatch on the requirement-segment form. Each branch (`bare`, `wildcard`, `range`) is its own function to stay under cyclomatic 8.

- [ ] **Step 3: Pass all resolve tests**

---

## Task 6: Real-world fixture — cognitoforms-py

- [ ] **Step 1: Copy `features.yaml` into testdata**

```sh
cp /Users/mpellon/dev/cognitoforms.py/features/cognitoforms-py/features.yaml \
   pkg/spec/acai/testdata/cognitoforms-py.yaml
```

- [ ] **Step 2: Assertion test that the full real spec loads**

```go
func TestLoad_CognitoFormsPy(t *testing.T) {
    s := loadFixture(t, "testdata/cognitoforms-py.yaml")
    if s.Name() != "cognitoforms-py" { ... }
    // Spot-check: AUTH should have at least 4 requirements; CLIENT at least 6.
    if len(s.Resolve("cognitoforms-py.AUTH.*")) < 4 { ... }
    if len(s.Resolve("cognitoforms-py.CLIENT.*")) < 6 { ... }
    // No constraint requirement should be Kind=Component.
    for _, r := range s.Requirements() {
        if r.Component == "PACKAGING" && r.Kind != spec.KindConstraint { ... }
    }
}
```

This is the load-bearing test for the design — if the real spec doesn't round-trip, the abstraction is wrong.

- [ ] **Step 3: Pass**

---

## Task 7: CHANGELOG + commit + PR

**Files:**
- Modify: `CHANGELOG.md`
- Modify: `go.sum` (auto)

- [ ] **Step 1: Add CHANGELOG entry under `## [Unreleased]`**

- [ ] **Step 2: `make fmt-check vet build test-short test-race lint`**

Skip `make complexity` — pre-existing failures on 24 baseline files. New files must stay under 500 lines and under cyclo 8 / cog 8; verify by running `gocyclo` directly on the new files only.

- [ ] **Step 3: Commit + push + open PR**

```sh
git add -A
git commit -m "feat(spec): add pkg/spec/ interface + acai loader

PR1 of 6 in the spec-first workflow arc. ..."
git push -u origin spec-loader
gh pr create --repo michellepellon/tracker --base main --title "..." --body "..."
```

---

## Sequencing notes

Tasks 1 → 2 → 3 → 4 → 5 → 6 are strictly sequential (each depends on the previous). Task 7 is the gate.

Subagent parallelism doesn't help much here — the work is small enough that the orchestration overhead exceeds the savings.
