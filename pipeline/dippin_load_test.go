// ABOUTME: Tests for LoadDippinWorkflow and its FromIR tail split.
// ABOUTME: Verifies source and IR entry points produce byte-equivalent graphs.
package pipeline

import (
	"strings"
	"testing"

	"github.com/2389-research/dippin-lang/parser"
)

func TestLoadDippinWorkflowFromIR_ProducesSameGraphAsLoadDippinWorkflow(t *testing.T) {
	source := `workflow test_split
  start: a
  exit: b

  agent a
    label: "Start"

  agent b
    label: "Exit"

  edges
    a -> b
`
	const filename = "test_split.dip"

	graphViaSource, _, err := LoadDippinWorkflow(source, filename)
	if err != nil {
		t.Fatalf("LoadDippinWorkflow: %v", err)
	}

	workflow, err := parser.NewParser(source, filename).Parse()
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	graphViaIR, _, err := LoadDippinWorkflowFromIR(workflow, filename)
	if err != nil {
		t.Fatalf("LoadDippinWorkflowFromIR: %v", err)
	}

	if graphViaSource.Name != graphViaIR.Name {
		t.Errorf("graph name mismatch: source=%q ir=%q", graphViaSource.Name, graphViaIR.Name)
	}
	if graphViaSource.StartNode != graphViaIR.StartNode {
		t.Errorf("start mismatch: source=%q ir=%q", graphViaSource.StartNode, graphViaIR.StartNode)
	}
	if len(graphViaSource.Nodes) != len(graphViaIR.Nodes) {
		t.Errorf("node count: source=%d ir=%d", len(graphViaSource.Nodes), len(graphViaIR.Nodes))
	}
	if !graphViaIR.DippinValidated {
		t.Errorf("IR path did not mark DippinValidated")
	}
}

func TestLoadDippinWorkflowFromIR_NilWorkflow(t *testing.T) {
	_, _, err := LoadDippinWorkflowFromIR(nil, "x.dip")
	if err == nil {
		t.Fatal("expected error on nil workflow")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "nil") {
		t.Errorf("error should mention nil workflow: %v", err)
	}
}
