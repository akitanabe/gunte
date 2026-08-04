// Package adapter resolves source paths and metadata mappings into logical
// artifacts without performing serialization or filesystem I/O.
package adapter

import (
	"github.com/akitanabe/gunte/internal/compile"
	"github.com/akitanabe/gunte/internal/config"
)

// Source supplies the projected bytes and frontmatter used by one mapping.
// Frontmatter is read-only input; Adapt never changes the map or its values.
type Source struct {
	Projection  compile.SourceProjection
	Frontmatter map[string]any
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
