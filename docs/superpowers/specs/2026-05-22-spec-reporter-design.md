# Spec Reporter Design

> Status: **Design**. Implementation plan: [`docs/superpowers/plans/2026-05-22-spec-reporter-implementation.md`](../plans/2026-05-22-spec-reporter-implementation.md). PR2 of the 6-PR spec-first arc. Builds on PR1 (`pkg/spec/`).

## Motivation

PR1 introduced the loader — tracker can now read a spec file and resolve ACIDs locally. The engine still has no way to *report* status back to the spec server, or to *learn* what's already been done on a previous run. Both ends of that channel live in this PR.

The dippin grammar (PR0) lets a node declare `satisfies: foo.BAR.1`. When the engine eventually wires this up (PR3), it will need:

1. **At workflow start** — fetch existing ACID status so the workflow can skip work that's already passed on a previous run. This is the resume story for spec-first pipelines: kill a run mid-Component, restart, pick up where you left off without re-doing finished requirements.
2. **As nodes complete** — push status updates so the acai dashboard reflects truth without any human ferrying.

This PR ships the transport. PR3 wires it into the engine.

## Non-goals (PR2 scope)

- **Engine integration.** No engine code references the reporter yet. PR3 wires it.
- **Built-in workflow.** `ship_acai_spec.dip` is PR6.
- **A reporter for any format other than acai.** The interface is generic enough to support other formats later (gherkin status, Jira, etc.), but only the acai impl ships here.

## What changes

### New package `pkg/spec/reporter/`

A `Reporter` interface plus a process-level registry, mirroring `pkg/spec/`.

```go
package reporter

// Reporter is the bidirectional bridge between tracker and a spec status
// server. Pull fetches current status (for resume / skip-already-done);
// Push sends batched updates as work completes.
type Reporter interface {
    // Name is the reporter's registration key (e.g. "acai").
    Name() string

    // Available reports whether the reporter is configured and reachable.
    // False means callers should fall back to no-op behaviour without
    // surfacing an error — the absence of configuration is expected.
    Available(ctx context.Context) bool

    // Pull fetches current statuses for the given feature.
    // Returns map keyed by full ACID. An empty map (not error) means
    // "feature unknown server-side" — treat as fresh start.
    Pull(ctx context.Context, target Target) (map[string]Status, error)

    // Push writes a batch of status updates. Implementations may batch,
    // retry, or buffer internally. Push must never block engine progress
    // beyond a reasonable per-call timeout — failures are logged and
    // returned, never panicked.
    Push(ctx context.Context, target Target, updates []Status) error
}

// Target identifies the destination (Feature + Implementation slot).
type Target struct {
    Feature        string // e.g. "cognitoforms-py"
    Product        string // e.g. "cognitoforms-py"
    Implementation string // e.g. "main" (typically the git branch)
}

// Status is a single ACID's reported state.
type Status struct {
    ACID    string   // Full ACID, e.g. "cognitoforms-py.AUTH.1"
    State   State    // Pass / Fail / Blocked / Pending / Unknown
    Comment string   // Free-text; conventionally "file:line" evidence
    Refs    []string // Optional source-code references
}

// State is the lifecycle position of a single requirement.
type State int

const (
    StateUnknown State = iota
    StatePending
    StatePass
    StateFail
    StateBlocked
)

func (s State) String() string
```

The registry surface mirrors `pkg/spec/`:

```go
func Register(r Reporter)
func Lookup(name string) (Reporter, bool)
func Registered() []string
```

### New package `pkg/spec/reporter/acai/`

The first `Reporter` implementation. Shells out to the `acai` CLI rather than re-implementing the acai HTTP API client — for three reasons:

1. **The CLI is the canonical client.** Anything the CLI does correctly (auth header construction, server URL resolution, JSON envelope formatting) we get for free.
2. **It's already authorized by the team.** Tracker doesn't need to learn about `ACAI_API_TOKEN` or know what the production server URL is — `acai` already handles both.
3. **Lower surface area.** No HTTP client, no token plumbing, no retry policy to design here. The CLI is the contract.

The reporter is testable via a `commandRunner` function that can be injected for unit tests:

```go
type commandRunner func(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)
```

Default runner uses `exec.CommandContext`. Tests inject a fake that captures args + returns canned output.

#### `Available(ctx)` behavior

Returns `true` when:
- The `acai` binary is on `$PATH` (`exec.LookPath("acai")` succeeds), AND
- Running `acai feature <Target.Feature> --product <p> --impl <i> --json` exits 0.

The second check confirms the token is configured AND the server is reachable AND the feature exists. False on any of those means the engine should treat the reporter as a no-op (still safe to call Push/Pull — they'll just no-op too).

#### `Pull` behavior

Shells out to:
```
acai feature <feature> --product <product> --impl <impl> --json --include-refs
```

Parses the JSON output into `map[acid]Status`. Maps acai's status strings to `State`:
- `"pass"` / `"passed"` → `StatePass`
- `"fail"` / `"failed"` → `StateFail`
- `"blocked"` → `StateBlocked`
- `"pending"` / `""` → `StatePending`
- anything else → `StateUnknown`

Empty result is *not* an error — it means "this feature isn't known to the server yet."

#### `Push` behavior

Shells out to:
```
acai set-status '<json>' --product <product> --impl <impl>
```

The JSON arg is built from the `[]Status` slice. Format (best guess based on the acai CLI help text; PR3 may need to adjust once we observe a real server response):

```json
[
  {"acid": "feature.AUTH.1", "status": "pass", "comment": "src/auth.py:42"},
  {"acid": "feature.AUTH.2", "status": "fail", "comment": "tests/test_auth.py:18: assertion failed"}
]
```

The reporter does no batching itself in PR2 — callers (PR3 engine integration) are expected to batch. The reporter's job is one push call per invocation, surfacing any subprocess error to the caller.

#### Failure-mode policy

- Binary missing → `Available` false; `Pull` returns empty + nil error (treated as no-op); `Push` returns a sentinel `ErrUnavailable`.
- Token missing → `Available` false (CLI returns `"Missing API bearer token configuration."` on stderr; we detect that string); same downstream behaviour.
- Server unreachable → `Pull` and `Push` return a non-nil error containing the CLI's stderr. The engine (PR3) is expected to log + continue, not abort.
- Garbled JSON from CLI → `Pull` returns the parse error; rare and worth surfacing.

## Acceptance criteria

A caller can:

1. Import `github.com/2389-research/tracker/pkg/spec/reporter` plus the acai sub-package and call `reporter.Lookup("acai")` to get a registered Reporter.
2. Call `r.Available(ctx)` to detect whether the reporter is usable; receive `false` (with no panic) when `ACAI_API_TOKEN` is unset.
3. Call `r.Pull(ctx, target)` against a fake CLI runner and receive the parsed `map[acid]Status`.
4. Call `r.Push(ctx, target, updates)` against a fake CLI runner and observe the exact CLI args invoked.
5. See `ErrUnavailable` returned (not panicked) when the binary isn't on PATH.
6. Run a real integration test (`go test -tags=integration ./pkg/spec/reporter/acai`) that exercises the live `acai` binary against the live server — skipped automatically when `ACAI_API_TOKEN` is absent.

## What this PR is NOT

- **Engine wired-in.** No tracker code outside `pkg/spec/reporter/` calls into the reporter. The engine integration is PR3.
- **Batched.** PR2 keeps batching simple: one Push call → one subprocess invocation. PR3 may add coalescing on the engine side.
- **Retry policy.** Failures are surfaced to callers; the engine decides retry semantics in PR3.
- **Multi-loader.** Only the acai reporter ships here.

Zero diff outside `pkg/spec/reporter/`, `CHANGELOG.md`, and `docs/superpowers/`.

## Open questions

- **JSON format for `set-status`.** I couldn't verify the exact field names without a live server. Best-guess shape `{acid, status, comment}` matches CLI help text; PR3 will adjust if a real server response shows different field names. Encapsulating this in `pkg/spec/reporter/acai/` means changes are localized.
- **Should we cache `Pull` results?** Probably yes (one Pull at engine start, results consulted many times). But that's an engine concern — the reporter returns a fresh map each time. PR3 can cache externally.
- **Should `Push` support buffering across calls?** Same answer: PR3 can wrap with a buffering layer if needed. The reporter stays simple.
