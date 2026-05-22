# `ship_acai_spec.dip` Design

> Status: **Design**. Implementation plan: [`docs/superpowers/plans/2026-05-22-ship-acai-spec-implementation.md`](../plans/2026-05-22-ship-acai-spec-implementation.md). PR6 (final) of the 6-PR spec-first arc.

## Motivation

PR1–PR5.5 built the machinery: loader, reporter, prompt injection, condition routing, verify_acid. This PR ships the **canonical workflow** that uses all of it. Without a template, every author would have to assemble the pieces from scratch and rediscover the patterns we've already settled on.

The deliverable is a single `.dip` file that authors can copy with `tracker init ship_acai_spec`, point at their own `features.yaml`, customize the per-component scope, and run end-to-end:

1. Implement → an agent reads the spec via `${ctx.spec.requirements}` and writes code referencing each ACID
2. Verify → a tool node greps the working tree via `verify_acid:` and populates `spec.coverage.*`
3. Fix loop → a conditional edge routes back to Implement when any ACID is `uncovered`
4. Push → the engine automatically reports `StatePass` for every satisfied ACID after each successful node
5. Done

## Non-goals

- **Generic iteration over arbitrary spec components.** Tracker's `.dip` grammar doesn't natively support "loop over the spec's components" — that would need a manager_loop subgraph and component-list plumbing in `ctx.spec.*`. PR6 ships a workflow with **placeholder concrete components** (matching the cognitoforms-py shape) that authors edit. A generic component-iterating variant is a future PR.
- **Self-modifying spec.** The workflow doesn't add or remove requirements; if the spec changes, the author re-runs.
- **Multi-language scaffolding.** This template assumes Python (matches the cognitoforms-py case we're proving). Other languages need their own variant.

## What changes

### `examples/ship_acai_spec.dip` (new)

A workflow with this shape:

```dip
workflow ShipAcaiSpec
  goal: "Implement a Python library against an acai feature spec, with per-ACID verification and dashboard reporting."
  spec: acai features/cognitoforms-py/features.yaml
  requires: git
  start: Start
  exit: Done

  defaults
    model: claude-sonnet-4-6
    provider: anthropic
    max_retries: 2

  agent Start
    label: Start

  # Phase 1: Implement everything declared in the spec.
  agent Implement
    label: "Implement spec requirements"
    satisfies: cognitoforms-py.AUTH.*, cognitoforms-py.CLIENT.*, cognitoforms-py.FORMS.*, ...
    prompt:
      Implement the following requirements as a Python library.
      Reference each ACID in a code comment near the satisfying logic.
      Mirror the ACID in the matching test's name.

      ${ctx.spec.requirements}

  # Phase 2: Confirm every ACID is referenced somewhere in src/ or tests/.
  tool Verify
    label: "Verify ACIDs are referenced in code"
    command:
      set -eu
      grep -rn 'cognitoforms-py\.' src/ tests/ 2>/dev/null || echo no-refs-found
    verify_acid: cognitoforms-py.AUTH.*, cognitoforms-py.CLIENT.*, ...
    timeout: 30s

  # Phase 3: If anything is uncovered, route back to implement.
  agent FixGaps
    label: "Implement missing ACIDs"
    prompt:
      One or more ACIDs from the spec are not yet referenced in code or tests.
      Implement the missing requirements.

      ${ctx.spec.requirements}

  agent Done
    label: Done

  edges
    Start -> Implement
    Implement -> Verify
    Verify -> Done    when ctx.spec.coverage.cognitoforms-py.AUTH.1 = covered
    Verify -> FixGaps when ctx.spec.coverage.cognitoforms-py.AUTH.1 = uncovered
    FixGaps -> Verify
```

The file:
- **Lints clean** (`dippin lint`) — no DIP warnings.
- **Grades A** (`dippin doctor`) — same bar as the other built-in workflows.
- **Loads against a real spec** — the test in `pipeline/spec_ship_test.go` parses it and confirms `Graph.Spec`, `Satisfies`, and `VerifyACID` all populate.

### `workflows/ship_acai_spec.dip` (new, synced copy)

Per tracker's existing pattern. The `examples/` copy is the source of truth; `make sync-workflows` copies it to `workflows/`; `make check-workflows` verifies they're in sync.

### `Makefile` (3 modifications)

```makefile
sync-workflows:
  ...
  cp examples/ship_acai_spec.dip workflows/

check-workflows:
  for f in ... ship_acai_spec.dip; do ...

doctor:
  for f in ... ship_acai_spec.dip; do ...
```

### `docs/spec-first-authoring.md` (new)

A short author-facing guide:
- Quick start: `tracker init ship_acai_spec` → customize → run
- The four mechanics (spec, satisfies, verify_acid, status routing) cross-referenced to their design docs
- A worked example using the cognitoforms-py spec
- Failure modes and how to debug them

### `pipeline/spec_ship_test.go` (new)

End-to-end test that:
1. Loads `examples/ship_acai_spec.dip` against the real `cognitoforms-py/features.yaml` (copied into testdata to avoid path issues).
2. Asserts `Graph.Spec.Name() == "cognitoforms-py"`.
3. Asserts the `Implement` node carries the full satisfies list.
4. Asserts the `Verify` node carries the full VerifyACID list.
5. Asserts `Graph.LintWarnings` is empty (no TRK-SAT / TRK-VAC warnings).

## Acceptance criteria

A workflow author can:

1. Run `tracker init ship_acai_spec` and get a local copy of the template.
2. Edit the `spec:` line to point at their own `features.yaml`.
3. Edit the `satisfies:` and `verify_acid:` patterns to match their feature names.
4. Run `tracker validate ship_acai_spec.dip` and see zero errors.
5. Run `tracker simulate ship_acai_spec.dip` and see the engine load the spec, populate coverage, and route the conditional edges correctly under fake inputs.
6. Run `tracker ship_acai_spec` against a real spec and have it execute the implement → verify → fix loop with real LLM calls and real status pushes (this is the actual "shipping" step).

## What this PR is NOT

- **A test of the real LLM path.** PR6 verifies the workflow loads and routes correctly under simulation. Running it for real against a live LLM is a separate operational step (handled by the author when they execute `tracker ship_acai_spec`).
- **A spec-component iterator.** The workflow is one-pass (all components implemented together). A generic per-component iterator using `manager_loop` is a future PR.
- **A test of the acai server I/O.** PR2's integration tests cover that. PR6 doesn't push to a real server; the workflow is a template, and real pushes happen when authors run it.

## Compatibility

- New files only. No diff outside `examples/`, `workflows/`, `Makefile`, `docs/`, `pipeline/spec_ship_test.go`, `pipeline/testdata/`, and `CHANGELOG.md`.
- The `make doctor` target gains one new file in its grade-A check.
