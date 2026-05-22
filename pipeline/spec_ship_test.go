// ABOUTME: End-to-end load test for examples/ship_acai_spec.dip — proves the template workflow composes against the cognitoforms-py spec.
// ABOUTME: Exercises spec attach, satisfies/verify_acid resolution, and graph build without invoking any handler.
package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestShipAcaiSpec_LoadsCleanAgainstCognitoformsPy is the load-bearing
// integration test for the spec-first arc. If anything in PR1–PR5.5 broke
// composition, this test catches it: load the canonical template against a
// real acai feature.yaml, confirm the spec attaches, confirm both
// satisfies and verify_acid resolve to ≥1 requirement, confirm there are
// no fatal validation errors and no TRK-SAT / TRK-VAC warnings.
func TestShipAcaiSpec_LoadsCleanAgainstCognitoformsPy(t *testing.T) {
	dipPath := filepath.Join("..", "examples", "ship_acai_spec.dip")
	data, err := os.ReadFile(dipPath)
	if err != nil {
		t.Fatalf("read %s: %v", dipPath, err)
	}

	graph, diags, err := LoadDippinWorkflow(string(data), dipPath)
	if err != nil {
		for _, d := range diags {
			t.Logf("diagnostic: %s", d.String())
		}
		t.Fatalf("LoadDippinWorkflow: %v", err)
	}

	if graph.Spec == nil {
		t.Fatalf("Graph.Spec is nil — expected loaded cognitoforms-py spec")
	}
	if graph.Spec.Name() != "cognitoforms-py" {
		t.Errorf("Spec.Name = %q, want cognitoforms-py", graph.Spec.Name())
	}

	implement, ok := graph.Nodes["Implement"]
	if !ok {
		t.Fatalf("Implement node not found")
	}
	if len(implement.Satisfies) == 0 {
		t.Fatalf("Implement.Satisfies is empty")
	}
	// AUTH.* alone should resolve to at least 4 requirements (per
	// cognitoforms-py/features.yaml).
	if got := len(graph.Spec.Resolve("cognitoforms-py.AUTH.*")); got < 4 {
		t.Errorf("AUTH.* resolved to %d, want >=4", got)
	}

	verify, ok := graph.Nodes["Verify"]
	if !ok {
		t.Fatalf("Verify node not found")
	}
	if len(verify.VerifyACID) == 0 {
		t.Fatalf("Verify.VerifyACID is empty")
	}

	for _, w := range graph.LintWarnings {
		// Empty wildcards/ranges would surface here.
		if strings.Contains(w, "TRK-SAT") || strings.Contains(w, "TRK-VAC") {
			t.Errorf("unexpected resolution warning: %s", w)
		}
	}
}

// TestShipAcaiSpec_EdgesRouteOnCoverage confirms the conditional edges
// referencing ctx.spec.coverage.* are present and serialized correctly
// (i.e. the dippin condition Raw text survives the adapter unchanged).
func TestShipAcaiSpec_EdgesRouteOnCoverage(t *testing.T) {
	dipPath := filepath.Join("..", "examples", "ship_acai_spec.dip")
	data, err := os.ReadFile(dipPath)
	if err != nil {
		t.Fatalf("read %s: %v", dipPath, err)
	}
	graph, _, err := LoadDippinWorkflow(string(data), dipPath)
	if err != nil {
		t.Fatalf("LoadDippinWorkflow: %v", err)
	}

	var foundCovered, foundUncovered bool
	for _, e := range graph.Edges {
		if e.From != "Verify" {
			continue
		}
		switch {
		case strings.Contains(e.Condition, "ctx.spec.coverage.cognitoforms-py.AUTH.1 = covered"):
			foundCovered = true
		case strings.Contains(e.Condition, "ctx.spec.coverage.cognitoforms-py.AUTH.1 = uncovered"):
			foundUncovered = true
		}
	}
	if !foundCovered {
		t.Errorf("expected an edge from Verify guarded by ctx.spec.coverage = covered")
	}
	if !foundUncovered {
		t.Errorf("expected an edge from Verify guarded by ctx.spec.coverage = uncovered")
	}
}
