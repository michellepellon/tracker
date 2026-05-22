// ABOUTME: Tests for verify_acid — load-time validation, post-execution grep, spec.coverage.* keys.
// ABOUTME: Uses real filesystem scanning via t.TempDir so the grep machinery is exercised end-to-end.
package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- scanWorkingTreeForLiterals: filesystem scanner ---

func TestScanWorkingTree_FindsLiteralInFile(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "src", "auth.py"), "# example.AUTH.1\nfoo()\n")
	mustWriteFile(t, filepath.Join(dir, "tests", "test_auth.py"), "# example.AUTH.2\nbar()\n")

	chdir(t, dir)
	got, err := scanWorkingTreeForLiterals([]string{"example.AUTH.1", "example.AUTH.2", "example.AUTH.3"})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !got["example.AUTH.1"] {
		t.Errorf("example.AUTH.1 should be found")
	}
	if !got["example.AUTH.2"] {
		t.Errorf("example.AUTH.2 should be found")
	}
	if got["example.AUTH.3"] {
		t.Errorf("example.AUTH.3 should be missing")
	}
}

func TestScanWorkingTree_SkipsKnownNoiseDirs(t *testing.T) {
	dir := t.TempDir()
	// Plant the literal in .git/, which should be skipped.
	mustWriteFile(t, filepath.Join(dir, ".git", "objects", "leak"), "example.AUTH.1\n")
	// Plant a NOT-IN-CODE acid in a regular file to confirm the scan ran.
	mustWriteFile(t, filepath.Join(dir, "src", "real.go"), "// example.AUTH.2\n")

	chdir(t, dir)
	got, err := scanWorkingTreeForLiterals([]string{"example.AUTH.1", "example.AUTH.2"})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got["example.AUTH.1"] {
		t.Errorf("scan should skip .git/ but found example.AUTH.1 there")
	}
	if !got["example.AUTH.2"] {
		t.Errorf("scan should have found example.AUTH.2 in src/")
	}
}

func TestScanWorkingTree_SkipsBinaryExtensions(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "logo.png"), "example.AUTH.1\n") // .png is binary by allowlist
	mustWriteFile(t, filepath.Join(dir, "src", "real.go"), "// example.AUTH.1\n")

	chdir(t, dir)
	got, err := scanWorkingTreeForLiterals([]string{"example.AUTH.1"})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !got["example.AUTH.1"] {
		t.Errorf("example.AUTH.1 should be found in real.go even though logo.png was skipped")
	}
}

// --- verifyNodeACIDs: full engine wiring ---

func TestVerifyNodeACIDs_PopulatesCoverageKeys(t *testing.T) {
	graph := loadVerifyGraph(t)
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "src", "auth.py"), "# example.AUTH.1\n")
	chdir(t, dir)

	e := NewEngine(graph, NewHandlerRegistry())
	pctx := NewPipelineContext()
	e.verifyNodeACIDs(pctx, graph.Nodes["V"])

	if got, _ := pctx.Get("spec.coverage.example.AUTH.1"); got != "covered" {
		t.Errorf("spec.coverage.example.AUTH.1 = %q, want covered", got)
	}
	if got, _ := pctx.Get("spec.coverage.example.AUTH.2"); got != "uncovered" {
		t.Errorf("spec.coverage.example.AUTH.2 = %q, want uncovered", got)
	}
}

func TestVerifyNodeACIDs_NoVerifyACIDIsNoOp(t *testing.T) {
	graph := loadSpecGraph(t) // from spec_prompt_test.go — no VerifyACID on nodes A or B
	e := NewEngine(graph, NewHandlerRegistry())
	pctx := NewPipelineContext()
	e.verifyNodeACIDs(pctx, graph.Nodes["A"])

	if _, ok := pctx.Get("spec.coverage.example.AUTH.1"); ok {
		t.Errorf("spec.coverage.* should be unset for nodes without VerifyACID")
	}
}

func TestVerifyNodeACIDs_NoSpecIsNoOp(t *testing.T) {
	graph := NewGraph("nospec")
	graph.AddNode(&Node{ID: "X", VerifyACID: []string{"anything.X.1"}, Handler: "tool"})
	e := NewEngine(graph, NewHandlerRegistry())
	pctx := NewPipelineContext()
	e.verifyNodeACIDs(pctx, graph.Nodes["X"])

	if _, ok := pctx.Get("spec.coverage.anything.X.1"); ok {
		t.Errorf("spec.coverage.* should be unset when Graph.Spec is nil")
	}
}

func TestVerifyNodeACIDs_NoDirtyBleed(t *testing.T) {
	graph := loadVerifyGraph(t)
	dir := t.TempDir()
	chdir(t, dir)

	e := NewEngine(graph, NewHandlerRegistry())
	pctx := NewPipelineContext()
	e.verifyNodeACIDs(pctx, graph.Nodes["V"])
	pctx.Set("trigger", "scope") // any dirty write to provoke ScopeToNode
	pctx.ScopeToNode("V")

	if _, ok := pctx.Get("node.V.spec.coverage.example.AUTH.1"); ok {
		t.Errorf("spec.coverage.* bled into node scope")
	}
	if got, _ := pctx.Get("spec.coverage.example.AUTH.1"); got != "uncovered" {
		t.Errorf("global spec.coverage.example.AUTH.1 lost: %q", got)
	}
}

// --- Load-time validation of verify_acid against the loaded spec ---

func TestLoadWorkflow_VerifyACIDBareUnknownIsFatal(t *testing.T) {
	src := `workflow VerifyBad
  goal: "bad verify"
  spec: acai with_spec_features.yaml
  start: V
  exit: D

  tool V
    command: echo
    verify_acid: example.NOPE.1

  agent D
    prompt: done

  edges
    V -> D
`
	_, _, err := LoadDippinWorkflow(src, filepath.Join("testdata", "verify_bad.dip"))
	if err == nil {
		t.Fatalf("expected error for unknown verify_acid")
	}
	if !strings.Contains(err.Error(), "example.NOPE.1") {
		t.Errorf("error should mention the bad ACID: %v", err)
	}
}

func TestLoadWorkflow_VerifyACIDEmptyWildcardWarns(t *testing.T) {
	src := `workflow VerifyWild
  goal: "wildcard miss"
  spec: acai with_spec_features.yaml
  start: V
  exit: D

  tool V
    command: echo
    verify_acid: example.NOPE.*

  agent D
    prompt: done

  edges
    V -> D
`
	graph, _, err := LoadDippinWorkflow(src, filepath.Join("testdata", "verify_wild.dip"))
	if err != nil {
		t.Fatalf("wildcard miss should warn, not error: %v", err)
	}
	found := false
	for _, w := range graph.LintWarnings {
		if strings.Contains(w, "TRK-VAC") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected TRK-VAC warning; got: %v", graph.LintWarnings)
	}
}

// --- helpers ---

// loadVerifyGraph loads a workflow whose tool node V declares verify_acid
// for two of the spec's AUTH requirements.
func loadVerifyGraph(t *testing.T) *Graph {
	t.Helper()
	src := `workflow VerifyDemo
  goal: "verify_acid demo"
  spec: acai with_spec_features.yaml
  start: V
  exit: D

  tool V
    command: echo
    verify_acid: example.AUTH.1, example.AUTH.2

  agent D
    prompt: done

  edges
    V -> D
`
	graph, _, err := LoadDippinWorkflow(src, filepath.Join("testdata", "verify_demo.dip"))
	if err != nil {
		t.Fatalf("load verify_demo: %v", err)
	}
	return graph
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}
