package pipeline

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/2389-research/dippin-lang/ir"
	"github.com/2389-research/dippin-lang/parser"
	"github.com/2389-research/dippin-lang/validator"

	"github.com/2389-research/tracker/pkg/spec"
)

// LoadDippinWorkflow parses a dippin-lang source, then delegates to
// LoadDippinWorkflowFromIR for validation, lint, and conversion to a Graph.
// This ensures all .dip entry points apply consistent validation semantics.
//
// filename is used for error messages (e.g., "inline.dip" or "/path/to/file.dip").
// Returns the graph and any validation/lint diagnostics (warnings only).
// Validation errors are returned as fatal errors.
func LoadDippinWorkflow(source, filename string) (*Graph, []validator.Diagnostic, error) {
	workflow, err := parser.NewParser(source, filename).Parse()
	if err != nil {
		return nil, nil, fmt.Errorf("parse Dippin file: %w", err)
	}
	return LoadDippinWorkflowFromIR(workflow, filename)
}

// LoadDippinWorkflowFromIR runs dippin's structural validator (DIP001–DIP009)
// and linter (DIP101–DIP115) on an already-parsed IR workflow, then converts
// it to tracker's Graph representation and marks it dippin-validated.
//
// Diagnostics returned cover both validate and lint passes; validation errors
// are fatal, lint warnings are non-fatal. This function exists separately from
// LoadDippinWorkflow so that callers which already hold a parsed *ir.Workflow
// (e.g., the .dipx bundle loader, which gets one back from dipx.Open) can
// reuse the validate/lint/convert tail without re-parsing source.
func LoadDippinWorkflowFromIR(workflow *ir.Workflow, filename string) (*Graph, []validator.Diagnostic, error) {
	if workflow == nil {
		return nil, nil, fmt.Errorf("nil workflow for %s", filename)
	}

	// Run Dippin structural validation (DIP001–DIP009).
	valResult := validator.Validate(workflow)
	if valResult.HasErrors() {
		return nil, valResult.Diagnostics, fmt.Errorf("%d validation error(s) in %s", len(valResult.Errors()), filename)
	}

	// Run Dippin lint checks (DIP101+). Warnings only — don't block.
	// dippin-lang is the single source of truth for DIP-coded lint; tracker
	// no longer maintains a parallel implementation.
	lintResult := validator.Lint(workflow)

	// Convert IR to tracker's Graph representation.
	graph, err := FromDippinIR(workflow)
	if err != nil {
		return nil, nil, fmt.Errorf("convert Dippin IR to graph: %w", err)
	}

	// Mark graph as already validated by dippin-lang so that tracker's
	// own validator skips redundant structural checks (DIP001–DIP009).
	graph.DippinValidated = true

	// Stash formatted lint warnings on the graph so ValidateAll/-WithLint
	// can surface them through the standard warnings channel without
	// re-running any DIP check on the tracker side.
	graph.LintWarnings = formatLintWarnings(lintResult.Diagnostics)

	// If the workflow declared a spec, resolve the loader, load the spec,
	// and validate every node's satisfies declarations against it. Failures
	// here are fatal — a workflow that references a missing loader or an
	// unknown ACID is broken and shouldn't run.
	if workflow.Spec != nil {
		if err := attachSpec(graph, workflow, filename); err != nil {
			return nil, nil, err
		}
	}

	// Return all diagnostics (both validation and lint) so callers can log them.
	var allDiags []validator.Diagnostic
	allDiags = append(allDiags, valResult.Diagnostics...)
	allDiags = append(allDiags, lintResult.Diagnostics...)

	return graph, allDiags, nil
}

// attachSpec resolves the workflow's spec.Loader, loads the document, stashes
// it on graph.Spec, and validates every node's Satisfies declarations.
// Bare-ACID misses are fatal; wildcard / range misses become warnings.
func attachSpec(graph *Graph, workflow *ir.Workflow, filename string) error {
	loader, ok := spec.Lookup(workflow.Spec.Loader)
	if !ok {
		return fmt.Errorf("unknown spec loader %q (registered: %v)",
			workflow.Spec.Loader, spec.Registered())
	}
	path := resolveSpecPath(filename, workflow.Spec.Path)
	loaded, err := loader.Load(path)
	if err != nil {
		return fmt.Errorf("load spec %s: %w", path, err)
	}
	graph.Spec = loaded
	graph.SpecLoader = workflow.Spec.Loader

	warnings, err := validateSatisfies(workflow, loaded)
	if err != nil {
		return err
	}
	graph.LintWarnings = append(graph.LintWarnings, warnings...)
	return nil
}

// resolveSpecPath returns workflow.Spec.Path relative to the directory holding
// the .dip file. Absolute paths are returned unchanged.
func resolveSpecPath(filename, specPath string) string {
	if filepath.IsAbs(specPath) {
		return specPath
	}
	dir := filepath.Dir(filename)
	return filepath.Join(dir, specPath)
}

// validateSatisfies walks every node's Satisfies entries and resolves them
// against the loaded spec. Returns warnings (for wildcard / range patterns
// that resolve to nothing) and an error (for bare ACIDs that don't exist).
func validateSatisfies(workflow *ir.Workflow, loaded spec.Spec) ([]string, error) {
	var warnings []string
	for _, n := range workflow.Nodes {
		nodeWarnings, err := validateNodeSatisfies(n.ID, n.Satisfies, loaded)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, nodeWarnings...)
	}
	return warnings, nil
}

// validateNodeSatisfies checks every Satisfies entry on a single node.
func validateNodeSatisfies(nodeID string, refs []string, loaded spec.Spec) ([]string, error) {
	var warnings []string
	for _, ref := range refs {
		warning, err := checkSatisfiesEntry(nodeID, ref, loaded)
		if err != nil {
			return nil, err
		}
		if warning != "" {
			warnings = append(warnings, warning)
		}
	}
	return warnings, nil
}

// checkSatisfiesEntry resolves a single ACID reference and reports whether
// the result is an error, a warning, or clean.
func checkSatisfiesEntry(nodeID, ref string, loaded spec.Spec) (warning string, err error) {
	matches := loaded.Resolve(ref)
	if len(matches) > 0 {
		return "", nil
	}
	if isBareACID(ref) {
		return "", fmt.Errorf("node %q satisfies unknown ACID %q", nodeID, ref)
	}
	return fmt.Sprintf("warning[TRK-SAT]: node %q satisfies %q resolves to no requirements", nodeID, ref), nil
}

// isBareACID returns true when ref is a bare ACID (no wildcard, no range) —
// i.e. a reference that must resolve to exactly one requirement.
func isBareACID(ref string) bool {
	return !strings.Contains(ref, "*") && !strings.Contains(ref, "[")
}

// formatLintWarnings returns single-line "warning[CODE]: message" strings for
// every warning-severity diagnostic. Used to seed Graph.LintWarnings so the
// "Validation Warnings" output rendered by tracker validate / simulate stays
// one-line-per-warning regardless of dippin-lang's multi-line String() form.
// Errors are not included — those are returned as a fatal error.
func formatLintWarnings(diags []validator.Diagnostic) []string {
	if len(diags) == 0 {
		return nil
	}
	out := make([]string, 0, len(diags))
	for _, d := range diags {
		if d.Severity != validator.SeverityWarning {
			continue
		}
		out = append(out, fmt.Sprintf("warning[%s]: %s", d.Code, d.Message))
	}
	return out
}
