# Spec-First Authoring

Tracker's spec-first workflow pattern uses [acai](https://acai.sh) `feature.yaml` documents as the contract between human authors and AI agents. The agent reads a structured list of requirements (ACIDs), implements them, the engine verifies coverage, and status flows back to the acai dashboard automatically.

This guide shows you how to use the built-in `ship_acai_spec` workflow.

## Prerequisites

1. **A `feature.yaml` spec** describing your project's requirements. See [acai.sh/blog/specsmaxxing](https://acai.sh/blog/specsmaxxing) for the format and philosophy.
2. **`ACAI_API_TOKEN` configured** (optional, but needed for live status dashboard reporting). Without it, the workflow still runs — coverage results land in PipelineContext and route conditions work; the reporter just no-ops.
3. **A git repo** as the working directory. The workflow's `requires: git` enforces this at startup.

## Quick start

```sh
# 1. Copy the template from the tracker source repo into your project.
#    (ship_acai_spec is not yet embedded in the binary — see Limitations.)
cp "$TRACKER_SRC/examples/ship_acai_spec.dip" ./ship_acai_spec.dip

# 2. Open ship_acai_spec.dip and edit:
#    - spec: line — point at your feature.yaml
#    - satisfies: / verify_acid: lists — replace the cognitoforms-py
#      patterns with the components your spec actually has

# 3. Validate
tracker validate ship_acai_spec.dip

# 4. Simulate (dry-run) to see the routing without LLM calls
tracker simulate ship_acai_spec.dip

# 5. Run for real
tracker ship_acai_spec.dip
```

## What the workflow does

| Phase | Node | What it does |
|-------|------|-------------|
| Start | `Implement` | Reads the spec via `${ctx.spec.requirements}`, generates code referencing each ACID in comments and test names |
| Verify | `Verify` | Greps `src/` and `tests/` for each ACID literal; populates `spec.coverage.<acid>` with `covered`/`uncovered` |
| Branch | edge | Routes to `Done` if covered, `FixGaps` if uncovered |
| Fix | `FixGaps` | Re-implements the missing ACIDs |
| Loop | edge | Returns to `Verify` (marked `restart: true` so dippin doesn't flag it as a cycle) |

After each successful node, the engine pushes `StatePass` for every ACID in that node's `satisfies:` to the acai server (via the `acai` CLI if available).

## The four building blocks

This pattern composes four mechanics from the spec-first arc. Each has its own design doc if you want to go deeper.

| Mechanic | What it does | Design doc |
|----------|--------------|-----------|
| `spec:` (workflow header) | Loads a `feature.yaml` and makes its requirements queryable on the graph | [PR1 / PR3 design](superpowers/specs/2026-05-22-spec-loader-design.md) |
| `satisfies:` (node attr) | Declares which ACIDs a node owns; engine pushes `StatePass` on success | [PR3 design](superpowers/specs/2026-05-22-spec-engine-load-design.md) |
| `${ctx.spec.requirements}` (prompt interpolation) | Injects the resolved requirement slice as YAML into the agent's prompt | [PR4 design](superpowers/specs/2026-05-22-spec-prompt-injection-design.md) |
| `verify_acid:` (tool attr) | Greps the working tree for ACID literals; populates `spec.coverage.<acid>` | [PR5.5 design](superpowers/specs/2026-05-22-spec-engine-load-design.md) (verify_acid section in CHANGELOG) |
| `ctx.spec.status.<acid>` (condition var) | Carries server-side ACID status pulled at `Run` start; route edges on it | [PR5 design](superpowers/specs/2026-05-22-spec-evaluator-design.md) |

## Failure modes and how to debug them

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `unknown spec loader "X"` at load | Typo in `spec: X path` header | Use a registered loader name (`acai` is the only one shipped today) |
| `load spec <path>: open ...: no such file` | Spec path doesn't resolve from the `.dip`'s directory | Use a path relative to where the `.dip` lives, not `cwd` |
| `node "X" satisfies unknown ACID "Y"` at load | The ACID doesn't exist in the loaded spec | Check the feature.yaml for the right component / number, or fix the typo |
| `warning[TRK-SAT]: ... satisfies "X" resolves to no requirements` | Wildcard / range covered nothing | Confirm the component name (case-sensitive) and that the spec actually has matching requirements |
| `warning[TRK-VAC]: ... verify_acid ...` | Same as above, for `verify_acid:` | Same fix |
| `${ctx.spec.requirements}` expands to empty in the agent's prompt | Node has no `satisfies:` or workflow has no `spec:` | Both must be present for the engine to populate the variable |
| Workflow runs but acai dashboard shows no updates | `ACAI_API_TOKEN` not set, or `acai` binary missing from PATH | Run `acai feature <name> --json` directly to confirm the CLI works; the reporter is best-effort and won't error if absent |
| `restart` edge flagged as cycle (DIP005) | Plain edge instead of `restart: true` | Add `restart: true` to the back-edge — see `examples/ship_acai_spec.dip` |

## Limitations (v1)

- **Not embedded as a built-in.** Unlike `build_product` etc., `ship_acai_spec` is not yet runnable via `tracker ship_acai_spec` from a downloaded binary. The reason: the embedded loader validates the workflow at startup, and the `spec:` header path doesn't resolve against the embedded FS. A future PR will either teach the embed layer to bundle spec files or make `spec:` loading lazy. For now, the template lives at `examples/ship_acai_spec.dip` in the tracker source tree; copy it manually.
- **Components are hard-coded in the template.** The `satisfies:` and `verify_acid:` lists must be written out. A future PR may add a generic component-iterating variant using `manager_loop`.
- **Verify greps the cwd.** No configurable search paths yet; if your code lives outside the working directory the scan misses it.
- **Coverage results aren't reported to the acai server.** Only the per-node-success `StatePass` push lands on the dashboard. Surfacing `spec.coverage.*` separately is a possible follow-up.
- **The grep is plain literal substring matching.** No support for AST-based discovery or "ACID mentioned but not actually exercised" detection.

For deeper context on the design and what's coming next, see `docs/superpowers/specs/2026-05-22-*` — that's where I (Claude) documented each PR's reasoning as the arc unfolded.
