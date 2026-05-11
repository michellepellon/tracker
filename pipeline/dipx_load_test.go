// ABOUTME: Tests for LoadDipxBundle — the .dipx → Graph + BundleInfo loader.
// ABOUTME: Uses real bundles via dipxtest.PackTestBundle (no synthetic ZIPs, no mocks).
package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/2389-research/tracker/pipeline/internal/dipxtest"
)

func TestLoadDipxBundle_HappyPath(t *testing.T) {
	dir := t.TempDir()
	entryPath := filepath.Join(dir, "entry.dip")
	if err := os.WriteFile(entryPath, []byte(dipxtest.MinimalDip("entry_test", "start", "exit")), 0o644); err != nil {
		t.Fatal(err)
	}
	bundlePath := dipxtest.PackTestBundle(t, entryPath)

	graph, subgraphs, info, err := LoadDipxBundle(context.Background(), bundlePath)
	if err != nil {
		t.Fatalf("LoadDipxBundle: %v", err)
	}
	if graph == nil {
		t.Fatal("graph is nil")
	}
	if !graph.DippinValidated {
		t.Error("graph not marked DippinValidated")
	}
	if !strings.HasPrefix(info.Identity, "sha256:") {
		t.Errorf("identity should start with sha256:, got %q", info.Identity)
	}
	if len(info.Identity) != len("sha256:")+64 {
		t.Errorf("identity should be sha256: + 64 hex chars, got len %d (%q)", len(info.Identity), info.Identity)
	}
	if info.EntryPath == "" {
		t.Error("BundleInfo.EntryPath is empty")
	}
	if subgraphs == nil {
		t.Error("subgraphs map should be non-nil (even if empty)")
	}
	t.Logf("bundle identity: %s", info.Identity)
	t.Logf("bundle entry path: %s", info.EntryPath)
}

func TestLoadDipxBundle_NotAValidBundle(t *testing.T) {
	// A plain .dip with .dipx extension.
	fake := filepath.Join(t.TempDir(), "bogus.dipx")
	if err := os.WriteFile(fake, []byte(dipxtest.MinimalDip("not_a_bundle", "start", "exit")), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := LoadDipxBundle(context.Background(), fake)
	if err == nil {
		t.Fatal("expected error on non-ZIP .dipx, got nil")
	}
	if !strings.Contains(err.Error(), "load bundle") {
		t.Errorf("error should wrap with 'load bundle': %v", err)
	}
}

func TestLoadDipxBundle_HashMismatch(t *testing.T) {
	dir := t.TempDir()
	entryPath := filepath.Join(dir, "entry.dip")
	if err := os.WriteFile(entryPath, []byte(dipxtest.MinimalDip("tampered_test", "start", "exit")), 0o644); err != nil {
		t.Fatal(err)
	}
	bundlePath := dipxtest.PackTestBundle(t, entryPath)

	// Tamper one byte at offset 100 (compressed data region).
	raw, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 200 {
		t.Skipf("bundle too small to tamper safely (%d bytes)", len(raw))
	}
	raw[100] ^= 0xFF
	if err := os.WriteFile(bundlePath, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, _, err = LoadDipxBundle(context.Background(), bundlePath)
	if err == nil {
		t.Fatal("expected error on tampered bundle, got nil")
	}
}
