// ABOUTME: Loads .dipx bundles produced by dippin v0.24+ — verifies hashes,
// ABOUTME: converts pre-parsed IR to tracker Graphs, returns content-addressed identity.
package pipeline

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/2389-research/dippin-lang/dipx"
)

// BundleInfo carries the metadata extracted from a loaded .dipx bundle.
// Identity is the canonical "sha256:<64 hex>" form of the bundle's
// content-addressed hash (SHA-256 of manifest.json bytes-as-stored).
// EntryPath is the canonical bundle-relative path of the entry workflow.
type BundleInfo struct {
	Identity  string
	EntryPath string
	Manifest  dipx.Manifest
}

// LoadDipxBundle opens a .dipx file, verifies all SHA-256 hashes via
// dipx.Open (strict mode), converts the entry workflow and every transitively-
// referenced subgraph from pre-parsed IR to tracker Graphs, and returns the
// graphs plus a BundleInfo carrying the bundle's content-addressed identity.
//
// The subgraphs map is keyed by canonical bundle path (matching manifest.Files
// entries). dipx has already verified ref closure and acyclicity, so no
// recursive walk is needed on tracker's side.
func LoadDipxBundle(ctx context.Context, path string) (*Graph, map[string]*Graph, BundleInfo, error) {
	bundle, err := dipx.Open(ctx, path)
	if err != nil {
		return nil, nil, BundleInfo{}, fmt.Errorf("load bundle %s: %w", path, err)
	}
	manifest := bundle.Manifest()

	entry := bundle.Entry()
	entryGraph, diags, err := LoadDippinWorkflowFromIR(entry, manifest.Entry)
	for _, d := range diags {
		fmt.Fprintln(os.Stderr, d.String())
	}
	if err != nil {
		return nil, nil, BundleInfo{}, fmt.Errorf("load bundle %s: entry %s: %w", path, manifest.Entry, err)
	}

	subgraphs := make(map[string]*Graph)
	for _, file := range manifest.Files {
		if file.Path == manifest.Entry {
			continue
		}
		wf, err := bundle.Lookup(file.Path)
		if err != nil {
			return nil, nil, BundleInfo{}, fmt.Errorf("load bundle %s: lookup %s: %w", path, file.Path, err)
		}
		sub, subDiags, err := LoadDippinWorkflowFromIR(wf, file.Path)
		for _, d := range subDiags {
			fmt.Fprintln(os.Stderr, d.String())
		}
		if err != nil {
			return nil, nil, BundleInfo{}, fmt.Errorf("load bundle %s: subgraph %s: %w", path, file.Path, err)
		}
		subgraphs[file.Path] = sub
	}

	id := bundle.Identity()
	info := BundleInfo{
		Identity:  "sha256:" + hex.EncodeToString(id[:]),
		EntryPath: manifest.Entry,
		Manifest:  manifest,
	}
	return entryGraph, subgraphs, info, nil
}
