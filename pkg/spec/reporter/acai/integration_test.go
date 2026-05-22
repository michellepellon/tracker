// ABOUTME: Integration test against the real acai CLI binary — skipped without ACAI_API_TOKEN.
// ABOUTME: Build tag `integration` keeps it out of the default test run; opt-in via -tags=integration.

//go:build integration

package acai_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/2389-research/tracker/pkg/spec/reporter"
	acai "github.com/2389-research/tracker/pkg/spec/reporter/acai"
)

func TestIntegration_Available(t *testing.T) {
	if os.Getenv("ACAI_API_TOKEN") == "" {
		t.Skip("ACAI_API_TOKEN unset; skipping live integration test")
	}
	if _, err := exec.LookPath("acai"); err != nil {
		t.Skip("acai binary not on PATH; skipping")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r := acai.New()
	if !r.Available(ctx) {
		t.Errorf("Available() = false with ACAI_API_TOKEN set; expected true (token may be invalid)")
	}
}

func TestIntegration_PullEmptyFeatureReturnsNoError(t *testing.T) {
	if os.Getenv("ACAI_API_TOKEN") == "" {
		t.Skip("ACAI_API_TOKEN unset")
	}
	if _, err := exec.LookPath("acai"); err != nil {
		t.Skip("acai binary not on PATH")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Use a feature name that almost certainly doesn't exist server-side, so the
	// CLI returns an empty result rather than real data we don't want to depend on.
	target := reporter.Target{
		Feature:        "definitely-not-a-real-feature-12345",
		Product:        "definitely-not-a-real-feature-12345",
		Implementation: "test",
	}
	got, err := acai.New().Pull(ctx, target)
	// We accept either: (a) empty map + nil error if acai treats "unknown
	// feature" as 0 results, or (b) a non-nil error containing the CLI's
	// stderr. Both are acceptable surface contracts.
	if err == nil && len(got) != 0 {
		t.Errorf("expected empty result for nonsense feature, got %d entries", len(got))
	}
}
