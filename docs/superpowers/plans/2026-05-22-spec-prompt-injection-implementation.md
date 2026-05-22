# Spec Prompt Injection Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Populate `ctx.spec.requirements` (YAML) and `ctx.spec.requirements_json` (JSON) before each node's handler runs, so authors can interpolate the resolved satisfies into their prompts via tracker's existing `${ctx.X}` syntax. PR4 of the spec-first arc.

**Architecture:** Two layers:
1. **Helper in `pipeline/spec_engine.go`** — `injectSatisfiesContext(pctx, node)` resolves the node's satisfies against `Graph.Spec`, serializes to YAML+JSON, sets two context keys.
2. **Single-line wire-up in `pipeline/engine.go`** — called from `processActiveNode` before `prepareExecNode`.

**Tech Stack:** `gopkg.in/yaml.v3` already in deps from PR1. Test runner via `go test`. Coverage ≥80%. Complexity ≤8.

**Reference:** `docs/superpowers/specs/2026-05-22-spec-prompt-injection-design.md`.

---

## File Structure

**Created:**
- `pipeline/spec_prompt_test.go` — injectSatisfiesContext unit tests + end-to-end through ExpandVariables.

**Modified:**
- `pipeline/spec_engine.go` — add `injectSatisfiesContext` + marshaling helpers.
- `pipeline/engine.go` — one-line call from `processActiveNode`.
- `CHANGELOG.md`.

---

## Task 1: `injectSatisfiesContext` helper (TDD)

- [ ] Failing tests in `pipeline/spec_prompt_test.go`:
  - Node with `Satisfies: [example.AUTH.1]` against the existing `with_spec_features.yaml` fixture → `pctx.Get("spec.requirements")` contains `id: example.AUTH.1` and the requirement text.
  - Node with `Satisfies: [example.AUTH.*]` → YAML contains both AUTH.1 and AUTH.2 (sorted by ID).
  - Node with `Satisfies: nil` → both keys unset.
  - Graph with no Spec → both keys unset.
  - YAML output is deterministic across multiple invocations (same input → same bytes).
  - JSON output parses cleanly and contains the same ACID set as YAML.
  - Deprecated requirements appear in the output (engine does not filter).
- [ ] Implement `injectSatisfiesContext` + `resolveSatisfies` + `marshalRequirementsYAML` + `marshalRequirementsJSON`.
- [ ] All tests pass.

---

## Task 2: Wire into `processActiveNode`

- [ ] Add `e.injectSatisfiesContext(s.pctx, node)` after the existing outcome/preferred_label reset.
- [ ] Existing engine tests continue to pass.

---

## Task 3: End-to-end test through `ExpandVariables`

- [ ] Test: load a workflow with `satisfies: example.AUTH.1`, manually call `injectSatisfiesContext`, then `ExpandVariables("Prompt: ${ctx.spec.requirements}", ...)` returns a string containing the requirement text.
- [ ] Same shape for `${ctx.spec.requirements_json}`.

---

## Task 4: CHANGELOG + commit + push + PR

- [ ] CHANGELOG entry.
- [ ] `make fmt vet build test-short test-race coverage lint doctor`.
- [ ] Complexity check on new code.
- [ ] Commit, push, open PR against `michellepellon/tracker:main`.

---

## Sequencing notes

T1 → T2 → T3 strictly sequential. T4 is the gate.
