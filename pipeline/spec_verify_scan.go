// ABOUTME: Filesystem scanner used by verify_acid to find ACID literals in the working tree.
// ABOUTME: Walks `.` recursively, skipping common noise dirs and binary files, reading each text file once.

package pipeline

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// scanSkipDirs lists directory names that scanWorkingTreeForLiterals skips
// during its walk. These are large, machine-generated, or otherwise unlikely
// to contain hand-authored ACID references and would only slow the scan.
var scanSkipDirs = map[string]bool{
	".git":          true,
	".gocache":      true,
	"node_modules":  true,
	"vendor":        true,
	"bin":           true,
	"dist":          true,
	"build":         true,
	".venv":         true,
	"__pycache__":   true,
	".pytest_cache": true,
}

// scanMaxFileSize caps how many bytes the scanner reads from any one file.
// Files larger than this are silently skipped — they're typically binaries,
// vendored data, or test fixtures unlikely to carry ACID references.
const scanMaxFileSize = 1 << 20 // 1 MiB

// scanWorkingTreeForLiterals walks the current working directory and reports
// which of the given literals appear anywhere in any text file under it.
// Returns a map keyed by literal; true means at least one occurrence was
// found. The scan stops looking for a given literal after the first match.
//
// Errors walking individual files are silently ignored (the file simply
// contributes no matches); only an outright failure to read the root
// directory returns an error to the caller.
func scanWorkingTreeForLiterals(literals []string) (map[string]bool, error) {
	found := initFoundMap(literals)
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := filepath.WalkDir(root, makeScanWalker(found)); err != nil {
		return nil, err
	}
	return found, nil
}

func initFoundMap(literals []string) map[string]bool {
	out := make(map[string]bool, len(literals))
	for _, l := range literals {
		out[l] = false
	}
	return out
}

// makeScanWalker returns the fs.WalkDirFunc closure used by
// scanWorkingTreeForLiterals. Factored out to keep that function's
// cyclomatic complexity below the project's gate.
func makeScanWalker(found map[string]bool) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return walkDirAction(d)
		}
		if shouldSkipFile(path, d) {
			return nil
		}
		scanFileForLiterals(path, found)
		return nil
	}
}

// walkDirAction returns SkipDir for known-noise directory names and nil
// otherwise. The root directory (Name() == ".") is always traversed.
func walkDirAction(d fs.DirEntry) error {
	if d.Name() != "." && scanSkipDirs[d.Name()] {
		return filepath.SkipDir
	}
	return nil
}

// shouldSkipFile reports whether the entry is too big to be worth scanning,
// or has an extension known to indicate binary content.
func shouldSkipFile(path string, d fs.DirEntry) bool {
	info, err := d.Info()
	if err != nil {
		return true
	}
	if info.Size() > scanMaxFileSize {
		return true
	}
	return isLikelyBinaryExt(filepath.Ext(path))
}

// isLikelyBinaryExt is a small allowlist of file extensions we definitively
// don't want to scan. Anything else is read as text; the scanner tolerates
// individual non-UTF8 lines.
func isLikelyBinaryExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".png", ".jpg", ".jpeg", ".gif", ".pdf", ".zip", ".tar", ".gz",
		".bz2", ".xz", ".woff", ".woff2", ".ttf", ".eot", ".ico", ".so",
		".dylib", ".dll", ".exe", ".bin", ".class", ".jar", ".o", ".a":
		return true
	}
	return false
}

// scanFileForLiterals reads path line-by-line and marks any literal it sees
// as found. Existing-found entries are left alone (first-match wins).
func scanFileForLiterals(path string, found map[string]bool) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		markLineMatches(scanner.Text(), found)
	}
}

// markLineMatches updates found with any literal that appears in line.
// Already-found literals are skipped (first-match wins).
func markLineMatches(line string, found map[string]bool) {
	for literal, seen := range found {
		if seen {
			continue
		}
		if strings.Contains(line, literal) {
			found[literal] = true
		}
	}
}
