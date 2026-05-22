// ABOUTME: Reporter interface for bidirectional spec status I/O — pull existing status, push new updates.
// ABOUTME: Format-agnostic; concrete implementations live in sibling sub-packages and register via init().

// Package reporter defines the bridge between tracker's engine and an external
// spec status server. A Reporter exposes two directions:
//
//	Pull — fetch current ACID statuses at workflow start (so the engine can
//	       skip work that's already done on a previous run).
//	Push — write status updates as nodes complete (so the dashboard reflects
//	       truth without a human ferrying state).
//
// Reporters register themselves with the process-level Registry at package
// init; callers resolve a reporter by name via Lookup. The interface is
// intentionally minimal: there is no batching, no retry, and no caching at
// the Reporter level — those concerns belong to the engine, which composes
// the Reporter into whatever policy a given workflow needs.
//
// This package ships in PR2 of the spec-first workflow arc. The engine wires
// up to Reporter in PR3. See
// docs/superpowers/specs/2026-05-22-spec-reporter-design.md for design
// rationale.
package reporter

import (
	"context"
	"errors"
)

// ErrUnavailable is returned (or wrapped) by Reporter calls when the reporter
// is not configured for the current environment (e.g. missing binary, missing
// auth token). Callers should treat it as "this reporter is not in play"
// rather than as a fault — the engine logs and continues.
var ErrUnavailable = errors.New("reporter unavailable")

// Reporter is the bidirectional bridge between tracker and a spec status
// server.
type Reporter interface {
	// Name is the registration key (e.g. "acai"). Must match what callers
	// pass to Lookup.
	Name() string

	// Available reports whether the reporter is configured and reachable
	// for the given context. False means callers should expect Pull / Push
	// to behave as a no-op (or return ErrUnavailable for Push). Available
	// must not panic and must not block beyond a reasonable per-call
	// timeout — typical impls probe locally (PATH lookup, env var check).
	Available(ctx context.Context) bool

	// Pull fetches current ACID statuses for the given target. Returns a
	// map keyed by full ACID. An empty map with nil error means "feature
	// unknown server-side" — treat as fresh start. A non-nil error means
	// the reporter could not reach the server; callers typically log and
	// continue with an empty status set rather than aborting.
	Pull(ctx context.Context, target Target) (map[string]Status, error)

	// Push writes a batch of status updates to the server. Implementations
	// perform exactly one transport call per invocation — callers are
	// responsible for batching upstream if they want fewer round trips.
	// On Available()=false, Push returns ErrUnavailable.
	Push(ctx context.Context, target Target, updates []Status) error
}

// Target identifies the destination slot for a status update.
//
// Feature is the acai feature name (the leading ACID segment, e.g.
// "cognitoforms-py"). Product is the owning product (often the same as
// Feature for simple projects). Implementation is the named slot — typically
// the git branch name — that distinguishes parallel work tracks on the
// same feature.
type Target struct {
	Feature        string
	Product        string
	Implementation string
}

// Status is a single ACID's reported state.
type Status struct {
	// ACID is the full requirement identifier, e.g. "cognitoforms-py.AUTH.1".
	ACID string

	// State is the lifecycle position. StateUnknown is the zero value and
	// is treated as "no information" by the engine — equivalent to absent.
	State State

	// Comment is a free-text annotation. The convention (set by tracker's
	// engine in PR3) is "file:line" evidence pointing at the code or test
	// that satisfies the requirement. Reporters preserve whatever string
	// callers provide.
	Comment string

	// Refs is an optional list of source-code references — file paths or
	// "file:line" strings — that prove the requirement is covered. Acai's
	// server can ingest these to populate per-ACID detail in the dashboard.
	Refs []string
}

// State is the lifecycle position of a single requirement.
type State int

const (
	// StateUnknown is the zero value. Equivalent to "no status reported."
	StateUnknown State = iota
	// StatePending means work has been claimed but not yet completed.
	StatePending
	// StatePass means the requirement is satisfied — at least one code or
	// test reference exists that proves it.
	StatePass
	// StateFail means the requirement was attempted and failed verification.
	StateFail
	// StateBlocked means progress is blocked on an external dependency.
	StateBlocked
)

// String returns the lower-case state name suitable for logs and JSON.
func (s State) String() string {
	switch s {
	case StatePending:
		return "pending"
	case StatePass:
		return "pass"
	case StateFail:
		return "fail"
	case StateBlocked:
		return "blocked"
	default:
		return "unknown"
	}
}
