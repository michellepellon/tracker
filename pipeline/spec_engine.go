// ABOUTME: Engine-side glue between Graph.Spec and the spec reporter — Pull at Run start, Push after successful satisfies-bearing nodes.
// ABOUTME: Failure-mode policy is best-effort: missing reporter, server errors, and unavailable transports never block the workflow.

package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/2389-research/tracker/pkg/spec"
	"github.com/2389-research/tracker/pkg/spec/reporter"
)

// SpecStatusKeyPrefix is the PipelineContext key prefix under which the
// engine stashes ACID statuses pulled at Run start. The keys live in the
// user-visible context (writable via MergeWithoutDirty so they don't bleed
// into node-scoped namespaces). Workflow authors route on them with
// conditions like `when ctx.spec.status.foo.BAR.1 = pass`.
const SpecStatusKeyPrefix = "spec.status."

// Context keys populated by injectSatisfiesContext for nodes that declare
// `satisfies:`. Authors interpolate `${ctx.spec.requirements}` (YAML) or
// `${ctx.spec.requirements_json}` (JSON) in their prompt or other expandable
// fields to receive the resolved requirement slice.
const (
	SpecRequirementsKey     = "spec.requirements"
	SpecRequirementsJSONKey = "spec.requirements_json"
)

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
	if len(statuses) == 0 {
		return
	}
	updates := make(map[string]string, len(statuses))
	for acid, status := range statuses {
		updates[SpecStatusKeyPrefix+acid] = status.State.String()
	}
	// MergeWithoutDirty stores the keys in the user-visible context (so the
	// condition evaluator can read them as `ctx.spec.status.<acid>`) without
	// marking them dirty (so they don't bleed into per-node scoped
	// namespaces when ScopeToNode runs after each handler).
	pctx.MergeWithoutDirty(updates)
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

// injectSatisfiesContext populates two PipelineContext keys with the
// requirement slice resolved from a node's Satisfies declarations:
//
//	spec.requirements       YAML (deterministic, sorted-by-ACID)
//	spec.requirements_json  JSON (same data, easier for tool nodes to parse)
//
// Authors interpolate these via ${ctx.spec.requirements} in their prompt
// or other expandable fields. The keys are node-scoped: nodes without
// Satisfies, or workflows without a loaded spec, get both keys cleared
// to empty string so a prior node's content cannot bleed into this node's
// prompt expansion.
func (e *Engine) injectSatisfiesContext(pctx *PipelineContext, node *Node) {
	if !e.shouldInjectSatisfies(node) {
		pctx.Set(SpecRequirementsKey, "")
		pctx.Set(SpecRequirementsJSONKey, "")
		return
	}
	reqs := resolveSatisfies(node.Satisfies, e.graph.Spec)
	if len(reqs) == 0 {
		pctx.Set(SpecRequirementsKey, "")
		pctx.Set(SpecRequirementsJSONKey, "")
		return
	}
	yamlBytes, err := marshalRequirementsYAML(reqs)
	if err != nil {
		e.emitSpecWarning(fmt.Sprintf("spec requirements YAML serialization for node %q failed: %v", node.ID, err))
		return
	}
	jsonBytes, err := marshalRequirementsJSON(reqs)
	if err != nil {
		e.emitSpecWarning(fmt.Sprintf("spec requirements JSON serialization for node %q failed: %v", node.ID, err))
		return
	}
	pctx.Set(SpecRequirementsKey, string(yamlBytes))
	pctx.Set(SpecRequirementsJSONKey, string(jsonBytes))
}

// shouldInjectSatisfies reports whether the engine has the inputs needed to
// build a spec.requirements value for this node.
func (e *Engine) shouldInjectSatisfies(node *Node) bool {
	if e == nil || e.graph == nil || e.graph.Spec == nil {
		return false
	}
	if node == nil || len(node.Satisfies) == 0 {
		return false
	}
	return true
}

// resolveSatisfies walks a node's satisfies patterns, resolves each via the
// loaded spec, deduplicates by ACID, and returns the result sorted by ACID
// for deterministic output.
func resolveSatisfies(patterns []string, loaded spec.Spec) []spec.Requirement {
	seen := map[string]bool{}
	var out []spec.Requirement
	for _, pat := range patterns {
		for _, r := range loaded.Resolve(pat) {
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// requirementDoc is the serialization shape for one requirement in YAML/JSON.
// Stable field ordering and omitempty rules give us byte-deterministic output
// for the same input. Defined as a struct (not raw map) so the field order in
// the YAML output matches the order declared here.
type requirementDoc struct {
	ID         string   `json:"id" yaml:"id"`
	Feature    string   `json:"feature" yaml:"feature"`
	Component  string   `json:"component" yaml:"component"`
	Number     string   `json:"number" yaml:"number"`
	Kind       string   `json:"kind" yaml:"kind"`
	Text       string   `json:"text" yaml:"text"`
	Notes      []string `json:"notes,omitempty" yaml:"notes,omitempty"`
	Deprecated bool     `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
	Parent     string   `json:"parent,omitempty" yaml:"parent,omitempty"`
}

func docFromRequirement(r spec.Requirement) requirementDoc {
	return requirementDoc{
		ID:         r.ID,
		Feature:    r.Feature,
		Component:  r.Component,
		Number:     r.Number,
		Kind:       r.Kind.String(),
		Text:       r.Text,
		Notes:      r.Notes,
		Deprecated: r.Deprecated,
		Parent:     r.Parent,
	}
}

func marshalRequirementsYAML(reqs []spec.Requirement) ([]byte, error) {
	docs := make([]requirementDoc, len(reqs))
	for i, r := range reqs {
		docs[i] = docFromRequirement(r)
	}
	return yaml.Marshal(docs)
}

func marshalRequirementsJSON(reqs []spec.Requirement) ([]byte, error) {
	docs := make([]requirementDoc, len(reqs))
	for i, r := range reqs {
		docs[i] = docFromRequirement(r)
	}
	return json.Marshal(docs)
}
