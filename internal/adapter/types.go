// Package adapter resolves source paths and metadata mappings into logical
// artifacts without performing serialization or filesystem I/O.
package adapter

import (
	"github.com/akitanabe/gunte/internal/compile"
	"github.com/akitanabe/gunte/internal/config"
)

// Source supplies one mapping's input. Multiline profiles use WholeSource as
// normalized whole-source bytes; semantic profiles use Projection and Frontmatter.
// Frontmatter is read-only input; Adapt never changes the map or its values.
type Source struct {
	Projection compile.SourceProjection
	// WholeSource contains normalized whole-source bytes for multiline profiles.
	WholeSource []byte
	// Frontmatter is used with Projection by semantic profiles.
	Frontmatter map[string]any
}

// RuleMatch is one source/rule match, retained independently of path validity.
type RuleMatch struct {
	TargetIndex int
	RuleIndex   int
	Captures    []string
}

// SourceRuleMatches contains all rule matches for one configured source.
type SourceRuleMatches struct {
	Path    string
	Matches []RuleMatch
}

// RuleMatches is the complete source/rule match result for every target.
type RuleMatches struct {
	Sources []SourceRuleMatches
}

// OpaqueOnly reports whether a source matched at least one rule and every
// matched rule uses the opaque multiline profile.
func (matches RuleMatches) OpaqueOnly(project config.ProjectConfig, sourceIndex int) bool {
	if sourceIndex < 0 || sourceIndex >= len(matches.Sources) || len(matches.Sources[sourceIndex].Matches) == 0 {
		return false
	}
	for _, match := range matches.Sources[sourceIndex].Matches {
		if match.TargetIndex < 0 || match.TargetIndex >= len(project.Targets) || match.RuleIndex < 0 || match.RuleIndex >= len(project.Targets[match.TargetIndex].Rules) || project.Targets[match.TargetIndex].Rules[match.RuleIndex].Profile != config.ProfileMultilineText {
			return false
		}
	}
	return true
}

// MetadataValue is the typed, pre-serialization value of a metadata mapping.
// String is used by string and plain_token. Strings is used by list values.
type MetadataValue struct {
	Type    config.MetadataType
	String  string
	Strings []string
}

// MetadataField preserves mapping declaration order for the serializer.
type MetadataField struct {
	Field string
	Value MetadataValue
}

// Artifact is the logical output of one target rule. Body and metadata are
// ready for a later profile serializer; no bytes are written here.
type Artifact struct {
	TargetID   string
	SourcePath string
	Path       string
	Profile    config.Profile
	Header     string
	BodyField  string
	Body       []byte
	Metadata   []MetadataField
	Value      *MetadataValue
	Contracts  []compile.ProjectedDeclaration
	Anchors    []compile.ProjectedDeclaration
}

// Result contains artifacts in target declaration order, then source order.
type Result struct {
	Artifacts []Artifact
}

// PathPlan is the validated rule and output-path decision for every target.
// It deliberately excludes metadata and serialized profile data so selection
// can happen after global path validation without evaluating unselected output.
type PathPlan struct {
	Artifacts        []PlannedArtifact
	UnmatchedSources []string
}

// PlannedArtifact identifies one selected rule and its normalized output path.
type PlannedArtifact struct {
	TargetIndex int
	SourceIndex int
	RuleIndex   int
	Path        string
}

// Severity distinguishes mapping errors from the optional unmatched source
// warning.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Diagnostic identifies, when applicable, the source, target, rule, and
// metadata field involved in a deterministic adapter diagnostic.
type Diagnostic struct {
	Code     string
	Severity Severity
	Source   string
	Target   string
	Rule     int
	Field    string
	Message  string
}
