// ABOUTME: Default commandRunner — backs the acai Reporter with exec.CommandContext invocations.
// ABOUTME: Tests inject a fake runner via WithRunner; production uses defaultRunner.
package acai

import (
	"bytes"
	"context"
	"os/exec"
)

// defaultRunner invokes the named binary with args via exec.CommandContext.
// It captures stdout and stderr separately so the Reporter can surface the
// CLI's stderr text in error returns without polluting stdout parsers.
func defaultRunner(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}
