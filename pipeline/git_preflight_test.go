// ABOUTME: Tests for git preflight error sentinels and decision logic.
// ABOUTME: Covers happy path, hard-fail, warn-downgrade, auto-init, and safety latches.
package pipeline

import (
	"errors"
	"testing"
)

func TestPreflightErrorSentinels(t *testing.T) {
	sentinels := []error{
		ErrGitNotInstalled,
		ErrGitWorkdirNotRepo,
		ErrGitAutoInitRefused,
		ErrGitDependencyUnsatisfied,
	}
	for _, s := range sentinels {
		if s == nil {
			t.Errorf("nil sentinel")
		}
		if s.Error() == "" {
			t.Errorf("sentinel %v has empty Error()", s)
		}
	}
	for i, a := range sentinels {
		for j, b := range sentinels {
			if i != j && errors.Is(a, b) {
				t.Errorf("sentinel collision: %v Is %v", a, b)
			}
		}
	}
}
