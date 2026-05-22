// ABOUTME: Process-level registry mapping reporter names to Reporter implementations.
// ABOUTME: Reporters register at package init; callers resolve by string name.
package reporter

import "sync"

var (
	registryMu sync.RWMutex
	registry   = map[string]Reporter{}
)

// Register adds a Reporter to the process-level registry, keyed by Reporter.Name.
// Re-registration with the same name silently replaces the previous entry —
// matches the convention from pkg/spec.Register.
//
// Typical usage is from a package init():
//
//	func init() { reporter.Register(&acaiReporter{}) }
func Register(r Reporter) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[r.Name()] = r
}

// Lookup returns the Reporter registered under name, or ok=false if absent.
// Safe for concurrent callers.
func Lookup(name string) (Reporter, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	r, ok := registry[name]
	return r, ok
}

// Registered returns the names of every currently-registered reporter, in
// arbitrary order. Useful for diagnostics and for tracker doctor's spec probe.
func Registered() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}
	return out
}
