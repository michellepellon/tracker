// ABOUTME: Registry round-trip and type-level tests for pkg/spec.
// ABOUTME: Establishes the contract every Loader implementation must satisfy.
package spec_test

import (
	"testing"

	"github.com/2389-research/tracker/pkg/spec"
)

type fakeLoader struct{ name string }

func (f fakeLoader) Name() string                     { return f.name }
func (f fakeLoader) Load(_ string) (spec.Spec, error) { return nil, nil }

func TestRegistry_RegisterAndLookup(t *testing.T) {
	spec.Register(fakeLoader{name: "registry-rt-1"})
	l, ok := spec.Lookup("registry-rt-1")
	if !ok {
		t.Fatalf("Lookup returned ok=false for registered loader")
	}
	if l.Name() != "registry-rt-1" {
		t.Errorf("Loader.Name = %q, want registry-rt-1", l.Name())
	}
}

func TestRegistry_LookupMissing(t *testing.T) {
	_, ok := spec.Lookup("never-registered-xyz")
	if ok {
		t.Errorf("expected ok=false for missing loader")
	}
}

func TestRegistry_RegisteredList(t *testing.T) {
	spec.Register(fakeLoader{name: "list-a"})
	spec.Register(fakeLoader{name: "list-b"})
	names := spec.Registered()
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["list-a"] || !found["list-b"] {
		t.Errorf("Registered() = %v; missing list-a or list-b", names)
	}
}

func TestRegistry_DoubleRegistration_LastWins(t *testing.T) {
	spec.Register(fakeLoader{name: "dup"})
	spec.Register(fakeLoader{name: "dup"})
	names := spec.Registered()
	count := 0
	for _, n := range names {
		if n == "dup" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected one entry for duplicate registration, got %d in %v", count, names)
	}
}

func TestRequirement_ZeroValue(t *testing.T) {
	var r spec.Requirement
	if r.ID != "" {
		t.Errorf("zero ID = %q, want empty", r.ID)
	}
	if r.Kind != spec.KindComponent {
		t.Errorf("zero Kind = %v, want KindComponent", r.Kind)
	}
	if r.Deprecated {
		t.Errorf("zero Deprecated = true, want false")
	}
}

func TestKind_String(t *testing.T) {
	cases := []struct {
		k    spec.Kind
		want string
	}{
		{spec.KindComponent, "component"},
		{spec.KindConstraint, "constraint"},
	}
	for _, tc := range cases {
		if got := tc.k.String(); got != tc.want {
			t.Errorf("Kind(%d).String() = %q, want %q", tc.k, got, tc.want)
		}
	}
}
