// ABOUTME: Tests for ctx.spec.status.<acid> condition routing — confirms PR3's pull-results are reachable by the condition evaluator.
// ABOUTME: Also covers no-dirty-bleed semantics so the keys stay global instead of getting copied into per-node namespaces.
package pipeline

import (
	"testing"
)

func TestSpecStatus_ConditionRoutesOnPass(t *testing.T) {
	pctx := NewPipelineContext()
	// Mimic what pullSpecStatuses does after a successful Pull.
	pctx.MergeWithoutDirty(map[string]string{
		"spec.status.example.AUTH.1": "pass",
		"spec.status.example.AUTH.2": "fail",
	})

	cases := []struct {
		expr string
		want bool
	}{
		{"ctx.spec.status.example.AUTH.1 = pass", true},
		{"ctx.spec.status.example.AUTH.1 != pass", false},
		{"ctx.spec.status.example.AUTH.2 = fail", true},
		{"ctx.spec.status.example.AUTH.2 = pass", false},
		// Absent key: lenient evaluator returns empty string; "" != "pass" → true.
		{"ctx.spec.status.example.NOPE.1 = pass", false},
		{"ctx.spec.status.example.NOPE.1 != pass", true},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			got, err := EvaluateCondition(tc.expr, pctx)
			if err != nil {
				t.Fatalf("EvaluateCondition(%q): %v", tc.expr, err)
			}
			if got != tc.want {
				t.Errorf("EvaluateCondition(%q) = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestSpecStatus_ConditionCombinedWithAnd(t *testing.T) {
	// Tracker's edge-condition evaluator uses `&&` / `||` literally.
	// Dippin's higher-level `and` / `or` words flow through Condition.Raw
	// as-is, so authors of conditions that mix multiple spec.status checks
	// should use `&&` for now (or rely on the manager_loop expression path
	// which serializes to `&&`).
	pctx := NewPipelineContext()
	pctx.MergeWithoutDirty(map[string]string{
		"spec.status.example.AUTH.1": "pass",
		"spec.status.example.AUTH.2": "pass",
		"spec.status.example.AUTH.3": "fail",
	})

	cases := []struct {
		expr string
		want bool
	}{
		{"ctx.spec.status.example.AUTH.1 = pass && ctx.spec.status.example.AUTH.2 = pass", true},
		{"ctx.spec.status.example.AUTH.1 = pass && ctx.spec.status.example.AUTH.3 = pass", false},
		{"ctx.spec.status.example.AUTH.3 = pass || ctx.spec.status.example.AUTH.1 = pass", true},
		{"ctx.spec.status.example.AUTH.3 = pass || ctx.spec.status.example.AUTH.3 = fail", true},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			got, err := EvaluateCondition(tc.expr, pctx)
			if err != nil {
				t.Fatalf("EvaluateCondition(%q): %v", tc.expr, err)
			}
			if got != tc.want {
				t.Errorf("EvaluateCondition(%q) = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestSpecStatus_NoDirtyBleedIntoNodeScope(t *testing.T) {
	pctx := NewPipelineContext()
	pctx.MergeWithoutDirty(map[string]string{
		"spec.status.example.AUTH.1": "pass",
	})
	// Simulate a node completing and the engine scoping its dirty keys into
	// the per-node namespace. We then assert no node.A.spec.status.* keys
	// were created — proves MergeWithoutDirty did its job.
	pctx.Set("some_dirty_key", "dirty_value") // any dirty write to trigger scope copy
	pctx.ScopeToNode("A")

	// The dirty key SHOULD have been scoped:
	if got, _ := pctx.Get("node.A.some_dirty_key"); got != "dirty_value" {
		t.Errorf("dirty key not scoped: node.A.some_dirty_key = %q, want dirty_value", got)
	}
	// The spec.status key SHOULD NOT have been scoped:
	if got, ok := pctx.Get("node.A.spec.status.example.AUTH.1"); ok {
		t.Errorf("spec.status key bled into node scope: node.A.spec.status.example.AUTH.1 = %q (should be absent)", got)
	}
	// The original key is still readable globally:
	if got, _ := pctx.Get("spec.status.example.AUTH.1"); got != "pass" {
		t.Errorf("original spec.status key disappeared: got %q, want pass", got)
	}
}

func TestSpecStatus_AllStateValuesRouteCorrectly(t *testing.T) {
	pctx := NewPipelineContext()
	pctx.MergeWithoutDirty(map[string]string{
		"spec.status.x.A.1": "pass",
		"spec.status.x.A.2": "fail",
		"spec.status.x.A.3": "blocked",
		"spec.status.x.A.4": "pending",
		"spec.status.x.A.5": "unknown",
	})
	cases := map[string]bool{
		"ctx.spec.status.x.A.1 = pass":    true,
		"ctx.spec.status.x.A.2 = fail":    true,
		"ctx.spec.status.x.A.3 = blocked": true,
		"ctx.spec.status.x.A.4 = pending": true,
		"ctx.spec.status.x.A.5 = unknown": true,
	}
	for expr, want := range cases {
		t.Run(expr, func(t *testing.T) {
			got, err := EvaluateCondition(expr, pctx)
			if err != nil {
				t.Fatalf("EvaluateCondition(%q): %v", expr, err)
			}
			if got != want {
				t.Errorf("EvaluateCondition(%q) = %v, want %v", expr, got, want)
			}
		})
	}
}
