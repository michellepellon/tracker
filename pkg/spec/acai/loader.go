// ABOUTME: acai feature.yaml loader — parses a single spec file into pkg/spec types.
// ABOUTME: Registers itself with the spec.Registry at init time under name "acai".

// Package acai implements a spec.Loader for acai's feature.yaml format.
//
// Workflow authors reference this loader via `spec: acai <path>` in the .dip
// header. Tracker resolves the loader by name and invokes Load to obtain a
// spec.Spec. The loader handles acai's short-form and long-form requirement
// shapes, sub-requirements, `<N>-note:` annotations, deprecation flags, and
// the components vs. constraints distinction. See
// docs/superpowers/specs/2026-05-22-spec-loader-design.md for design rationale
// and https://acai.sh/llms.txt for the source format.
package acai

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/2389-research/tracker/pkg/spec"
)

func init() {
	spec.Register(loader{})
}

const loaderName = "acai"

type loader struct{}

func (loader) Name() string { return loaderName }

func (loader) Load(path string) (spec.Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("acai: open %s: %w", path, err)
	}
	return parse(path, data)
}

// rawDoc is the structural shell of a feature.yaml — we use yaml.Node for the
// requirement bodies because their value can be either a scalar (short form)
// or a mapping (long form with `requirement:` / `deprecated:`).
type rawDoc struct {
	Feature     rawFeature            `yaml:"feature"`
	Components  map[string]rawSection `yaml:"components"`
	Constraints map[string]rawSection `yaml:"constraints"`
}

type rawFeature struct {
	Name        string `yaml:"name"`
	Product     string `yaml:"product"`
	Description string `yaml:"description"`
}

type rawSection struct {
	Description  string               `yaml:"description"`
	Requirements map[string]yaml.Node `yaml:"requirements"`
}

func parse(path string, data []byte) (spec.Spec, error) {
	var doc rawDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("acai: parse %s: %w", path, err)
	}
	if doc.Feature.Name == "" {
		return nil, fmt.Errorf("acai: %s: feature.name is required", path)
	}
	if len(doc.Components) == 0 && len(doc.Constraints) == 0 {
		return nil, fmt.Errorf("acai: %s: spec has no components or constraints", path)
	}

	s := &acaiSpec{name: doc.Feature.Name}
	if err := s.absorb(path, doc); err != nil {
		return nil, err
	}
	return s, nil
}

// acaiSpec is the concrete spec.Spec returned by Load.
type acaiSpec struct {
	name string
	reqs []spec.Requirement
	idx  map[string]int // ID -> index into reqs
}

func (s *acaiSpec) Name() string { return s.name }

func (s *acaiSpec) Requirements() []spec.Requirement {
	out := make([]spec.Requirement, len(s.reqs))
	copy(out, s.reqs)
	return out
}

func (s *acaiSpec) Requirement(acid string) (spec.Requirement, bool) {
	i, ok := s.idx[acid]
	if !ok {
		return spec.Requirement{}, false
	}
	return s.reqs[i], true
}

// absorb walks the parsed document and flattens it into Requirements.
func (s *acaiSpec) absorb(path string, doc rawDoc) error {
	s.idx = map[string]int{}
	if err := s.absorbSections(path, doc.Components, spec.KindComponent); err != nil {
		return err
	}
	return s.absorbSections(path, doc.Constraints, spec.KindConstraint)
}

func (s *acaiSpec) absorbSections(path string, sections map[string]rawSection, kind spec.Kind) error {
	for _, name := range sortedKeys(sections) {
		section := sections[name]
		if err := s.absorbSection(path, name, section, kind); err != nil {
			return err
		}
	}
	return nil
}

func (s *acaiSpec) absorbSection(path, component string, section rawSection, kind spec.Kind) error {
	notes, err := collectNotes(path, component, section.Requirements)
	if err != nil {
		return err
	}
	for _, key := range sortedKeys(section.Requirements) {
		if err := s.absorbOne(path, component, key, section.Requirements[key], notes, kind); err != nil {
			return err
		}
	}
	return nil
}

// collectNotes walks a section's requirements once and pulls out every
// "<N>-note" entry, keyed by the numbered requirement it attaches to.
func collectNotes(path, component string, reqs map[string]yaml.Node) (map[string][]string, error) {
	notes := map[string][]string{}
	for key, node := range reqs {
		num, ok := strings.CutSuffix(key, "-note")
		if !ok {
			continue
		}
		if isInvalidNumber(num) {
			return nil, fmt.Errorf("acai: %s: %s.%s: sub-sub-requirements (%q) not allowed",
				path, component, key, key)
		}
		notes[num] = append(notes[num], nodeText(node))
	}
	return notes, nil
}

// absorbOne handles a single key from a section's requirements map: rejects
// sub-sub-requirements, builds the Requirement, attaches any notes, and
// appends it to the spec.
func (s *acaiSpec) absorbOne(path, component, key string, node yaml.Node, notes map[string][]string, kind spec.Kind) error {
	if strings.HasSuffix(key, "-note") {
		return nil
	}
	if isInvalidNumber(key) {
		return fmt.Errorf("acai: %s: %s.%s: sub-sub-requirements not allowed", path, component, key)
	}
	req, err := s.buildRequirement(component, key, node, kind)
	if err != nil {
		return fmt.Errorf("acai: %s: %s.%s: %w", path, component, key, err)
	}
	req.Notes = notes[key]
	s.idx[req.ID] = len(s.reqs)
	s.reqs = append(s.reqs, req)
	return nil
}

func (s *acaiSpec) buildRequirement(component, number string, node yaml.Node, kind spec.Kind) (spec.Requirement, error) {
	r := spec.Requirement{
		ID:        s.name + "." + component + "." + number,
		Feature:   s.name,
		Component: component,
		Number:    number,
		Kind:      kind,
	}
	if parent, ok := parentACID(s.name, component, number); ok {
		r.Parent = parent
	}
	if err := populateBody(&r, node); err != nil {
		return spec.Requirement{}, err
	}
	return r, nil
}

// populateBody fills the Text and Deprecated fields from a yaml.Node that may
// be either a scalar (short form: "1: text") or a mapping (long form:
// "1: { requirement: text, deprecated: true }").
func populateBody(r *spec.Requirement, node yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		r.Text = node.Value
		return nil
	case yaml.MappingNode:
		return populateBodyMapping(r, node)
	default:
		return fmt.Errorf("unexpected node kind %d", node.Kind)
	}
}

func populateBodyMapping(r *spec.Requirement, node yaml.Node) error {
	var body struct {
		Requirement string `yaml:"requirement"`
		Deprecated  bool   `yaml:"deprecated"`
	}
	if err := node.Decode(&body); err != nil {
		return fmt.Errorf("decode long-form requirement: %w", err)
	}
	r.Text = body.Requirement
	r.Deprecated = body.Deprecated
	return nil
}

// nodeText returns the scalar text of a yaml.Node, or its raw value if it's
// not a scalar. Used for `<N>-note:` siblings.
func nodeText(node yaml.Node) string {
	if node.Kind == yaml.ScalarNode {
		return node.Value
	}
	return strings.TrimSpace(node.Value)
}

// isInvalidNumber returns true for sub-sub-requirements like "1-1-1" — per
// the acai spec, only a single dash is permitted in a requirement number.
func isInvalidNumber(num string) bool {
	return strings.Count(num, "-") > 1
}

// parentACID computes the parent ACID for a sub-requirement.
// "1-2" → "feature.COMPONENT.1"; top-level numbers return ok=false.
func parentACID(feature, component, number string) (string, bool) {
	dash := strings.Index(number, "-")
	if dash <= 0 {
		return "", false
	}
	return feature + "." + component + "." + number[:dash], true
}

// sortedKeys returns the keys of a map in sorted order, for deterministic iteration.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
