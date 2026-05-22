// ABOUTME: Registry round-trip + State/Status zero-value tests for pkg/spec/reporter.
// ABOUTME: Establishes the Reporter contract every impl must satisfy.
package reporter_test

import (
	"context"
	"testing"

	"github.com/2389-research/tracker/pkg/spec/reporter"
)

type fakeReporter struct{ name string }

func (f fakeReporter) Name() string                     { return f.name }
func (f fakeReporter) Available(_ context.Context) bool { return false }
func (f fakeReporter) Pull(_ context.Context, _ reporter.Target) (map[string]reporter.Status, error) {
	return nil, nil
}
func (f fakeReporter) Push(_ context.Context, _ reporter.Target, _ []reporter.Status) error {
	return nil
}

func TestRegistry_RegisterAndLookup(t *testing.T) {
	reporter.Register(fakeReporter{name: "registry-rt-1"})
	r, ok := reporter.Lookup("registry-rt-1")
	if !ok {
		t.Fatalf("Lookup returned ok=false for registered reporter")
	}
	if r.Name() != "registry-rt-1" {
		t.Errorf("Reporter.Name = %q, want registry-rt-1", r.Name())
	}
}

func TestRegistry_LookupMissing(t *testing.T) {
	_, ok := reporter.Lookup("never-registered-zzz")
	if ok {
		t.Errorf("expected ok=false for missing reporter")
	}
}

func TestRegistry_RegisteredList(t *testing.T) {
	reporter.Register(fakeReporter{name: "list-x"})
	reporter.Register(fakeReporter{name: "list-y"})
	names := reporter.Registered()
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["list-x"] || !found["list-y"] {
		t.Errorf("Registered() = %v; missing list-x or list-y", names)
	}
}

func TestState_String(t *testing.T) {
	cases := []struct {
		s    reporter.State
		want string
	}{
		{reporter.StateUnknown, "unknown"},
		{reporter.StatePending, "pending"},
		{reporter.StatePass, "pass"},
		{reporter.StateFail, "fail"},
		{reporter.StateBlocked, "blocked"},
	}
	for _, tc := range cases {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("State(%d).String() = %q, want %q", tc.s, got, tc.want)
		}
	}
}

func TestStatus_ZeroValue(t *testing.T) {
	var s reporter.Status
	if s.ACID != "" || s.State != reporter.StateUnknown || s.Comment != "" || s.Refs != nil {
		t.Errorf("zero Status not empty: %+v", s)
	}
}

func TestErrUnavailable_NonNil(t *testing.T) {
	if reporter.ErrUnavailable == nil {
		t.Fatalf("ErrUnavailable should be a non-nil sentinel")
	}
	if reporter.ErrUnavailable.Error() == "" {
		t.Errorf("ErrUnavailable should have a non-empty message")
	}
}
