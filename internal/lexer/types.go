// Package lexer recognizes Gunte's line-oriented source notation without I/O.
package lexer

import "github.com/akitanabe/gunte/internal/source"

// Position identifies a byte in a normalized source buffer. Offset is
// zero-origin; Line and byte-based Column are one-origin.
type Position struct {
	Offset int
	Line   int
	Column int
}

// MarkerKind identifies a recognized directive line.
type MarkerKind string

const (
	ContractOpen  MarkerKind = "contract_open"
	ContractClose MarkerKind = "contract_close"
	AnchorMarker  MarkerKind = "anchor"
	OnlyOpen      MarkerKind = "only_open"
	OnlyClose     MarkerKind = "only_close"
)

// Marker describes a recognized directive. Range is the complete directive
// line, including its LF when present, in the normalized source buffer.
type Marker struct {
	Kind     MarkerKind
	Token    string
	Range    source.Range
	Position Position
}

// Block describes one directive block, including an EOF-open block without a
// paired closing marker. ContentRange is a half-open range beginning
// immediately after the opening marker and ending at the start of the closing
// marker line, or at body EOF when Close is nil.
type Block struct {
	Token        string
	Open         Marker
	Close        *Marker
	ContentRange source.Range
}

// Anchor records an anchor declaration and its complete marker-line range.
type Anchor struct {
	Token    string
	Range    source.Range
	Position Position
}

// TermUse records the exact {{name}} token range in the normalized buffer.
type TermUse struct {
	Name     string
	Range    source.Range
	Position Position
}

// IR is the source notation recognized within the supplied body range.
// Markers and TermUses retain source order within their respective sequences.
type IR struct {
	Markers       []Marker
	ContractSpans []Block
	OnlyBlocks    []Block
	Anchors       []Anchor
	TermUses      []TermUse
}
