# Spec Loader Design

> Status: **Design**. Implementation plan: [`docs/superpowers/plans/2026-05-22-spec-loader-implementation.md`](../plans/2026-05-22-spec-loader-implementation.md). PR1 of a planned 6-PR arc.

## Motivation

The dippin grammar gained `spec:` and `satisfies:` in [michellepellon/dippin-lang#1](https://github.com/michellepellon/dippin-lang/pull/1) (merged 2026-05-22). Workflows can now declare:

```dip
workflow ImplementAuth
  spec: acai features/auth/features.yaml
  ...

  agent Implement
    satisfies: auth.SESSION.[1-4], auth.TOKEN.*
```

Dippin parses and validates the shapes but is intentionally format-agnostic — it never opens `features.yaml`. The actual loading, ACID expansion, and resolution against the loaded spec belong in the consumer (tracker). This PR introduces the layer that does that.

Motivated by acai's [specsmaxxing](https://acai.sh/blog/specsmaxxing) post: as AI codegen gets cheaper, the spec becomes the primary artifact, and the toolchain should treat structured specs as a first-class input, not as opaque prompt text.

## Non-goals (PR1 scope)

- **Engine integration.** Agent prompt injection on `satisfies:`, `ctx.spec.*` evaluator bindings, and `verify_acid:` are PR3+.
- **Reporter.** Bidirectional acai server I/O (pull-at-start, push-as-you-go) is PR2.
- **CLI surface.** `tracker doctor` spec probe, `tracker validate` ACID resolution, new `tracker spec` subcommand — PR5.
- **Built-in workflow.** `ship_acai_spec.dip` is PR6.

This PR is the data layer only: an interface, a registry, and the acai implementation. No tracker-internal consumer wires up to it yet. That keeps the diff small and the abstraction reviewable in isolation.

## What changes

### New package `pkg/spec/`

A `Loader` interface, a `Spec` interface, a `Requirement` type, and a process-level `Registry`. The interface is deliberately small — load a file, expose requirements, resolve ACID patterns.

```go
package spec

// Loader reads a spec file from disk and returns a Spec.
// Loaders register themselves at process init via Register(loader).
type Loader interface {
    // Name is the key callers use to look up the loader (e.g. "acai").
    // Must match the loader name a workflow author writes in `spec: <name> <path>`.
    Name() string

    // Load reads and parses the spec at path. The path is expected to be
    // resolved relative to the .dip file's directory by the caller; the
    // loader doesn't perform path resolution itself.
    Load(path string) (Spec, error)
}

// Spec is the structured view of a loaded spec document.
type Spec interface {
    // Name returns the spec's logical name (e.g. "cognitoforms-py").
    Name() string

    // Requirements returns every requirement, in declaration order.
    Requirements() []Requirement

    // Requirement returns a single requirement by ACID. Returns
    // (Requirement{}, false) if the ID is unknown.
    Requirement(acid string) (Requirement, bool)

    // Resolve expands an ACID pattern (bare, range, wildcard) into the
    // matching set of requirements. Unknown component or out-of-range
    // values return an empty slice and no error — callers detect "no
    // match" via len(result)==0.
    Resolve(pattern string) []Requirement
}

// Requirement is a single acceptance criterion drawn from a spec.
type Requirement struct {
    ID         string   // Full ACID, e.g. "cognitoforms-py.AUTH.1"
    Feature    string   // Lowercase feature name ("cognitoforms-py")
    Component  string   // Uppercase component ("AUTH")
    Number     string   // Requirement number as written, including sub ("1", "1-2")
    Kind       Kind     // Component vs Constraint (acai distinguishes these)
    Text       string   // Body text of the requirement
    Notes      []string // Optional notes attached via "<num>-note:"
    Deprecated bool     // True when the spec marked the requirement deprecated
    Parent     string   // Empty for top-level; the parent ACID for sub-requirements
    Raw        any      // Loader-specific raw blob (for advanced inspection)
}

type Kind int

const (
    KindComponent Kind = iota
    KindConstraint
)
```

### Registry

A process-level map of loader name → Loader. Loaders register themselves in `init()` of their package. Callers use `Lookup(name)` to resolve the workflow's `spec: <name>` to a concrete loader.

```go
func Register(l Loader)
func Lookup(name string) (Loader, bool)
func Registered() []string
```

The registry is intentionally global. Workflows reference loaders by string name; we'd otherwise need to pass a registry through every API surface that touches workflows. Global registry matches the precedent set by `image.RegisterFormat` and `database/sql.Register` in the Go stdlib.

### New package `pkg/spec/acai/`

The first `Loader` implementation. Reads acai's `feature.yaml` format (per the [acai skill spec](https://acai.sh/llms.txt)):

```yaml
feature:
  name: cognitoforms-py
  product: cognitoforms-py
  description: ...

components:
  AUTH:
    requirements:
      1: A caller can construct the client...
      1-1: A sub-requirement.
      1-note: Hidden constraint about TLS.
      2:
        requirement: ...
        deprecated: true

constraints:
  PACKAGING:
    description: ...
    requirements:
      1: ...
```

Key parser responsibilities:

- Flatten `components.<NAME>.requirements.<N>` and `constraints.<NAME>.requirements.<N>` into `[]Requirement` with `Kind` set appropriately.
- Handle short-form (`1: "text"`) and long-form (`1: { requirement: "text", deprecated: true }`) requirement shapes.
- Attach `<num>-note:` siblings to the matching numbered requirement as `Notes`.
- Detect sub-requirements (`1-1`, `1-2`) and set `Parent` to the parent ACID (`feature.COMPONENT.1`).
- Surface `deprecated: true` on the resulting Requirement.
- Reject sub-sub-requirements (`1-1-1`) per the acai spec ("INVALID — keep sub-requirements 1 level deep").
- Return an `error` on malformed YAML, missing `feature.name`, or empty `components`+`constraints`.

`Resolve(pattern)` implements ACID expansion:

- `foo.BAR.1` → exact match or empty.
- `foo.BAR.*` → every requirement in component BAR.
- `foo.BAR.[1-3]` → requirements 1, 2, 3 in component BAR that actually exist (gaps are silently skipped, not errors).

Pattern syntax matches dippin's `acidPattern` regex (DIP139), so anything that lints clean in dippin is something this loader can resolve.

## Acceptance criteria

A caller can:

1. Import `github.com/2389-research/tracker/pkg/spec` and call `spec.Lookup("acai")` to get a non-nil `Loader` (the acai package is imported for side-effect registration).
2. Call `loader.Load("/path/to/features.yaml")` and receive a `Spec` whose `Requirements()` includes every requirement declared in the file, with `Deprecated`, `Notes`, and `Parent` populated where applicable.
3. Call `spec.Requirement("cognitoforms-py.AUTH.1")` and receive that single requirement; the same call with a missing ACID returns `(Requirement{}, false)`.
4. Call `spec.Resolve("cognitoforms-py.AUTH.*")` and receive every AUTH requirement in declaration order.
5. Call `spec.Resolve("cognitoforms-py.AUTH.[1-3]")` and receive AUTH.1, AUTH.2, AUTH.3 (skipping any that don't exist).
6. Load the actual `features/cognitoforms-py/features.yaml` from the cognitoforms.py project and have every requirement parse correctly, including the constraints section.
7. See a clear error when loading a malformed YAML file, a file with no `feature.name`, or a sub-sub-requirement.

## What this PR is NOT

- **A runtime integration.** No engine code references `pkg/spec` yet. That's PR3.
- **A workflow consumer.** No `.dip` file in `examples/` uses spec features yet. That's PR6.
- **An I/O integration with the acai server.** That's PR2.
- **A change to any existing handler.** Zero diff outside `pkg/spec/`, `go.mod`, `docs/`, and `CHANGELOG.md`.

The only existing files this PR modifies are `go.mod` (the dippin `replace` directive), `go.sum` (auto-updated), and `CHANGELOG.md`.

## Compatibility

- **Backwards compatibility:** Net-new package. Nothing in tracker imports it yet, so no downstream impact.
- **Dippin pin:** Uses a `replace` directive in `go.mod` pointing at `michellepellon/dippin-lang` at SHA `1e446b9` (the merge of dippin PR #1). When 2389-research/dippin-lang tags a release with the spec/satisfies grammar, this directive is removed and the require line is bumped.

## Open questions

- **Should `Spec` expose a `Components()` accessor?** Workflows typically reference whole components (`AUTH.*`); a convenience method would save callers from filtering `Requirements()`. Defer until a real consumer needs it.
- **YAML library choice.** `gopkg.in/yaml.v3` is the de-facto standard. Tracker doesn't currently depend on it. Adding it costs ~600 KB and pulls in no transitive deps not already in the graph. The alternative (rolling a hand-parser for the narrow acai shape) is worse — YAML's whitespace + anchors + flow style are not worth re-implementing.
- **Should the loader chase `feature.product` references?** acai's docs show a separate `product` field. Not used by anything yet; ignore for now and revisit when the reporter (PR2) needs it.
