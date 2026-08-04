// Package compile validates source declarations and projects normalized source
// bodies for every configured target without performing I/O.
package compile

import (
	"github.com/akitanabe/gunte/internal/lexer"
	"github.com/akitanabe/gunte/internal/source"
)

// SourceUnit groups the parsed data for one configured source path.
type SourceUnit struct {
	Path     string
	Document source.Document
	IR       lexer.IR
}

// SourcePosition identifies a declaration in its normalized source buffer.
type SourcePosition struct {
	Path   string
	Offset int
	Line   int
	Column int
}

// ProjectedDeclaration connects a source declaration to one target projection.
// ProjectedRange uses offsets in SourceProjection.Bytes. Contract ranges are
// half-open content ranges; emitted anchors use an empty range at their output
// offset. ProjectedRange is unspecified when Emitted is false.
type ProjectedDeclaration struct {
	ID             string
	Source         SourcePosition
	Emitted        bool
	ProjectedRange source.Range
}

// SourceProjection is one source body after target-specific term replacement
// and directive projection. Frontmatter bytes are not included.
type SourceProjection struct {
	Path      string
	Bytes     []byte
	Contracts []ProjectedDeclaration
	Anchors   []ProjectedDeclaration
}

// TargetProjection contains source projections in configured source order.
type TargetProjection struct {
	TargetID string
	Sources  []SourceProjection
}

// Result contains target projections in configured target order.
type Result struct {
	Targets []TargetProjection
}
