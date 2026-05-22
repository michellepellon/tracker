# Spec Evaluator Binding Design

> Status: **Design**. Implementation plan: [`docs/superpowers/plans/2026-05-22-spec-evaluator-implementation.md`](../plans/2026-05-22-spec-evaluator-implementation.md). PR5 of the spec-first arc. Builds on PR3.

## Motivation

PR3 introduced `pullSpecStatuses` which seeds the engine with ACID statuses from the spec server at workflow start. It stashed them via `SetInternal` under keys like `spec.status.foo.BAR.1`. That makes the data available to handlers (which can call `pctx.GetInternal`) but **not to edge conditions**, because the dippin condition evaluator reads from the user-visible context, not the internal store.

The result is awkward: a workflow author who wants to route on per-ACID status today has to write `when ctx.internal.spec.status.foo.BAR.1 = pass`. The `ctx.internal.*` namespace is a tracker implementation detail leaked into the surface area for workflow authors.

This PR closes the gap. After it, the canonical condition is:

```dip
edges
  CheckAuth -> SkipAuth when ctx.spec.status.example.AUTH.1 = pass
  CheckAuth -> ImplementAuth when ctx.spec.status.example.AUTH.1 != pass
```

No new operators, no new evaluator syntax. Just the keys promoted to the right namespace.

## Non-goals (PR5 scope)

- **`ctx.spec.passed(...)` function-call syntax.** Tracker's condition evaluator currently supports only operator-based comparisons (`=`, `!=`, `contains`, etc.). Adding function calls would require parser work in dippin AND tracker. The `ctx.spec.status.<acid> = pass` pattern covers the same use case with zero parser work. Defer function-call syntax unless we encounter an expression the operator vocabulary can't express.
- **`verify_acid:` declarative tool primitive.** Originally bundled into this PR; split out into its own (PR5.5) because it needs a dippin grammar change.
- **Updating built-in workflows.** None of the `examples/*.dip` shipped with tracker uses spec features yet; PR6 adds the first one.

## What changes

### `pipeline/spec_engine.go` — `MergeWithoutDirty` instead of `SetInternal`

In `pullSpecStatuses`, the per-ACID loop:

```go
for acid, status := range statuses {
    pctx.SetInternal(SpecStatusKeyPrefix+acid, status.State.String())
}
```

becomes a single batched call to `MergeWithoutDirty`:

```go
updates := make(map[string]string, len(statuses))
for acid, status := range statuses {
    updates[SpecStatusKeyPrefix+acid] = status.State.String()
}
pctx.MergeWithoutDirty(updates)
```

`MergeWithoutDirty` puts the keys into the user-visible store (so `ctx.Get("spec.status.example.AUTH.1")` works) **without** marking them dirty (so they don't bleed into `node.<id>.spec.status.*` after `ScopeToNode` runs). That's the key property: the values are global-and-readable, not per-node.

### `pipeline/condition.go` — no changes

Tracker's existing evaluator handles `ctx.spec.status.<acid>` natively:

1. `EvaluateCondition` calls `resolveVariable("ctx.spec.status.example.AUTH.1", ctx)`.
2. `resolveVariable` strips `ctx.` and calls `resolveCtxNamespace("spec.status.example.AUTH.1", ctx)`.
3. `resolveCtxNamespace` calls `ctx.Get("spec.status.example.AUTH.1")` — returns the value we stored via `MergeWithoutDirty`.

Zero diff in the evaluator. The whole PR is one helper rewrite + tests.

### Values authors can compare against

The values are the lowercase `String()` form of `reporter.State`:

| State           | String  | Author writes              |
|-----------------|---------|----------------------------|
| `StatePass`     | `pass`  | `... = pass`               |
| `StateFail`     | `fail`  | `... = fail`               |
| `StateBlocked`  | `blocked` | `... = blocked`          |
| `StatePending`  | `pending` | `... = pending`          |
| `StateUnknown`  | `unknown` | `... = unknown`          |
| not in map      | (absent) | `... = ""` or `... != pass` |

ACIDs not present server-side simply have no key in the context — conditions like `... = pass` return false, and `... != pass` returns true (which is usually the right route: "not yet passed, so go implement").

## Acceptance criteria

A workflow author can:

1. Write `when ctx.spec.status.foo.BAR.1 = pass` on an edge; if the acai server returned `pass` for that ACID at start, the edge fires.
2. Write `when ctx.spec.status.foo.BAR.1 != pass` on a fallback edge; the edge fires for any other state (including absent).
3. Combine multiple conditions: `when ctx.spec.status.foo.BAR.1 = pass and ctx.spec.status.foo.BAR.2 = pass`.
4. See no `node.A.spec.status.*` keys in any per-node namespace — the values are global, not scoped.
5. Run a workflow without a spec attached; `ctx.spec.status.*` references are absent (lenient evaluator returns empty string).

## What this PR is NOT

- **No engine wiring change.** PR3's call to `pullSpecStatuses` from `Engine.Run` is unchanged.
- **No new context-key names.** The key prefix `spec.status.` was introduced in PR3 and stays the same.
- **No condition evaluator changes.** Tracker's evaluator already supports `ctx.spec.status.<acid>` — only the storage side moves.

## Compatibility

- Workflows that previously read `ctx.internal.spec.status.X` continue to work — `MergeWithoutDirty` doesn't touch the internal store, so internal still returns empty (the evaluator's fall-through behavior). But since this PR is the first time a public name is offered, no real workflow has the old form yet; documenting the new convention is all that's needed.
- Existing PR3 tests assert `pctx.GetInternal("spec.status.X")`; those need to be updated to `pctx.Get("spec.status.X")` to match the new storage.

## Open questions

- **Should we keep a copy in internal too?** No — duplicating data invites drift. One source of truth.
- **Should `tracker doctor` report which ACIDs were pulled at Run start?** Useful for debugging but cosmetic. Defer to PR6.
