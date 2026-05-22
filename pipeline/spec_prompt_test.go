// ABOUTME: Tests for injectSatisfiesContext — pre-execution hook that populates spec.requirements / spec.requirements_json.
// ABOUTME: Includes an end-to-end test through ExpandVariables to confirm authors can interpolate the resolved YAML in prompts.
package pipeline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadSpecGraph(t *testing.T) *Graph {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "with_spec.dip"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	graph, _, err := LoadDippinWorkflow(string(data), filepath.Join("testdata", "with_spec.dip"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	return graph
}

func TestInjectSatisfies_PopulatesYAML(t *testing.T) {
	graph := loadSpecGraph(t)
	e := NewEngine(graph, NewHandlerRegistry())
	pctx := NewPipelineContext()
	e.injectSatisfiesContext(pctx, graph.Nodes["A"])

	got, ok := pctx.Get("spec.requirements")
	if !ok {
		t.Fatalf("spec.requirements not set")
	}
	for _, want := range []string{"id: example.AUTH.1", "id: example.AUTH.2", "First auth requirement.", "Second auth requirement."} {
		if !strings.Contains(got, want) {
			t.Errorf("YAML missing %q; got:\n%s", want, got)
		}
	}
}

func TestInjectSatisfies_PopulatesJSON(t *testing.T) {
	graph := loadSpecGraph(t)
	e := NewEngine(graph, NewHandlerRegistry())
	pctx := NewPipelineContext()
	e.injectSatisfiesContext(pctx, graph.Nodes["A"])

	got, ok := pctx.Get("spec.requirements_json")
	if !ok {
		t.Fatalf("spec.requirements_json not set")
	}
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("requirements_json did not parse: %v\n%s", err, got)
	}
	if len(parsed) != 2 {
		t.Fatalf("JSON has %d entries, want 2:\n%s", len(parsed), got)
	}
	ids := map[string]bool{}
	for _, entry := range parsed {
		ids[entry["id"].(string)] = true
	}
	for _, want := range []string{"example.AUTH.1", "example.AUTH.2"} {
		if !ids[want] {
			t.Errorf("JSON missing %s; got: %v", want, ids)
		}
	}
}

func TestInjectSatisfies_EmptySatisfiesClearsKeys(t *testing.T) {
	// Pre-populate the keys from a prior node, then verify a node with no
	// satisfies clears them so the previous node's content doesn't bleed in.
	graph := loadSpecGraph(t)
	e := NewEngine(graph, NewHandlerRegistry())
	pctx := NewPipelineContext()
	pctx.Set("spec.requirements", "leftover yaml")
	pctx.Set("spec.requirements_json", "leftover json")

	e.injectSatisfiesContext(pctx, graph.Nodes["B"]) // B has no satisfies

	if got, _ := pctx.Get("spec.requirements"); got != "" {
		t.Errorf("spec.requirements = %q, want empty for node with no satisfies", got)
	}
	if got, _ := pctx.Get("spec.requirements_json"); got != "" {
		t.Errorf("spec.requirements_json = %q, want empty for node with no satisfies", got)
	}
}

func TestInjectSatisfies_NoSpecAttachedClearsKeys(t *testing.T) {
	graph := NewGraph("nospec")
	graph.AddNode(&Node{ID: "X", Satisfies: []string{"any.thing.1"}})
	e := NewEngine(graph, NewHandlerRegistry())
	pctx := NewPipelineContext()
	pctx.Set("spec.requirements", "leftover")

	e.injectSatisfiesContext(pctx, graph.Nodes["X"])

	if got, _ := pctx.Get("spec.requirements"); got != "" {
		t.Errorf("spec.requirements = %q, want empty when Graph.Spec is nil", got)
	}
}

func TestInjectSatisfies_Deterministic(t *testing.T) {
	graph := loadSpecGraph(t)
	e := NewEngine(graph, NewHandlerRegistry())

	pctx1 := NewPipelineContext()
	pctx2 := NewPipelineContext()
	e.injectSatisfiesContext(pctx1, graph.Nodes["A"])
	e.injectSatisfiesContext(pctx2, graph.Nodes["A"])

	got1, _ := pctx1.Get("spec.requirements")
	got2, _ := pctx2.Get("spec.requirements")
	if got1 != got2 {
		t.Errorf("YAML serialization not deterministic:\n%s\n---\n%s", got1, got2)
	}
}

func TestInjectSatisfies_WildcardResolvesAll(t *testing.T) {
	// Construct a graph with a wildcard satisfies pattern.
	graph := loadSpecGraph(t)
	graph.Nodes["A"].Satisfies = []string{"example.AUTH.*"}
	e := NewEngine(graph, NewHandlerRegistry())
	pctx := NewPipelineContext()
	e.injectSatisfiesContext(pctx, graph.Nodes["A"])

	got, _ := pctx.Get("spec.requirements")
	for _, want := range []string{"id: example.AUTH.1", "id: example.AUTH.2"} {
		if !strings.Contains(got, want) {
			t.Errorf("wildcard YAML missing %q; got:\n%s", want, got)
		}
	}
}

// --- End-to-end through ExpandVariables ---

func TestInjectSatisfies_InterpolatesIntoPrompt(t *testing.T) {
	graph := loadSpecGraph(t)
	e := NewEngine(graph, NewHandlerRegistry())
	pctx := NewPipelineContext()
	e.injectSatisfiesContext(pctx, graph.Nodes["A"])

	tmpl := "Implement these requirements:\n${ctx.spec.requirements}\nDone."
	expanded, err := ExpandVariables(tmpl, pctx, nil, graph.Attrs, false)
	if err != nil {
		t.Fatalf("ExpandVariables: %v", err)
	}
	for _, want := range []string{"id: example.AUTH.1", "First auth requirement."} {
		if !strings.Contains(expanded, want) {
			t.Errorf("expanded prompt missing %q; got:\n%s", want, expanded)
		}
	}
}

func TestInjectSatisfies_JSONInterpolation(t *testing.T) {
	graph := loadSpecGraph(t)
	e := NewEngine(graph, NewHandlerRegistry())
	pctx := NewPipelineContext()
	e.injectSatisfiesContext(pctx, graph.Nodes["A"])

	expanded, err := ExpandVariables("${ctx.spec.requirements_json}", pctx, nil, graph.Attrs, false)
	if err != nil {
		t.Fatalf("ExpandVariables: %v", err)
	}
	// Confirm it parses as JSON.
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(expanded), &parsed); err != nil {
		t.Errorf("interpolated JSON does not parse: %v\n%s", err, expanded)
	}
}

// --- Pre-execution hook runs through processActiveNode (integration) ---

// captureHandler is a no-op handler that snapshots the value of
// spec.requirements at the moment Execute is invoked, so we can confirm
// the engine's pre-execution injection fired before handler dispatch.
// Multiple instances are registered under different names by the test.
type captureHandler struct {
	name string
	seen *map[string]string
}

func (h *captureHandler) Name() string { return h.name }
func (h *captureHandler) Execute(_ context.Context, node *Node, pctx *PipelineContext) (Outcome, error) {
	v, _ := pctx.Get("spec.requirements")
	(*h.seen)[node.ID] = v
	return Outcome{Status: OutcomeSuccess}, nil
}

func TestInjectSatisfies_RunsBeforeHandlerExecution(t *testing.T) {
	graph := loadSpecGraph(t)
	registry := NewHandlerRegistry()
	seen := map[string]string{}
	for _, name := range []string{"codergen", "start", "exit"} {
		registry.Register(&captureHandler{name: name, seen: &seen})
	}

	e := NewEngine(graph, registry)
	if _, err := e.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	seenForA := seen["A"]
	if !strings.Contains(seenForA, "id: example.AUTH.1") {
		t.Errorf("handler for node A did not see injected spec.requirements; got:\n%s", seenForA)
	}
	if got := seen["B"]; strings.Contains(got, "id: example.AUTH.1") {
		t.Errorf("handler for node B (no satisfies) should NOT see node A's spec.requirements; got:\n%s", got)
	}
}
