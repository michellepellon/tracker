# Spec Prompt Injection Design

> Status: **Design**. Implementation plan: [`docs/superpowers/plans/2026-05-22-spec-prompt-injection-implementation.md`](../plans/2026-05-22-spec-prompt-injection-implementation.md). PR4 of the 6-PR spec-first arc. Builds on PR1–PR3.

## Motivation

PR3 made tracker pull statuses at run start and push them after success. The author still has to construct their prompt from scratch — there's no automatic way for an agent to see *which requirements it's supposed to satisfy*. So the LLM has to be told the requirements either through manually-embedded prose ("Implement AUTH 1 through 4: ...") or by reading the spec file via a tool — both lossy.

This PR fixes that by exposing the resolved requirement slice as a context variable that the workflow author interpolates into the agent's prompt. The minimum useful behaviour:

1. **Before each node's handler runs**, if the node has `satisfies:`, the engine resolves every pattern against `Graph.Spec`, serializes the result, and sets two PipelineContext keys: `spec.requirements` (YAML) and `spec.requirements_json` (JSON).
2. **Authors interpolate** via tracker's existing `${ctx.spec.requirements}` syntax in the agent's `prompt:` field.
3. **Nodes without `satisfies:`** see both keys empty (lenient mode) — the interpolation is a no-op.

After this PR, the canonical spec-first agent prompt looks like:

```dip
agent ImplementAuth
  satisfies: example.AUTH.[1-4]
  prompt:
    Implement the following requirements. Reference each ACID in a code
    comment near the satisfying logic.

    ${ctx.spec.requirements}
```

The agent receives a fully-formed prompt with structured ACID context — no extra wiring on the author's part beyond writing the interpolation token.

## Non-goals (PR4 scope)

- **Auto-prepending.** PR4 does *not* automatically inject the requirements when the prompt doesn't reference them. Authors opt in by writing `${ctx.spec.requirements}` in their prompt. The "magic mode" (auto-prepend if not interpolated) is surprising and harder to reason about — defer until we see evidence we need it.
- **`ctx.spec.passed(...)` evaluator binding.** That's PR5 (a different code path — condition evaluator vs. variable interpolation).
- **`verify_acid:` declarative tool primitive.** PR5.
- **Format customization.** PR4 emits one canonical YAML shape. If authors want different formatting (Markdown, custom JSON), they can post-process via a tool node. Adding format options now is premature.
- **Filtering deprecated requirements.** Both kept and deprecated requirements appear in the output. Authors can filter via prose ("ignore deprecated entries") if they care; future PRs can add a `spec.requirements_active` variant.

## What changes

### `pipeline/spec_engine.go` — `injectSatisfiesContext` helper

A new method on `*Engine` invoked once per node before the handler runs:

```go
func (e *Engine) injectSatisfiesContext(pctx *PipelineContext, node *Node) {
    if e.graph == nil || e.graph.Spec == nil || node == nil || len(node.Satisfies) == 0 {
        return
    }
    reqs := resolveSatisfies(node.Satisfies, e.graph.Spec)
    if len(reqs) == 0 {
        return
    }
    pctx.Set("spec.requirements", marshalRequirementsYAML(reqs))
    pctx.Set("spec.requirements_json", marshalRequirementsJSON(reqs))
}
```

`resolveSatisfies` walks the patterns, calls `Graph.Spec.Resolve` on each, and deduplicates the resulting requirements by ACID. `marshalRequirementsYAML` produces stable, sorted-by-ACID YAML for deterministic LLM input.

### YAML format

```yaml
- id: example.AUTH.1
  feature: example
  component: AUTH
  number: "1"
  kind: component
  text: A caller can construct the client...
  notes:
    - hidden constraint about TLS
- id: example.AUTH.2
  feature: example
  component: AUTH
  number: "2"
  kind: component
  text: A caller can opt to send the access token...
  deprecated: true
```

Field order is fixed (id, feature, component, number, kind, text, notes, deprecated, parent) so the output is byte-stable for the same input. Empty fields are omitted (no `notes: []`, no `deprecated: false`, no `parent: ""`).

### JSON format

Mirror of the YAML — same field names, same omission rules, sorted by ACID. Available under `spec.requirements_json` for callers who want to parse the requirements inside a tool node.

### `pipeline/engine.go` — Wire into `processActiveNode`

A single line addition near the top of `processActiveNode`, after the outcome/preferred_label reset but before `prepareExecNode`:

```go
e.injectSatisfiesContext(s.pctx, node)
```

The injection runs every iteration — if a node retries, the context is refreshed (no change in this PR, but allows future variants like `spec.requirements_unfinished`).

### Author-facing behavior

Per-node, before the agent (or tool) handler runs:

| node.Satisfies         | Graph.Spec | spec.requirements | spec.requirements_json |
|------------------------|------------|-------------------|------------------------|
| empty / nil            | any        | (unset — leaves any prior value)        | (unset)                |
| set, resolves to 0     | non-nil    | (unset)           | (unset)                |
| set, resolves to ≥1    | non-nil    | YAML              | JSON                   |
| set                    | nil        | (unset — no spec to resolve against)    | (unset)                |

Lenient `${ctx.spec.requirements}` interpolation returns empty string for unset keys — workflows that interpolate without setting satisfies just get an empty interpolation.

## Acceptance criteria

A workflow author can:

1. Write `prompt: Implement these: ${ctx.spec.requirements}` on an agent with `satisfies: example.AUTH.1`; the expanded prompt contains a YAML block listing AUTH.1's full requirement text.
2. Use `${ctx.spec.requirements_json}` to get JSON instead.
3. Write a tool node that consumes the JSON via env var or stdin (subject to tool_command's existing `${ctx.*}` allowlist — `spec.requirements_json` is NOT on the allowlist, so this only works via tool nodes that read from a file populated by a prior agent step).
4. See empty string returned for both keys when the node has no `satisfies:` declared.
5. See a wildcard `satisfies: example.AUTH.*` expand to *every* AUTH requirement in the resolved YAML / JSON.

## What this PR is NOT

- **The condition evaluator doesn't learn anything new.** `when ctx.spec.passed("...")` still parses as an unknown-namespace condition. PR5.
- **The codergen handler doesn't know about spec.** All injection happens *before* the handler is invoked, so the handler sees a fully-expanded prompt and stays oblivious.
- **No CLI surface change.** `tracker doctor` doesn't report whether prompts interpolate the variable. PR5 or PR6.

## Compatibility

- Workflows without `satisfies:` are unaffected — the injection helper bails on empty input.
- Workflows that interpolate `${ctx.spec.requirements}` but don't declare `satisfies:` get an empty interpolation (lenient mode default).
- The reserved `spec.requirements` and `spec.requirements_json` context keys are documented; future tracker versions should avoid colliding with them. (Existing `writes:` validation rejects collisions with reserved engine keys; we may want to extend that allowlist in a follow-up, but the keys here are namespaced enough that natural collisions are very unlikely.)

## Tool-command safety

`spec.requirements` and `spec.requirements_json` are LLM-origin content (they're derived from spec file contents written by humans, but they flow alongside other untrusted text). They are NOT on tracker's tool_command safe-key allowlist. A `tool` node that writes `command: echo ${ctx.spec.requirements}` will be rejected by the existing tool_command guard (DIP124 + runtime check). That's the desired behaviour — spec text into shell is a footgun.

## Open questions

- **Should the injection refresh on retry?** Current design: yes (re-injected every `processActiveNode` iteration). This is correct for "requirements don't change between retries" but means we re-serialize identical YAML each time. Probably fine; revisit if profiling shows hot.
- **Notes ordering.** Multiple `<N>-note:` entries on a single requirement preserve their declared order via the acai loader. Documenting this in the spec doc for consumers.
