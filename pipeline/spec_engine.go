// ABOUTME: Engine-side glue between Graph.Spec and the spec reporter — Pull at Run start, Push after successful satisfies-bearing nodes.
// ABOUTME: Failure-mode policy is best-effort: missing reporter, server errors, and unavailable transports never block the workflow.

package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/2389-research/tracker/pkg/spec"
	"github.com/2389-research/tracker/pkg/spec/reporter"
)

// SpecStatusKeyPrefix is the PipelineContext internal-key prefix under which
// the engine stashes ACID statuses pulled at Run start. Downstream features
// (PR4+: condition evaluator, prompt injection) read these via GetInternal.
const SpecStatusKeyPrefix = "spec.status."

// pullSpecStatuses calls the matching reporter's Pull (if Available) and
// seeds the PipelineContext with one internal key per ACID. Best-effort:
// missing reporter, missing token, transport errors all degrade silently.
func (e *Engine) pullSpecStatuses(ctx context.Context, pctx *PipelineContext) {
	rep, target, ok := e.resolveReporter()
	if !ok {
		return
	}
	if !rep.Available(ctx) {
		e.emitSpecWarning(fmt.Sprintf("spec reporter %q unavailable; continuing without prior status", e.graph.SpecLoader))
		return
	}
	statuses, err := rep.Pull(ctx, target)
	if err != nil {
		e.emitSpecWarning(fmt.Sprintf("spec reporter pull failed: %v", err))
		return
	}
	for acid, status := range statuses {
		pctx.SetInternal(SpecStatusKeyPrefix+acid, status.State.String())
	}
}

// pushNodeSuccess reports a node's satisfies set as StatePass to the matching
// reporter. Called after a node returns a success outcome. Best-effort:
// reporter unavailability and transport errors are logged via a warning
// event but never bubble up to the engine loop.
func (e *Engine) pushNodeSuccess(ctx context.Context, node *Node) {
	if node == nil || len(node.Satisfies) == 0 {
		return
	}
	rep, target, ok := e.resolveReporter()
	if !ok {
		return
	}
	if !rep.Available(ctx) {
		return
	}
	updates := buildPushUpdates(node, e.graph.Spec)
	if len(updates) == 0 {
		return
	}
	if err := rep.Push(ctx, target, updates); err != nil {
		e.emitSpecWarning(fmt.Sprintf("spec reporter push for node %q failed: %v", node.ID, err))
	}
}

// resolveReporter returns the reporter matching graph.SpecLoader and the
// target for this run, or ok=false when no spec is attached or no reporter
// is registered under the spec's loader name.
func (e *Engine) resolveReporter() (reporter.Reporter, reporter.Target, bool) {
	if e.graph == nil || e.graph.Spec == nil || e.graph.SpecLoader == "" {
		return nil, reporter.Target{}, false
	}
	rep, ok := reporter.Lookup(e.graph.SpecLoader)
	if !ok {
		return nil, reporter.Target{}, false
	}
	target := reporter.Target{
		Feature:        e.graph.Spec.Name(),
		Product:        e.graph.Spec.Name(),
		Implementation: currentImplementation(),
	}
	return rep, target, true
}

// buildPushUpdates expands every satisfies pattern on the node against the
// loaded spec and emits one StatusUpdate per resolved ACID. Patterns that
// resolve to nothing contribute nothing — load-time validateSatisfies
// already warned about empty wildcards / ranges.
func buildPushUpdates(node *Node, loaded spec.Spec) []reporter.Status {
	seen := map[string]bool{}
	var out []reporter.Status
	for _, ref := range node.Satisfies {
		for _, r := range loaded.Resolve(ref) {
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			out = append(out, reporter.Status{
				ACID:    r.ID,
				State:   reporter.StatePass,
				Comment: "node:" + node.ID,
			})
		}
	}
	return out
}

// currentImplementation returns the current git branch name, falling back to
// "unknown" when git is unavailable or the working directory isn't a git
// repo. The acai server uses this string to namespace status updates per
// branch / implementation slot.
func currentImplementation() string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "unknown"
	}
	branch := strings.TrimSpace(out.String())
	if branch == "" {
		return "unknown"
	}
	return branch
}

// emitSpecWarning emits a non-fatal warning event when spec I/O misbehaves.
// The engine continues execution; nothing about spec reporting is on the
// critical path.
func (e *Engine) emitSpecWarning(msg string) {
	e.emit(PipelineEvent{
		Type:    EventWarning,
		Message: msg,
	})
}
