// ABOUTME: Unit tests for the engine's spec Pull/Push wire-up — exercises a fake reporter via the reporter registry.
// ABOUTME: Real acai subprocess is not invoked; integration coverage lives in pkg/spec/reporter/acai/integration_test.go.
package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/2389-research/tracker/pkg/spec/reporter"
)

// fakeReporter is a recording reporter used to verify Pull / Push call sites
// without invoking the acai subprocess. Registered under a unique name per
// test so parallel tests don't collide.
type fakeReporter struct {
	mu          sync.Mutex
	name        string
	available   bool
	pullReturn  map[string]reporter.Status
	pullErr     error
	pushErr     error
	pullCalls   int
	pushCalls   int
	lastPushes  [][]reporter.Status
	lastTargets []reporter.Target
}

func (f *fakeReporter) Name() string                     { return f.name }
func (f *fakeReporter) Available(_ context.Context) bool { return f.available }
func (f *fakeReporter) Pull(_ context.Context, t reporter.Target) (map[string]reporter.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pullCalls++
	f.lastTargets = append(f.lastTargets, t)
	return f.pullReturn, f.pullErr
}
func (f *fakeReporter) Push(_ context.Context, t reporter.Target, updates []reporter.Status) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pushCalls++
	f.lastTargets = append(f.lastTargets, t)
	cp := make([]reporter.Status, len(updates))
	copy(cp, updates)
	f.lastPushes = append(f.lastPushes, cp)
	return f.pushErr
}

func registerFake(t *testing.T, name string, fr *fakeReporter) {
	t.Helper()
	fr.name = name
	reporter.Register(fr)
}

func loadGraphWithSpecAndOverrideLoader(t *testing.T, fakeName string) *Graph {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "with_spec.dip"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	graph, _, err := LoadDippinWorkflow(string(data), filepath.Join("testdata", "with_spec.dip"))
	if err != nil {
		t.Fatalf("load workflow: %v", err)
	}
	// Repoint the graph's reporter selector at the test's fake reporter,
	// which was registered under a unique name. The spec itself is the
	// real acai-format yaml — we only swap who handles transport.
	graph.SpecLoader = fakeName
	return graph
}

// --- pullSpecStatuses ---

func TestPullSpecStatuses_PopulatesPipelineContext(t *testing.T) {
	fake := &fakeReporter{
		available: true,
		pullReturn: map[string]reporter.Status{
			"example.AUTH.1": {ACID: "example.AUTH.1", State: reporter.StatePass},
			"example.AUTH.2": {ACID: "example.AUTH.2", State: reporter.StateFail},
		},
	}
	registerFake(t, "fake-pull-populates", fake)

	graph := loadGraphWithSpecAndOverrideLoader(t, "fake-pull-populates")
	e := NewEngine(graph, NewHandlerRegistry())
	pctx := NewPipelineContext()
	e.pullSpecStatuses(context.Background(), pctx)

	if fake.pullCalls != 1 {
		t.Errorf("Pull called %d times, want 1", fake.pullCalls)
	}
	got, _ := pctx.GetInternal("spec.status.example.AUTH.1")
	if got != "pass" {
		t.Errorf("ctx[spec.status.example.AUTH.1] = %q, want pass", got)
	}
	got2, _ := pctx.GetInternal("spec.status.example.AUTH.2")
	if got2 != "fail" {
		t.Errorf("ctx[spec.status.example.AUTH.2] = %q, want fail", got2)
	}
}

func TestPullSpecStatuses_NoSpecIsNoOp(t *testing.T) {
	// Graph with no spec attached — pull should never invoke any reporter.
	graph := NewGraph("nospec")
	e := NewEngine(graph, NewHandlerRegistry())
	pctx := NewPipelineContext()
	e.pullSpecStatuses(context.Background(), pctx) // must not panic
}

func TestPullSpecStatuses_ReporterUnavailable(t *testing.T) {
	fake := &fakeReporter{available: false}
	registerFake(t, "fake-pull-unavail", fake)

	graph := loadGraphWithSpecAndOverrideLoader(t, "fake-pull-unavail")
	e := NewEngine(graph, NewHandlerRegistry())
	pctx := NewPipelineContext()
	e.pullSpecStatuses(context.Background(), pctx)

	if fake.pullCalls != 0 {
		t.Errorf("Pull called %d times on unavailable reporter, want 0", fake.pullCalls)
	}
}

func TestPullSpecStatuses_TransportErrorContinues(t *testing.T) {
	fake := &fakeReporter{
		available: true,
		pullErr:   errors.New("network down"),
	}
	registerFake(t, "fake-pull-err", fake)

	graph := loadGraphWithSpecAndOverrideLoader(t, "fake-pull-err")
	e := NewEngine(graph, NewHandlerRegistry())
	pctx := NewPipelineContext()
	e.pullSpecStatuses(context.Background(), pctx) // must not panic, must not error
}

// --- pushNodeSuccess ---

func TestPushNodeSuccess_SendsResolvedACIDs(t *testing.T) {
	fake := &fakeReporter{available: true}
	registerFake(t, "fake-push-resolved", fake)

	graph := loadGraphWithSpecAndOverrideLoader(t, "fake-push-resolved")
	e := NewEngine(graph, NewHandlerRegistry())

	node := graph.Nodes["A"]
	if node == nil {
		t.Fatalf("node A not found")
	}
	e.pushNodeSuccess(context.Background(), node)

	if fake.pushCalls != 1 {
		t.Fatalf("Push called %d times, want 1", fake.pushCalls)
	}
	gotACIDs := map[string]bool{}
	for _, u := range fake.lastPushes[0] {
		gotACIDs[u.ACID] = true
		if u.State != reporter.StatePass {
			t.Errorf("update %s state = %v, want StatePass", u.ACID, u.State)
		}
		if u.Comment != "node:A" {
			t.Errorf("update %s comment = %q, want node:A", u.ACID, u.Comment)
		}
	}
	for _, want := range []string{"example.AUTH.1", "example.AUTH.2"} {
		if !gotACIDs[want] {
			t.Errorf("Push missing ACID %q; got %v", want, gotACIDs)
		}
	}
}

func TestPushNodeSuccess_EmptySatisfiesIsNoOp(t *testing.T) {
	fake := &fakeReporter{available: true}
	registerFake(t, "fake-push-empty", fake)

	graph := loadGraphWithSpecAndOverrideLoader(t, "fake-push-empty")
	e := NewEngine(graph, NewHandlerRegistry())

	node := graph.Nodes["B"] // B has no satisfies in the fixture
	e.pushNodeSuccess(context.Background(), node)

	if fake.pushCalls != 0 {
		t.Errorf("Push called %d times on node with no satisfies, want 0", fake.pushCalls)
	}
}

func TestPushNodeSuccess_ReporterUnavailableIsNoOp(t *testing.T) {
	fake := &fakeReporter{available: false}
	registerFake(t, "fake-push-unavail", fake)

	graph := loadGraphWithSpecAndOverrideLoader(t, "fake-push-unavail")
	e := NewEngine(graph, NewHandlerRegistry())

	e.pushNodeSuccess(context.Background(), graph.Nodes["A"])
	if fake.pushCalls != 0 {
		t.Errorf("Push called %d times on unavailable reporter, want 0", fake.pushCalls)
	}
}

func TestPushNodeSuccess_TransportErrorContinues(t *testing.T) {
	fake := &fakeReporter{available: true, pushErr: errors.New("network down")}
	registerFake(t, "fake-push-err", fake)

	graph := loadGraphWithSpecAndOverrideLoader(t, "fake-push-err")
	e := NewEngine(graph, NewHandlerRegistry())

	// Should not panic, should not propagate the error.
	e.pushNodeSuccess(context.Background(), graph.Nodes["A"])
	if fake.pushCalls != 1 {
		t.Errorf("Push called %d times, want 1 (error from transport, not skipped)", fake.pushCalls)
	}
}

// --- resolveReporter / Target ---

func TestResolveReporter_TargetCarriesFeatureAndImpl(t *testing.T) {
	fake := &fakeReporter{available: true}
	registerFake(t, "fake-target", fake)

	graph := loadGraphWithSpecAndOverrideLoader(t, "fake-target")
	e := NewEngine(graph, NewHandlerRegistry())

	_, target, ok := e.resolveReporter()
	if !ok {
		t.Fatalf("resolveReporter returned ok=false")
	}
	if target.Feature != "example" {
		t.Errorf("Feature = %q, want example", target.Feature)
	}
	if target.Product != "example" {
		t.Errorf("Product = %q, want example", target.Product)
	}
	if target.Implementation == "" {
		t.Errorf("Implementation is empty; expected git branch or 'unknown'")
	}
}

func TestResolveReporter_NoSpec(t *testing.T) {
	e := &Engine{graph: NewGraph("nospec")}
	if _, _, ok := e.resolveReporter(); ok {
		t.Errorf("resolveReporter ok=true on graph with no spec, want false")
	}
}

func TestResolveReporter_UnknownReporter(t *testing.T) {
	graph := loadGraphWithSpecAndOverrideLoader(t, "absolutely-not-registered-xyz")
	e := NewEngine(graph, NewHandlerRegistry())
	if _, _, ok := e.resolveReporter(); ok {
		t.Errorf("resolveReporter ok=true on unknown loader, want false")
	}
}
