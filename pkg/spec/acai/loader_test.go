// ABOUTME: Tests for the acai feature.yaml loader — happy path, full feature set, error paths, real fixture.
// ABOUTME: Exercises Load, Name, Requirements, Requirement, and Resolve via the public spec.Spec interface.
package acai_test

import (
	"strings"
	"testing"

	"github.com/2389-research/tracker/pkg/spec"
	_ "github.com/2389-research/tracker/pkg/spec/acai" // registers loader at init
)

func acaiLoader(t *testing.T) spec.Loader {
	t.Helper()
	l, ok := spec.Lookup("acai")
	if !ok {
		t.Fatalf("acai loader not registered")
	}
	return l
}

func loadFixture(t *testing.T, path string) spec.Spec {
	t.Helper()
	s, err := acaiLoader(t).Load(path)
	if err != nil {
		t.Fatalf("Load(%q): %v", path, err)
	}
	return s
}

// --- happy path: minimal fixture ---

func TestLoad_Minimal(t *testing.T) {
	s := loadFixture(t, "testdata/minimal.yaml")

	if s.Name() != "example" {
		t.Errorf("Name = %q, want example", s.Name())
	}

	reqs := s.Requirements()
	if len(reqs) != 2 {
		t.Fatalf("Requirements len = %d, want 2", len(reqs))
	}
	if reqs[0].ID != "example.AUTH.1" {
		t.Errorf("reqs[0].ID = %q", reqs[0].ID)
	}
	if reqs[0].Feature != "example" || reqs[0].Component != "AUTH" || reqs[0].Number != "1" {
		t.Errorf("reqs[0] segments wrong: %+v", reqs[0])
	}
	if reqs[0].Text != "First auth requirement." {
		t.Errorf("reqs[0].Text = %q", reqs[0].Text)
	}
	if reqs[0].Kind != spec.KindComponent {
		t.Errorf("reqs[0].Kind = %v, want KindComponent", reqs[0].Kind)
	}
}

// --- full feature set: sub-requirements, notes, long-form, deprecated, constraints ---

func TestLoad_Full_AllRequirementsPresent(t *testing.T) {
	s := loadFixture(t, "testdata/full.yaml")
	reqs := s.Requirements()
	wantIDs := []string{
		"full.AUTH.1", "full.AUTH.1-1", "full.AUTH.2", "full.AUTH.3",
		"full.PKG.1", "full.PKG.2",
	}
	got := map[string]bool{}
	for _, r := range reqs {
		got[r.ID] = true
	}
	for _, id := range wantIDs {
		if !got[id] {
			t.Errorf("missing requirement %q in %v", id, requirementIDs(reqs))
		}
	}
}

func TestLoad_Full_NotesAttachToParent(t *testing.T) {
	s := loadFixture(t, "testdata/full.yaml")
	r, ok := s.Requirement("full.AUTH.1")
	if !ok {
		t.Fatalf("AUTH.1 not found")
	}
	if len(r.Notes) != 1 || r.Notes[0] != "A note attached to requirement 1." {
		t.Errorf("Notes = %#v, want one note", r.Notes)
	}
}

func TestLoad_Full_SubRequirementParent(t *testing.T) {
	s := loadFixture(t, "testdata/full.yaml")
	r, ok := s.Requirement("full.AUTH.1-1")
	if !ok {
		t.Fatalf("AUTH.1-1 not found")
	}
	if r.Parent != "full.AUTH.1" {
		t.Errorf("Parent = %q, want full.AUTH.1", r.Parent)
	}
	if r.Number != "1-1" {
		t.Errorf("Number = %q, want 1-1", r.Number)
	}
}

func TestLoad_Full_LongForm(t *testing.T) {
	s := loadFixture(t, "testdata/full.yaml")
	r, ok := s.Requirement("full.AUTH.2")
	if !ok {
		t.Fatalf("AUTH.2 not found")
	}
	if r.Text != "Long-form requirement." {
		t.Errorf("Text = %q, want Long-form requirement.", r.Text)
	}
	if !r.Deprecated {
		t.Errorf("Deprecated = false, want true")
	}
}

func TestLoad_Full_ConstraintKind(t *testing.T) {
	s := loadFixture(t, "testdata/full.yaml")
	r, ok := s.Requirement("full.PKG.1")
	if !ok {
		t.Fatalf("PKG.1 not found")
	}
	if r.Kind != spec.KindConstraint {
		t.Errorf("Kind = %v, want KindConstraint", r.Kind)
	}
}

// --- error paths ---

func TestLoad_Error_MissingFeatureName(t *testing.T) {
	_, err := acaiLoader(t).Load("testdata/malformed_no_feature.yaml")
	if err == nil {
		t.Fatalf("expected error for missing feature.name")
	}
	if !strings.Contains(err.Error(), "feature.name") {
		t.Errorf("error %q should mention feature.name", err)
	}
}

func TestLoad_Error_SubSubRequirement(t *testing.T) {
	_, err := acaiLoader(t).Load("testdata/malformed_subsub.yaml")
	if err == nil {
		t.Fatalf("expected error for sub-sub-requirement")
	}
	if !strings.Contains(err.Error(), "1-1-1") {
		t.Errorf("error %q should mention 1-1-1", err)
	}
}

func TestLoad_Error_EmptySpec(t *testing.T) {
	_, err := acaiLoader(t).Load("testdata/malformed_empty.yaml")
	if err == nil {
		t.Fatalf("expected error for spec with no components or constraints")
	}
}

func TestLoad_Error_FileNotFound(t *testing.T) {
	_, err := acaiLoader(t).Load("testdata/does-not-exist.yaml")
	if err == nil {
		t.Fatalf("expected error for missing file")
	}
}

// --- Resolve ---

func TestResolve_Bare(t *testing.T) {
	s := loadFixture(t, "testdata/full.yaml")
	got := s.Resolve("full.AUTH.1")
	if len(got) != 1 || got[0].ID != "full.AUTH.1" {
		t.Errorf("Resolve(AUTH.1) = %v", requirementIDs(got))
	}
}

func TestResolve_Wildcard(t *testing.T) {
	s := loadFixture(t, "testdata/full.yaml")
	got := s.Resolve("full.AUTH.*")
	// AUTH has 1, 1-1, 2, 3 — wildcard returns every requirement in the component.
	wantIDs := []string{"full.AUTH.1", "full.AUTH.1-1", "full.AUTH.2", "full.AUTH.3"}
	if !sameSet(requirementIDs(got), wantIDs) {
		t.Errorf("Resolve(AUTH.*) = %v, want %v", requirementIDs(got), wantIDs)
	}
}

func TestResolve_Range(t *testing.T) {
	s := loadFixture(t, "testdata/full.yaml")
	got := s.Resolve("full.AUTH.[1-2]")
	// Range matches top-level requirement numbers only; sub-requirements are not auto-included.
	wantIDs := []string{"full.AUTH.1", "full.AUTH.2"}
	if !sameSet(requirementIDs(got), wantIDs) {
		t.Errorf("Resolve(AUTH.[1-2]) = %v, want %v", requirementIDs(got), wantIDs)
	}
}

func TestResolve_RangeGapsSkipped(t *testing.T) {
	s := loadFixture(t, "testdata/full.yaml")
	got := s.Resolve("full.AUTH.[1-99]")
	// AUTH top-level numbers are 1, 2, 3 — range [1-99] returns those three, no error.
	wantIDs := []string{"full.AUTH.1", "full.AUTH.2", "full.AUTH.3"}
	if !sameSet(requirementIDs(got), wantIDs) {
		t.Errorf("Resolve(AUTH.[1-99]) = %v, want %v", requirementIDs(got), wantIDs)
	}
}

func TestResolve_UnknownFeatureReturnsEmpty(t *testing.T) {
	s := loadFixture(t, "testdata/full.yaml")
	if got := s.Resolve("wrongname.AUTH.1"); len(got) != 0 {
		t.Errorf("Resolve(wrongname.AUTH.1) = %v, want empty", requirementIDs(got))
	}
}

func TestResolve_UnknownComponentReturnsEmpty(t *testing.T) {
	s := loadFixture(t, "testdata/full.yaml")
	if got := s.Resolve("full.NOPE.1"); len(got) != 0 {
		t.Errorf("Resolve(full.NOPE.1) = %v, want empty", requirementIDs(got))
	}
}

// --- real-world regression fixture ---

func TestLoad_CognitoFormsPy(t *testing.T) {
	s := loadFixture(t, "testdata/cognitoforms-py.yaml")
	if s.Name() != "cognitoforms-py" {
		t.Errorf("Name = %q, want cognitoforms-py", s.Name())
	}
	// AUTH has 4 requirements per the features.yaml we wrote earlier; CLIENT has 6.
	if got := len(s.Resolve("cognitoforms-py.AUTH.*")); got < 4 {
		t.Errorf("AUTH count = %d, want at least 4", got)
	}
	if got := len(s.Resolve("cognitoforms-py.CLIENT.*")); got < 6 {
		t.Errorf("CLIENT count = %d, want at least 6", got)
	}
	// Spot-check: every constraint requirement should be Kind=Constraint.
	for _, r := range s.Requirements() {
		if r.Component == "PACKAGING" && r.Kind != spec.KindConstraint {
			t.Errorf("%s should be Kind=Constraint, got %v", r.ID, r.Kind)
		}
	}
}

// --- helpers ---

func requirementIDs(rs []spec.Requirement) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.ID
	}
	return out
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, x := range a {
		seen[x]++
	}
	for _, x := range b {
		seen[x]--
	}
	for _, v := range seen {
		if v != 0 {
			return false
		}
	}
	return true
}
