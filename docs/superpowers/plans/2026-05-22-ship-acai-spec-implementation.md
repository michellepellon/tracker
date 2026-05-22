# `ship_acai_spec.dip` Implementation Plan

> Steps use checkbox (`- [ ]`) syntax.

**Reference:** `docs/superpowers/specs/2026-05-22-ship-acai-spec-design.md`.

---

## File Structure

**Created:**
- `examples/ship_acai_spec.dip` — the template workflow
- `workflows/ship_acai_spec.dip` — synced copy (binary embeds it)
- `pipeline/spec_ship_test.go` — load test
- `pipeline/testdata/cognitoforms_py_features.yaml` — fixture for the load test
- `docs/spec-first-authoring.md` — author-facing guide

**Modified:**
- `Makefile` — sync-workflows + check-workflows + doctor lists
- `CHANGELOG.md`

---

## Task 1: Write the template workflow

- [ ] Draft `examples/ship_acai_spec.dip` with Implement → Verify → FixGaps loop.
- [ ] Use realistic patterns matching the cognitoforms-py feature shape.
- [ ] Confirm `dippin validate examples/ship_acai_spec.dip` passes.
- [ ] Confirm `dippin lint examples/ship_acai_spec.dip` is clean.
- [ ] Confirm `dippin doctor examples/ship_acai_spec.dip` grades A.

---

## Task 2: Sync to `workflows/`

- [ ] `make sync-workflows` (after extending the recipe).
- [ ] Update Makefile: extend the `sync-workflows`, `check-workflows`, and `doctor` recipes.
- [ ] Confirm `make check-workflows` reports the file in sync.
- [ ] Confirm `make doctor` succeeds with the new file.

---

## Task 3: End-to-end load test

- [ ] Copy `cognitoforms.py/features/cognitoforms-py/features.yaml` to `pipeline/testdata/cognitoforms_py_features.yaml` (rename so the spec path can resolve relative to the .dip).
- [ ] New `pipeline/spec_ship_test.go`:
  - Reads `examples/ship_acai_spec.dip`, runs it through `LoadDippinWorkflow`.
  - Asserts no error, `Graph.Spec != nil`, `Graph.Spec.Name() == "cognitoforms-py"`.
  - Asserts `Implement` node `Satisfies` contains the AUTH/CLIENT/etc. patterns.
  - Asserts `Verify` node `VerifyACID` matches.
  - Asserts `Graph.LintWarnings` is empty.

---

## Task 4: Author-facing docs

- [ ] `docs/spec-first-authoring.md` — short guide pointing at the design docs and walking through the cognitoforms-py case.

---

## Task 5: CI + commit + PR

- [ ] CHANGELOG entry.
- [ ] `make fmt vet build test-short test-race coverage lint doctor`.
- [ ] Commit, push, open PR.
