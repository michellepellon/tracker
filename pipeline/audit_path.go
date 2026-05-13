// ABOUTME: Resolves the on-disk location of the integrity-protected activity log.
// ABOUTME: Threat model for the relocation lives in CLAUDE.md (#213).
package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ActivityLogSentinel is the two-byte prefix the runtime writes ahead of
// every JSONL line in the secure activity log. The bytes are ASCII Unit
// Separator (0x1F) and Record Separator (0x1E) — control characters that
// a normal text-mode subprocess effectively never emits. Their presence
// is the runtime's "I wrote this" mark; their absence on a line in the
// secure file is the injection signal.
//
// This is detection, not authentication: the bytes are not secret and an
// attacker who reads tracker's source (open source) can emit them too.
// See the "Activity log integrity" section of CLAUDE.md for the threat
// model and the intentional scope of this defense.
const ActivityLogSentinel = "\x1f\x1e"

// auditDirEnvVar overrides the default secure-log base directory. Set to
// an absolute path; runs land under <override>/<runID>/activity.jsonl.
// Used by tests and embedders that want to pin a custom location.
const auditDirEnvVar = "TRACKER_AUDIT_DIR"

// xdgStateHomeEnvVar is the XDG Base Directory env var that, when set,
// determines where state-class user data lives. Default (per XDG spec)
// is $HOME/.local/state.
const xdgStateHomeEnvVar = "XDG_STATE_HOME"

// SecureActivityLogPath returns the absolute path where the runtime
// writes the integrity-protected activity log for runID. The path is
// outside any directory a tool subprocess sees as cmd.Dir, so the most
// common LLM-tool-mistake attack vectors (relative-path shell
// redirection, find-in-cwd globs) cannot reach it.
//
// Resolution order:
//
//  1. $TRACKER_AUDIT_DIR/<runID>/activity.jsonl — explicit operator override.
//  2. $XDG_STATE_HOME/tracker/runs/<runID>/activity.jsonl — when XDG_STATE_HOME is set.
//  3. $HOME/.local/state/tracker/runs/<runID>/activity.jsonl — Linux + macOS default.
//  4. %LOCALAPPDATA%\tracker\runs\<runID>\activity.jsonl — Windows fallback.
//  5. os.TempDir()/tracker-audit/<runID>/activity.jsonl — last-resort when no $HOME (containers, restricted envs).
//
// runID must be non-empty.
func SecureActivityLogPath(runID string) (string, error) {
	if runID == "" {
		return "", fmt.Errorf("secure activity log path: empty runID")
	}
	base, err := secureActivityLogBase()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, runID, "activity.jsonl"), nil
}

func secureActivityLogBase() (string, error) {
	if v := strings.TrimSpace(os.Getenv(auditDirEnvVar)); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(os.Getenv(xdgStateHomeEnvVar)); v != "" {
		return filepath.Join(v, "tracker", "runs"), nil
	}
	if runtime.GOOS == "windows" {
		if v := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); v != "" {
			return filepath.Join(v, "tracker", "runs"), nil
		}
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		return filepath.Join(home, ".local", "state", "tracker", "runs"), nil
	}
	// $HOME unresolvable (e.g. some minimal containers). Fall back to a
	// per-user-ish temp dir. This still keeps the log outside cmd.Dir,
	// which is the load-bearing property for the relative-path threat.
	return filepath.Join(os.TempDir(), "tracker-audit"), nil
}
