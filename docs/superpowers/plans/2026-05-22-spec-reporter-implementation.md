# Spec Reporter Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Ship `pkg/spec/reporter/` interface + `pkg/spec/reporter/acai/` impl. Pure library — no engine wiring. PR2 of the 6-PR spec-first arc.

**Architecture:** Three layers:

1. **`pkg/spec/reporter/` interface** — Reporter, Target, Status, State, Registry. Pure types + global registry, mirrors `pkg/spec/`.
2. **`pkg/spec/reporter/acai/` impl** — wraps the `acai` CLI via subprocess. Injectable `commandRunner` for testability. Registers at `init()`.
3. **Integration test** — exercises the real CLI binary, skipped when `ACAI_API_TOKEN` is absent.

**Tech Stack:** Go 1.25+. Test runner via `make test-short` / `make test-race`. Coverage gate ≥80%. Complexity ≤8 cyclo / ≤8 cog, file ≤500 lines (test files exempt).

**Reference:** `docs/superpowers/specs/2026-05-22-spec-reporter-design.md`.

---

## File Structure

**Created:**
- `pkg/spec/reporter/reporter.go` — Reporter, Target, Status, State, errors
- `pkg/spec/reporter/registry.go` — Register, Lookup, Registered
- `pkg/spec/reporter/reporter_test.go` — registry + State.String tests
- `pkg/spec/reporter/acai/reporter.go` — acai Reporter impl
- `pkg/spec/reporter/acai/runner.go` — default commandRunner (exec.CommandContext)
- `pkg/spec/reporter/acai/reporter_test.go` — unit tests with fake runner
- `pkg/spec/reporter/acai/integration_test.go` — live-binary tests, build tag `integration`

**Modified:**
- `CHANGELOG.md` — `## [Unreleased]` entry.

No existing files touched outside `CHANGELOG.md`.

---

## Task 1: `pkg/spec/reporter/` interface (TDD)

- [ ] **Step 1:** Write `reporter_test.go` with failing tests for Registry, State.String, and zero-value Status semantics.
- [ ] **Step 2:** Create `reporter.go` (types, ErrUnavailable sentinel) and `registry.go` (sync-protected map).
- [ ] **Step 3:** `go test ./pkg/spec/reporter/...` — all pass.

---

## Task 2: `pkg/spec/reporter/acai/` impl, happy path (TDD)

- [ ] **Step 1:** Failing tests covering:
  - `Available(ctx)` returns true when fake runner returns exit 0
  - `Available(ctx)` returns false when fake runner returns "Missing API bearer token" on stderr
  - `Pull` parses a canned `acai feature ... --json` JSON output into map[acid]Status
  - `Push` invokes the fake runner with the right argv and stdin
- [ ] **Step 2:** Implement `reporter.go` and `runner.go`. Inject runner via `New(opt ...Option)`; `WithRunner(f)` for tests.
- [ ] **Step 3:** Pass.

---

## Task 3: acai reporter — error paths (TDD)

- [ ] **Step 1:** Failing tests:
  - Binary missing (`exec.LookPath` fails) → `Available` false; `Pull` returns nil + nil (no-op); `Push` returns `ErrUnavailable`
  - Server unreachable (fake runner returns non-zero exit + stderr) → `Pull` and `Push` return errors containing the stderr text
  - Malformed JSON from CLI → `Pull` returns parse error
- [ ] **Step 2:** Implement the error-classification helper.
- [ ] **Step 3:** Pass.

---

## Task 4: State string mapping

- [ ] **Step 1:** Failing tests for the acai status string → State mapping (pass/passed/fail/failed/blocked/pending/empty/unknown).
- [ ] **Step 2:** Extract `parseState(s string) State` helper.
- [ ] **Step 3:** Pass.

---

## Task 5: Integration test (skipped without token)

- [ ] **Step 1:** Create `pkg/spec/reporter/acai/integration_test.go` with `//go:build integration` tag.
- [ ] **Step 2:** Test skips when `ACAI_API_TOKEN == ""`. When present, asserts `Available(ctx)` returns true against the real binary.
- [ ] **Step 3:** Verify `go test -tags=integration ./pkg/spec/reporter/acai/` passes when token is set, no-ops otherwise.

---

## Task 6: CHANGELOG + commit + push + PR

- [ ] **Step 1:** CHANGELOG entry under `## [Unreleased]`.
- [ ] **Step 2:** `make fmt`, `make vet`, `make build`, `make test-short`, `make test-race`, `make coverage`, `make lint`, `make doctor`. (Skip `make complexity` — pre-existing baseline failure.)
- [ ] **Step 3:** `gocyclo -over 8` + `gocognit -over 8` on new code; refactor any violations.
- [ ] **Step 4:** Commit, push, open PR against `michellepellon/tracker:main`.

---

## Sequencing notes

Tasks 1 → 2 → 3 → 4 are sequential. Task 5 is independent of 4 — can interleave if running subagents.
