// ABOUTME: Process-level registry mapping loader names to Loader implementations.
// ABOUTME: Loaders register themselves at package init; callers resolve by string name.
package spec

import "sync"

var (
	registryMu sync.RWMutex
	registry   = map[string]Loader{}
)

// Register adds a Loader to the process-level registry, keyed by Loader.Name.
// Re-registration with the same name silently replaces the previous entry —
// this matches the precedent of image.RegisterFormat and database/sql.Register
// in the Go stdlib and keeps test setup simple.
//
// Typical usage is from a package init():
//
//	func init() { spec.Register(loader{}) }
func Register(l Loader) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[l.Name()] = l
}

// Lookup returns the Loader registered under name, or ok=false if no loader is
// registered. Lookup is safe for concurrent callers.
func Lookup(name string) (Loader, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	l, ok := registry[name]
	return l, ok
}

// Registered returns the names of every loader currently in the registry, in
// arbitrary order. Useful for diagnostics ("unknown spec loader %q; registered
// loaders are: %v") and for tracker doctor's spec probe.
func Registered() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}
	return out
}
