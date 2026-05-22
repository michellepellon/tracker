// ABOUTME: Reporter implementation that wraps the `acai` CLI via subprocess.
// ABOUTME: Registers itself with pkg/spec/reporter at init time under name "acai".

// Package acai implements a reporter.Reporter that shells out to the acai
// command-line tool. Tracker uses this to pull existing ACID statuses at
// workflow start (resume / skip-already-done) and to push status updates as
// nodes complete.
//
// The reporter does not speak HTTP directly — it invokes the acai binary,
// which is the canonical client for the acai server. This keeps tracker
// agnostic about token storage, server URL resolution, JSON envelope shape,
// and other policy that the CLI already handles.
//
// See docs/superpowers/specs/2026-05-22-spec-reporter-design.md for design
// rationale.
package acai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/2389-research/tracker/pkg/spec/reporter"
)

const (
	reporterName = "acai"
	binaryName   = "acai"
)

// missingTokenMarker is the literal stderr string the acai CLI emits when no
// ACAI_API_TOKEN is configured. We detect this string so Available() can
// return false cleanly (rather than surfacing the error to callers).
const missingTokenMarker = "Missing API bearer token configuration"

func init() {
	reporter.Register(New())
}

// commandRunner runs a command and returns its stdout/stderr/error. Injected
// for testability — production uses the default runner backed by exec.CommandContext.
type commandRunner func(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)

// lookPath looks up a binary on $PATH. Injected for testability.
type lookPath func(name string) (string, error)

// Reporter implements reporter.Reporter by wrapping the acai CLI.
type Reporter struct {
	run      commandRunner
	lookPath lookPath
}

// Option configures a Reporter at construction time.
type Option func(*Reporter)

// WithRunner overrides the subprocess runner (test seam).
func WithRunner(r commandRunner) Option {
	return func(re *Reporter) { re.run = r }
}

// WithLookPath overrides the PATH lookup (test seam).
func WithLookPath(l lookPath) Option {
	return func(re *Reporter) { re.lookPath = l }
}

// New constructs a Reporter with the default subprocess and PATH lookup,
// overridable via options.
func New(opts ...Option) *Reporter {
	r := &Reporter{
		run:      defaultRunner,
		lookPath: exec.LookPath,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Name implements reporter.Reporter.
func (*Reporter) Name() string { return reporterName }

// Available implements reporter.Reporter. Returns true when the acai binary is
// on PATH and a probe invocation exits zero.
func (r *Reporter) Available(ctx context.Context) bool {
	if _, err := r.lookPath(binaryName); err != nil {
		return false
	}
	// Probe with a minimal feature lookup; we don't care about the result —
	// only whether the CLI can authenticate against the server.
	_, stderr, err := r.run(ctx, binaryName, "feature", "--help")
	if err != nil || bytes.Contains(stderr, []byte(missingTokenMarker)) {
		return false
	}
	return true
}

// Pull implements reporter.Reporter.
func (r *Reporter) Pull(ctx context.Context, t reporter.Target) (map[string]reporter.Status, error) {
	if _, err := r.lookPath(binaryName); err != nil {
		return nil, nil
	}
	args := []string{
		"feature", t.Feature,
		"--product", t.Product,
		"--impl", t.Implementation,
		"--json",
		"--include-refs",
	}
	stdout, stderr, err := r.run(ctx, binaryName, args...)
	if err != nil {
		return nil, fmt.Errorf("acai feature failed: %s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return parsePullResponse(stdout)
}

// Push implements reporter.Reporter.
func (r *Reporter) Push(ctx context.Context, t reporter.Target, updates []reporter.Status) error {
	if len(updates) == 0 {
		return nil
	}
	if _, err := r.lookPath(binaryName); err != nil {
		return reporter.ErrUnavailable
	}
	payload, err := encodePushPayload(updates)
	if err != nil {
		return fmt.Errorf("acai: encode push payload: %w", err)
	}
	args := []string{
		"set-status", payload,
		"--product", t.Product,
		"--impl", t.Implementation,
	}
	_, stderr, err := r.run(ctx, binaryName, args...)
	if err != nil {
		return fmt.Errorf("acai set-status failed: %s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return nil
}

// --- internal helpers ---

// rawPullResponse mirrors the `acai feature --json` output we consume.
// Acai may include additional fields; encoding/json ignores them by default.
type rawPullResponse struct {
	Feature      string `json:"feature"`
	Requirements []struct {
		ACID    string   `json:"acid"`
		Status  string   `json:"status"`
		Comment string   `json:"comment"`
		Refs    []string `json:"refs"`
	} `json:"requirements"`
}

func parsePullResponse(stdout []byte) (map[string]reporter.Status, error) {
	if len(bytes.TrimSpace(stdout)) == 0 {
		return map[string]reporter.Status{}, nil
	}
	var raw rawPullResponse
	if err := json.Unmarshal(stdout, &raw); err != nil {
		return nil, fmt.Errorf("acai: parse feature response: %w", err)
	}
	out := make(map[string]reporter.Status, len(raw.Requirements))
	for _, r := range raw.Requirements {
		if r.ACID == "" {
			continue
		}
		out[r.ACID] = reporter.Status{
			ACID:    r.ACID,
			State:   parseState(r.Status),
			Comment: r.Comment,
			Refs:    r.Refs,
		}
	}
	return out, nil
}

// pushEntry is a single ACID's update in the JSON arg to `acai set-status`.
type pushEntry struct {
	ACID    string   `json:"acid"`
	Status  string   `json:"status"`
	Comment string   `json:"comment,omitempty"`
	Refs    []string `json:"refs,omitempty"`
}

func encodePushPayload(updates []reporter.Status) (string, error) {
	out := make([]pushEntry, 0, len(updates))
	for _, u := range updates {
		out = append(out, pushEntry{
			ACID:    u.ACID,
			Status:  u.State.String(),
			Comment: u.Comment,
			Refs:    u.Refs,
		})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// parseState maps acai's status string to a reporter.State. Accepts both
// short ("pass") and past-tense ("passed") forms, and treats empty as
// pending (matches acai's convention of omitting the field for unstarted
// requirements).
func parseState(s string) reporter.State {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "pass", "passed":
		return reporter.StatePass
	case "fail", "failed":
		return reporter.StateFail
	case "blocked":
		return reporter.StateBlocked
	case "pending", "":
		return reporter.StatePending
	default:
		return reporter.StateUnknown
	}
}
