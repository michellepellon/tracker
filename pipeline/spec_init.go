// ABOUTME: Side-effect imports that register the acai spec loader and reporter at process init.
// ABOUTME: Imported by the pipeline package so any tracker entrypoint that uses pipeline sees the registrations.

package pipeline

import (
	// Register the acai spec.Loader under name "acai" so workflows
	// declaring `spec: acai <path>` can resolve their loader.
	_ "github.com/2389-research/tracker/pkg/spec/acai"

	// Register the acai reporter.Reporter under name "acai" so the
	// engine can pull / push status updates to the acai server.
	_ "github.com/2389-research/tracker/pkg/spec/reporter/acai"
)
