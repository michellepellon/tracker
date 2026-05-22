// ABOUTME: ACID pattern resolution — expands bare, range, and wildcard refs to concrete requirements.
// ABOUTME: Matches dippin's DIP139 syntactic shape; callers should pre-validate via dippin lint.
package acai

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/2389-research/tracker/pkg/spec"
)

// patternRE captures (feature, component-chain, requirement-segment). The
// shape matches dippin's acidPattern lint regex.
var patternRE = regexp.MustCompile(
	`^([a-z][a-z0-9_-]*)((?:\.[A-Z][A-Z0-9_]*)+)\.(\d+(?:-\d+)?|\*|\[\d+-\d+\])$`,
)

// Resolve expands an ACID pattern into matching requirements.
func (s *acaiSpec) Resolve(pattern string) []spec.Requirement {
	m := patternRE.FindStringSubmatch(pattern)
	if m == nil {
		return nil
	}
	feature, componentChain, reqSeg := m[1], strings.TrimPrefix(m[2], "."), m[3]
	if feature != s.name {
		return nil
	}
	switch {
	case reqSeg == "*":
		return s.resolveWildcard(componentChain)
	case strings.HasPrefix(reqSeg, "["):
		return s.resolveRange(componentChain, reqSeg)
	default:
		return s.resolveBare(s.name + "." + componentChain + "." + reqSeg)
	}
}

func (s *acaiSpec) resolveBare(acid string) []spec.Requirement {
	if r, ok := s.Requirement(acid); ok {
		return []spec.Requirement{r}
	}
	return nil
}

func (s *acaiSpec) resolveWildcard(component string) []spec.Requirement {
	var out []spec.Requirement
	for _, r := range s.reqs {
		if r.Component == component {
			out = append(out, r)
		}
	}
	return out
}

func (s *acaiSpec) resolveRange(component, seg string) []spec.Requirement {
	lo, hi, ok := parseRange(seg)
	if !ok {
		return nil
	}
	var out []spec.Requirement
	for _, r := range s.reqs {
		if inRange(r, component, lo, hi) {
			out = append(out, r)
		}
	}
	return out
}

// inRange reports whether a requirement is a top-level (non-sub) entry of the
// given component whose number falls in [lo, hi]. Sub-requirements have
// non-numeric Number fields (e.g. "1-1") and are excluded.
func inRange(r spec.Requirement, component string, lo, hi int) bool {
	if r.Component != component {
		return false
	}
	n, err := strconv.Atoi(r.Number)
	if err != nil {
		return false
	}
	return n >= lo && n <= hi
}

// parseRange unpacks "[N-M]" into (N, M, true).
func parseRange(seg string) (int, int, bool) {
	inner := strings.TrimSuffix(strings.TrimPrefix(seg, "["), "]")
	dash := strings.Index(inner, "-")
	if dash <= 0 {
		return 0, 0, false
	}
	lo, err1 := strconv.Atoi(inner[:dash])
	hi, err2 := strconv.Atoi(inner[dash+1:])
	if err1 != nil || err2 != nil || lo > hi {
		return 0, 0, false
	}
	return lo, hi, true
}
