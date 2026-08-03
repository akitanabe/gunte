package source

// Range is a half-open byte range [Start, End) in a normalized source buffer.
type Range struct {
	Start int
	End   int
}

// Diagnostic identifies a source parsing error in normalized-buffer coordinates.
type Diagnostic struct {
	Path    string
	Offset  int
	Line    int
	Column  int
	Message string
}

// Parts describes ranges in a normalized source buffer. FrontmatterRange ends
// after the closing +++ line's LF and, when present, one following blank line;
// BodyRange starts there and extends to the buffer's end. Ranges are half-open
// byte ranges.
type Parts struct {
	FrontmatterRange *Range
	BodyRange        Range
}

// Document contains normalized source bytes, its split ranges, and parsed TOML
// frontmatter. Its ranges are half-open byte ranges in Buffer; BodyRange starts
// immediately after FrontmatterRange and extends to the buffer's end.
type Document struct {
	Buffer           []byte
	FrontmatterRange *Range
	BodyRange        Range
	FrontmatterData  map[string]any
}
