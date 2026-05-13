package pipeline

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestSecureActivityLogPath_Override pins that TRACKER_AUDIT_DIR wins
// over every other resolution source.
func TestSecureActivityLogPath_Override(t *testing.T) {
	t.Setenv(auditDirEnvVar, "/tmp/custom-audit")
	t.Setenv(xdgStateHomeEnvVar, "/tmp/xdg-should-lose")
	t.Setenv("HOME", "/tmp/home-should-lose")

	got, err := SecureActivityLogPath("run-abc")
	if err != nil {
		t.Fatalf("SecureActivityLogPath: %v", err)
	}
	want := filepath.Join("/tmp/custom-audit", "run-abc", "activity.jsonl")
	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}

// TestSecureActivityLogPath_XDG pins that XDG_STATE_HOME is honored when
// TRACKER_AUDIT_DIR is unset.
func TestSecureActivityLogPath_XDG(t *testing.T) {
	t.Setenv(auditDirEnvVar, "")
	t.Setenv(xdgStateHomeEnvVar, "/var/lib/state")

	got, err := SecureActivityLogPath("run-abc")
	if err != nil {
		t.Fatalf("SecureActivityLogPath: %v", err)
	}
	want := filepath.Join("/var/lib/state", "tracker", "runs", "run-abc", "activity.jsonl")
	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}

// TestSecureActivityLogPath_HomeDefault pins the Linux+macOS default
// when neither override nor XDG is set.
func TestSecureActivityLogPath_HomeDefault(t *testing.T) {
	t.Setenv(auditDirEnvVar, "")
	t.Setenv(xdgStateHomeEnvVar, "")
	t.Setenv("HOME", "/Users/alice")

	got, err := SecureActivityLogPath("run-abc")
	if err != nil {
		t.Fatalf("SecureActivityLogPath: %v", err)
	}
	want := filepath.Join("/Users/alice", ".local", "state", "tracker", "runs", "run-abc", "activity.jsonl")
	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}

// TestSecureActivityLogPath_EmptyRunID rejects empty runID — defensive
// guard so a caller that forgot to populate runID gets a loud error
// instead of a path under the base dir's root.
func TestSecureActivityLogPath_EmptyRunID(t *testing.T) {
	_, err := SecureActivityLogPath("")
	if err == nil {
		t.Fatal("expected error for empty runID")
	}
	if !strings.Contains(err.Error(), "empty runID") {
		t.Errorf("error = %q, want it to mention empty runID", err)
	}
}

// TestActivityLogSentinel pins the exact byte sequence. Changing it is
// a wire-format break that requires a migration plan.
func TestActivityLogSentinel(t *testing.T) {
	if len(ActivityLogSentinel) != 2 {
		t.Fatalf("sentinel length = %d, want 2", len(ActivityLogSentinel))
	}
	if ActivityLogSentinel[0] != 0x1f {
		t.Errorf("sentinel[0] = 0x%02x, want 0x1f (Unit Separator)", ActivityLogSentinel[0])
	}
	if ActivityLogSentinel[1] != 0x1e {
		t.Errorf("sentinel[1] = 0x%02x, want 0x1e (Record Separator)", ActivityLogSentinel[1])
	}
}
