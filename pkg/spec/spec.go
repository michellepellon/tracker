// ABOUTME: Spec interface and Requirement type — the data layer for spec-first workflow authoring in tracker.
// ABOUTME: Format-agnostic; concrete formats (acai, gherkin, …) live in sibling sub-packages and register via init().

// Package spec defines the interfaces tracker uses to load and query external
// spec documents referenced by a workflow's `spec: <loader> <path>` header.
//
// A Loader reads a single spec file from disk and exposes its requirements via
// the Spec interface. Each requirement carries a stable identifier (ACID, per
// the dippin grammar) that workflow nodes reference in their `satisfies:`
// lists. Tracker resolves ACIDs to requirements at engine start and uses them
// to inject prompt context, drive verification, and report status back to the
// spec server.
//
// Loaders register themselves with the process-level Registry at init time;
// callers resolve a loader by name via Lookup.
//
// Dippin parses `spec:` and `satisfies:` but never opens a spec file — that
// concern lives here, in the runtime. This package is intentionally minimal:
// no I/O policy, no caching, no path resolution. Each Loader is free to
// implement its own. See docs/superpowers/specs/2026-05-22-spec-loader-design.md
// for the full design rationale.
package spec

// Loader reads a spec document from disk and returns a Spec.
//
// Loaders register themselves at process init via Register; callers resolve
// them by name via Lookup. The loader Name must match what workflow authors
// write in `spec: <name> <path>`.
type Loader interface {
	// Name returns the loader's registration key (e.g. "acai").
	Name() string

	// Load reads and parses the spec at path. The path is expected to have
	// been resolved by the caller (typically relative to the .dip file's
	// directory). Loaders do not perform path resolution themselves.
	Load(path string) (Spec, error)
}

// Spec is the structured view of a loaded spec document.
//
// Implementations must be safe for concurrent reads. The engine loads a spec
// once at startup and then queries it from many goroutines without further
// synchronization.
type Spec interface {
	// Name returns the spec's logical name as declared in the source (e.g.
	// the acai "feature.name" field). This is the leading segment of every
	// ACID the spec produces.
	Name() string

	// Requirements returns every requirement in declaration order.
	// Sub-requirements appear after their parent.
	Requirements() []Requirement

	// Requirement returns a single requirement by full ACID. The bool is
	// false when the ID is unknown — callers should not treat an unknown
	// ACID as an error, only as a miss.
	Requirement(acid string) (Requirement, bool)

	// Resolve expands an ACID pattern into the set of matching requirements.
	// Supported pattern shapes (matching dippin DIP139):
	//   bare:     feature.COMPONENT.N       → 0 or 1 result
	//   range:    feature.COMPONENT.[N-M]   → 0..N results (gaps skipped silently)
	//   wildcard: feature.COMPONENT.*       → every requirement in component
	// Unknown components or out-of-range values yield an empty slice; only
	// genuinely malformed patterns return nil (callers can also detect that
	// case via dippin's DIP139 at lint time).
	Resolve(pattern string) []Requirement
}

// Requirement is a single acceptance criterion drawn from a spec.
type Requirement struct {
	// ID is the full ACID, e.g. "cognitoforms-py.AUTH.1".
	ID string

	// Feature is the lowercase feature name — the leading ACID segment.
	Feature string

	// Component is the UPPERCASE component name — the middle ACID segment(s).
	// For nested components (acai allows multiple), this is the inner-most
	// component; the full chain is recoverable from ID.
	Component string

	// Number is the requirement's identifier within its component, as
	// written: "1" for a top-level requirement, "1-2" for a sub-requirement.
	Number string

	// Kind distinguishes a behavioural component requirement from a
	// constraint requirement (acai separates these; other loaders may
	// always use KindComponent).
	Kind Kind

	// Text is the requirement's body — the prose the spec author wrote.
	Text string

	// Notes carries supplementary annotations attached to the requirement
	// (in acai, via the "<N>-note:" sibling key).
	Notes []string

	// Deprecated reflects whether the source marked the requirement as
	// retired. Deprecated requirements still appear in Requirements() and
	// resolve normally, so callers can audit them — they just typically
	// shouldn't be counted toward coverage.
	Deprecated bool

	// Parent is the parent ACID for sub-requirements (e.g.
	// "cognitoforms-py.AUTH.1" for sub-requirement "cognitoforms-py.AUTH.1-1").
	// Empty for top-level requirements.
	Parent string

	// Raw is the loader-specific source representation, opaque to callers.
	// Used by the acai reporter to round-trip server-side hints; ignore for
	// generic consumption.
	Raw any
}

// Kind distinguishes a behavioural requirement from a cross-cutting constraint.
//
// acai's feature.yaml uses two top-level keys (`components:` and
// `constraints:`); other spec formats may flatten this distinction.
type Kind int

const (
	// KindComponent marks a behavioural acceptance criterion — the default.
	KindComponent Kind = iota
	// KindConstraint marks a cross-cutting requirement (packaging, observability,
	// testing discipline, etc.).
	KindConstraint
)

// String returns "component" or "constraint" for human-readable output.
func (k Kind) String() string {
	switch k {
	case KindConstraint:
		return "constraint"
	default:
		return "component"
	}
}
