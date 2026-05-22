# Spec Engine Load Design

> Status: **Design**. Implementation plan: [`docs/superpowers/plans/2026-05-22-spec-engine-load-implementation.md`](../plans/2026-05-22-spec-engine-load-implementation.md). PR3 of the 6-PR spec-first arc. Builds on PR1 (loader) + PR2 (reporter).

## Motivation

PR1 added the spec loader; PR2 added the reporter. Tracker can read spec files and shell out to acai. But nothing in the engine actually *calls* either — the abstractions are dead weight until the engine wires them up.

This PR puts the wire in. The minimum useful behavior:

1. **Load the spec at workflow startup.** When a `.dip` workflow declares `spec: acai path/to/features.yaml`, tracker resolves the loader by name, calls `Load`, stashes the result on the Graph.
2. **Validate `satisfies:` against the loaded spec.** Unknown ACIDs become hard errors (fail-fast at load, not at run). Wildcards or ranges that resolve to nothing become warnings.
3. **Pull existing statuses once at engine `Run` start.** Stash on the PipelineContext under an internal key so downstream features (PR4+) can read them.
4. **Push status updates after each successful node** that declared `satisfies:`. Best-effort — log and continue on reporter errors, never abort the workflow.

After this PR, a workflow author writing `spec: acai features.yaml` + `satisfies: foo.BAR.1` on their nodes will see ACID status flow back to the acai dashboard automatically. That's the smallest deliverable that justifies the abstraction.

## Non-goals (PR3 scope)

- **Agent prompt injection.** When an agent has `satisfies:`, the engine doesn't inject the resolved requirement text into the LLM prompt yet. That's PR4 — it's a separate concern with its own design questions (where in the prompt, how to format, what to do with notes/deprecated/parent).
- **`ctx.spec.*` evaluator bindings.** `when ctx.spec.passed("foo.BAR.1")` doesn't work yet — the condition evaluator gains no new syntax in this PR. That's PR5.
- **`verify_acid:` declarative tool primitive.** Sibling to `marker_grep:`. Also PR5.
- **Built-in `ship_acai_spec.dip` workflow.** PR6.
- **Reporter backoff / retry / batching.** PR2's reporter does one transport call per invocation; the engine in PR3 batches "all the satisfies ACIDs on one node" into one Push call but otherwise calls Push naively. If that turns out wrong, a later PR can add coalescing.

## What changes

### `pipeline/graph.go` — `Spec` field on `Graph` and `Satisfies` on `Node`

```go
type Graph struct {
    // ...existing fields...
    Spec spec.Spec // loaded spec or nil; populated by LoadDippinWorkflowFromIR
}

type Node struct {
    // ...existing fields...
    Satisfies []string // ACIDs this node declares; copied from ir.Node.Satisfies
}
```

`Satisfies` carries the raw pattern strings from dippin's IR forward — the engine resolves them against `Graph.Spec` at the moment of use rather than expanding eagerly at load time. That keeps the graph shape stable even if a future refactor wants to lazily expand wildcards.

### `pipeline/dippin_load.go` — Spec loading

After the existing dippin `Validate` + `Lint` passes, if `workflow.Spec != nil`:

1. Look up the loader via `spec.Lookup(workflow.Spec.Loader)`. Unknown loader → fatal error (`unknown spec loader %q; registered loaders: %v`).
2. Resolve `workflow.Spec.Path` relative to `filename`'s directory.
3. Call `loader.Load(resolvedPath)`. Parse failure → fatal error.
4. Stash on `Graph.Spec`.
5. Walk `workflow.Nodes`; for each `Satisfies` entry, call `graph.Spec.Resolve(pattern)`:
   - Bare ACID (`foo.BAR.1`) resolving to empty → fatal error (`tracker: node %q satisfies unknown ACID %q`).
   - Wildcard or range resolving to empty → warning appended to `Graph.LintWarnings` (`tracker: node %q satisfies pattern %q resolves to no requirements`).

`Satisfies` is also copied onto each `Graph.Node` so the engine can find it later without re-walking the IR.

### `pipeline/dippin_adapter.go` — `Node.Satisfies` plumbing

`convertNode` copies `irNode.Satisfies` onto the returned `*Node`. Trivial change.

### `pipeline/spec_init.go` (new) — Blank import of the acai packages

A new file with just blank imports:

```go
package pipeline

import (
    _ "github.com/2389-research/tracker/pkg/spec/acai"          // registers "acai" loader
    _ "github.com/2389-research/tracker/pkg/spec/reporter/acai" // registers "acai" reporter
)
```

So that loading the `pipeline` package transitively registers the acai loader + reporter. Without this, `spec.Lookup("acai")` returns false.

### `pipeline/engine.go` — Pull at start, Push after success

Two minimal additions:

1. **Pull at `Run` start.** Before entering the node-iteration loop, if `e.graph.Spec != nil`, look up a reporter matching the spec's loader name (`reporter.Lookup(workflow.Spec.Loader)`), check `Available(ctx)`, and call `Pull`. Results go onto the PipelineContext via `SetInternal("spec.status."+acid, state.String())`. Reporter errors are logged and ignored.
2. **Push after success.** After each node completes with `Outcome.Status == "success"`, if the node has `Satisfies`, build a `[]reporter.Status` (all StatePass, comment = `node:%s`), call `reporter.Push`. Errors logged, never propagated.

The reporter target's `Implementation` field defaults to the current git branch name (read via `git rev-parse --abbrev-ref HEAD`) with a fallback to `"unknown"` if git isn't available. This is deliberately simple — operators who need a different slot can override in a follow-up PR.

### `pipeline/spec_engine.go` (new) — Helpers extracted to keep complexity ≤ 8

The Pull / Push logic + git-branch lookup + status-key formatting live here so `engine.go` stays close to its current cyclomatic budget.

## Acceptance criteria

A workflow author can:

1. Write `spec: acai features.yaml` in their `.dip` header. When tracker loads the workflow, it parses the spec; an invalid spec path or unknown loader fails fast with a clear error.
2. Write `satisfies: foo.BAR.1` on a node where `foo.BAR.1` doesn't exist in the spec; tracker rejects the workflow at load time with a diagnostic naming the bad ACID.
3. Write `satisfies: foo.BAR.[1-99]` where only 1-3 exist; tracker accepts the workflow and emits a warning to LintWarnings.
4. Run a workflow with a valid spec + satisfies; with `ACAI_API_TOKEN` set and the acai binary on PATH, status updates appear in the dashboard for every node that completes successfully.
5. Run the same workflow without `ACAI_API_TOKEN`; the workflow completes normally (no errors, no panics) — the reporter is silently skipped via `Available()`.

## What this PR is NOT

- **No agent gets new prompt context.** The engine doesn't read `Graph.Spec` from inside the codergen handler. That's PR4.
- **No condition evaluator changes.** `when ctx.spec.passed("...")` parses as an unknown-namespace condition and emits the existing DIP120 warning. That's PR5.
- **No new node attribute.** `verify_acid:` doesn't exist yet. PR5.
- **No CLI surface.** `tracker doctor` doesn't probe the spec yet. PR5 or PR6.
- **No built-in workflow.** PR6.

## Compatibility

- Workflows without `spec:` are unaffected. Every code path keys off `graph.Spec != nil`.
- Workflows with `spec:` but no registered loader fail at load. This is the desired behaviour — the loader's job is to register itself; if it isn't, the workflow author needs to know.
- The reporter call sites are guarded by `Available()` — missing token or missing binary degrades to no-op silently.

## Open questions

- **Implementation slot.** Using the git branch name is a guess. acai's data model expects an Implementation name per branch, so this is probably right, but if the workflow lives in a non-git workspace this fails over to "unknown". Confirmed acceptable for PR3; PR6's `ship_acai_spec.dip` may want a `--impl` flag override.
- **Reporter selection.** PR3 hardcodes the assumption that `spec.Loader == reporter.Loader` (i.e., the acai loader pairs with the acai reporter). That's true today (only one of each), but the abstraction allows divergence. Defer until a second reporter format ships.
- **Failure-mode policy for Push.** Currently log + continue on error. Some workflows may want "abort the run if status push fails." Defer.
