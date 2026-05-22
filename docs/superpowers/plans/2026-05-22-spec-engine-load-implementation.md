# Spec Engine Load Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Wire `pkg/spec` + `pkg/spec/reporter` into the tracker engine. PR3 of the spec-first arc. Engine reads the spec at load, validates satisfies, calls Reporter.Pull at Run start, calls Reporter.Push after each successful node with satisfies.

**Architecture:** Five layers, in dependency order:

1. **IR plumbing** — `Graph.Spec`, `Node.Satisfies`. `convertNode` carries `satisfies` through.
2. **Spec loading** — `LoadDippinWorkflowFromIR` resolves and parses the spec when present.
3. **Validation** — walk Satisfies; bare unknown ACID = error; empty wildcard/range = warning.
4. **Engine Pull** — at `Run` start, populate PipelineContext.spec.status.* from Reporter.Pull.
5. **Engine Push** — after each success, batch the node's Satisfies into one Reporter.Push.

**Tech Stack:** Go 1.25+. `make test-short` / `make test-race`. Coverage ≥80%. Complexity ≤8 cyclo / ≤8 cog (test files exempt), file ≤500 lines.

**Reference:** `docs/superpowers/specs/2026-05-22-spec-engine-load-design.md`.

---

## File Structure

**Created:**
- `pipeline/spec_init.go` — blank imports of acai loader + reporter
- `pipeline/spec_engine.go` — helpers: Pull at start, Push after success, branch lookup
- `pipeline/spec_engine_test.go` — unit tests with fake reporter
- `pipeline/dippin_load_spec_test.go` — load-time spec resolution + satisfies validation
- `pipeline/testdata/with_spec.dip`, `pipeline/testdata/satisfies_unknown.dip`, etc. — fixtures

**Modified:**
- `pipeline/graph.go` — `Graph.Spec spec.Spec`, `Node.Satisfies []string`
- `pipeline/dippin_adapter.go` — `convertNode` copies `Satisfies`
- `pipeline/dippin_load.go` — load spec, validate satisfies
- `pipeline/engine.go` — call Pull at Run start, Push after success
- `CHANGELOG.md`

---

## Task 1: `Graph.Spec` + `Node.Satisfies`

- [ ] Failing tests in `pipeline/graph_test.go` for zero-value field semantics.
- [ ] Add `Spec spec.Spec` to `Graph`; add `Satisfies []string` to `Node`.
- [ ] Update `convertNode` to copy `irNode.Satisfies`.
- [ ] Verify existing tests still pass.

---

## Task 2: `pipeline/spec_init.go` blank imports

- [ ] Create file with blank imports of `pkg/spec/acai` and `pkg/spec/reporter/acai`.
- [ ] Add a test asserting `spec.Lookup("acai")` returns true after importing `pipeline`.

---

## Task 3: Spec loading in `LoadDippinWorkflowFromIR`

- [ ] Fixture `pipeline/testdata/with_spec.dip` references a fixture `features.yaml` in a sibling directory.
- [ ] Failing test loading `with_spec.dip` expects `graph.Spec != nil` and `Name() == "example"`.
- [ ] Failing test loading a workflow with an unknown loader expects a fatal error.
- [ ] Failing test loading a workflow with an invalid spec path expects a fatal error.
- [ ] Implement: after dippin validate/lint, if `workflow.Spec != nil`, resolve path relative to filename's dir, call `loader.Load`, stash on graph.

---

## Task 4: Validate satisfies against loaded spec

- [ ] Failing test: bare unknown ACID (`satisfies: example.NONE.1` with spec missing NONE) → fatal.
- [ ] Failing test: wildcard with no matches (`satisfies: example.NONE.*`) → warning appended to LintWarnings.
- [ ] Failing test: range with partial matches (`satisfies: example.AUTH.[1-99]` where AUTH only has 1-2) → no warning (partial matches are OK).
- [ ] Failing test: range with zero matches (`satisfies: example.AUTH.[50-99]` where AUTH only has 1-2) → warning.
- [ ] Implement validation pass after spec load.

---

## Task 5: Engine — Pull at Run start

- [ ] Unit test with fake reporter: `Engine.Run` calls `Pull` exactly once before the first node executes; results land on PipelineContext via `GetInternal("spec.status.<acid>")`.
- [ ] Unit test: Reporter.Available()=false → Pull not called; no panic.
- [ ] Unit test: Reporter.Pull errors → logged, run continues, PipelineContext spec.status.* empty.
- [ ] Implement in `pipeline/spec_engine.go`. Helper extracted to keep `Engine.Run` cyclomatic ≤ 8.

---

## Task 6: Engine — Push after successful node with Satisfies

- [ ] Unit test with fake reporter: node A has `Satisfies: [foo.BAR.1, foo.BAR.2]` and completes success → one Push call with two updates, both StatePass.
- [ ] Unit test: node with empty Satisfies → no Push call.
- [ ] Unit test: node fails (Outcome.Status != "success") → no Push (PR3 only reports successes; failure reporting is a separate concern).
- [ ] Unit test: Reporter.Push errors → logged, run continues normally.
- [ ] Implement in `pipeline/spec_engine.go`.

---

## Task 7: CHANGELOG + commit + push + PR

- [ ] CHANGELOG entry.
- [ ] `make fmt vet build test-short test-race coverage lint doctor`.
- [ ] `gocyclo -over 8` + `gocognit -over 8` on new code; refactor if needed.
- [ ] Commit, push, open PR against `michellepellon/tracker:main`.
