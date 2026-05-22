# Spec Evaluator Binding Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Promote `spec.status.<acid>` from tracker's internal context to the user-visible context, so workflow authors can route edges with `when ctx.spec.status.foo.BAR.1 = pass`. No evaluator changes — tracker's existing operator vocabulary covers it.

**Reference:** `docs/superpowers/specs/2026-05-22-spec-evaluator-design.md`.

---

## File Structure

**Created:**
- `pipeline/spec_evaluator_test.go` — condition-routing + no-dirty-bleed tests.

**Modified:**
- `pipeline/spec_engine.go` — `pullSpecStatuses` uses `MergeWithoutDirty` instead of `SetInternal`.
- `pipeline/spec_engine_test.go` — existing PR3 tests updated to read via `Get` instead of `GetInternal`.
- `CHANGELOG.md`.

No other files touched.

---

## Task 1: Migrate `pullSpecStatuses` storage

- [ ] Failing test: assert `pctx.Get("spec.status.example.AUTH.1")` returns "pass" after pulling against a fake reporter that returned `StatePass` for that ACID.
- [ ] Modify `pullSpecStatuses` to build a `map[string]string` and call `pctx.MergeWithoutDirty(updates)`.
- [ ] Update existing PR3 tests in `spec_engine_test.go` — `pctx.GetInternal(...)` → `pctx.Get(...)`.

---

## Task 2: Condition routing test (TDD)

- [ ] New test in `spec_evaluator_test.go`: load a workflow with an edge `when ctx.spec.status.example.AUTH.1 = pass`, populate the key via direct `pctx.Set` (mimicking what `pullSpecStatuses` does), call `EvaluateCondition` on the edge, confirm true.
- [ ] Negative case: same setup with status = "fail" → false.
- [ ] Absent key: condition `ctx.spec.status.foo.BAR.99 = pass` returns false (key not set, lenient).

---

## Task 3: No-dirty-bleed test (TDD)

- [ ] Test: call `pullSpecStatuses`, then `pctx.ScopeToNode("A")`, then assert `pctx.Get("node.A.spec.status.example.AUTH.1")` is empty/false.

---

## Task 4: CHANGELOG + commit + push + PR

- [ ] CHANGELOG entry under `[Unreleased]`.
- [ ] `make fmt vet build test-short test-race coverage lint doctor`.
- [ ] `gocyclo`/`gocognit` -over 8 on new code.
- [ ] Commit, push, open PR against `michellepellon/tracker:main`.
