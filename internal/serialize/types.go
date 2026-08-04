// Package serialize turns logical adapter artifacts into canonical bytes.
// Serialization is a pure calculation and performs no filesystem I/O.
package serialize

import (
	"github.com/akitanabe/gunte/internal/compile"
	"github.com/akitanabe/gunte/internal/source"
)

// Declaration is serializer-owned output provenance. ArtifactRange is a
// half-open range in the final artifact byte coordinate system.
type Declaration struct {
	ID            string
	Source        compile.SourcePosition
	Emitted       bool
	ArtifactRange source.Range
}

// Artifact is the final, in-memory artifact produced by Serialize.
type Artifact struct {
	TargetID   string
	SourcePath string
	Path       string
	Bytes      []byte
	Contracts  []Declaration
	Anchors    []Declaration
}

// Diagnostic describes a serialization input or grammar error.
type Diagnostic struct {
	Path    string
	Code    string
	Message string
}
