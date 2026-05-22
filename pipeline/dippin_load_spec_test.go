// ABOUTME: Tests for LoadDippinWorkflowFromIR's spec-loading and satisfies-validation pass.
// ABOUTME: Covers happy path, unknown loader, missing file, unknown ACID, empty wildcard.
package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadWithSpec(t *testing.T, name string) (*Graph, error) {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	graph, _, err := LoadDippinWorkflow(string(data), path)
	return graph, err
}

func TestLoadDippinWorkflow_AttachesSpec(t *testing.T) {
	graph, err := loadWithSpec(t, "with_spec.dip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if graph.Spec == nil {
		t.Fatalf("Graph.Spec is nil, expected loaded spec")
	}
	if graph.Spec.Name() != "example" {
		t.Errorf("Spec.Name = %q, want example", graph.Spec.Name())
	}
}

func TestLoadDippinWorkflow_CopiesSatisfiesOntoNode(t *testing.T) {
	graph, err := loadWithSpec(t, "with_spec.dip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	node, ok := graph.Nodes["A"]
	if !ok {
		t.Fatalf("node A not found")
	}
	want := []string{"example.AUTH.1", "example.AUTH.2"}
	if len(node.Satisfies) != len(want) {
		t.Fatalf("Satisfies = %v, want %v", node.Satisfies, want)
	}
	for i, v := range want {
		if node.Satisfies[i] != v {
			t.Errorf("Satisfies[%d] = %q, want %q", i, node.Satisfies[i], v)
		}
	}
}

func TestLoadDippinWorkflow_RejectsUnknownACID(t *testing.T) {
	_, err := loadWithSpec(t, "satisfies_unknown.dip")
	if err == nil {
		t.Fatalf("expected error for unknown ACID")
	}
	if !strings.Contains(err.Error(), "example.NOPE.1") {
		t.Errorf("error %q should mention the bad ACID", err)
	}
}

func TestLoadDippinWorkflow_WarnsOnEmptyWildcard(t *testing.T) {
	graph, err := loadWithSpec(t, "satisfies_empty_wildcard.dip")
	if err != nil {
		t.Fatalf("expected wildcard miss to warn, got error: %v", err)
	}
	found := false
	for _, w := range graph.LintWarnings {
		if strings.Contains(w, "TRK-SAT") && strings.Contains(w, "example.NOPE.*") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected TRK-SAT warning for empty wildcard; got: %v", graph.LintWarnings)
	}
}

func TestLoadDippinWorkflow_RejectsUnknownLoader(t *testing.T) {
	_, err := loadWithSpec(t, "spec_unknown_loader.dip")
	if err == nil {
		t.Fatalf("expected error for unknown loader")
	}
	if !strings.Contains(err.Error(), "unknown spec loader") || !strings.Contains(err.Error(), "nope") {
		t.Errorf("error %q should call out unknown loader name", err)
	}
}

func TestLoadDippinWorkflow_RejectsMissingSpecFile(t *testing.T) {
	_, err := loadWithSpec(t, "spec_missing_file.dip")
	if err == nil {
		t.Fatalf("expected error for missing spec file")
	}
	if !strings.Contains(err.Error(), "does-not-exist.yaml") {
		t.Errorf("error %q should mention the bad path", err)
	}
}

func TestLoadDippinWorkflow_NoSpecLeavesGraphSpecNil(t *testing.T) {
	src := `workflow NoSpec
  goal: "no spec"
  start: A
  exit: B

  agent A
    prompt: do it

  agent B
    prompt: done

  edges
    A -> B
`
	graph, _, err := LoadDippinWorkflow(src, "inline.dip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if graph.Spec != nil {
		t.Errorf("Graph.Spec should be nil when workflow has no spec, got %+v", graph.Spec)
	}
}
