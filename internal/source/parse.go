package source

// Parse normalizes, splits, and parses a source's optional TOML frontmatter without I/O.
func Parse(path string, input []byte) (Document, []Diagnostic) {
	normalized, diagnostics := Normalize(input)
	document := Document{Buffer: normalized}
	if len(diagnostics) != 0 {
		for index := range diagnostics {
			diagnostics[index].Path = path
		}
		return document, diagnostics
	}

	parts, diagnostics := Split(normalized)
	if len(diagnostics) != 0 {
		for index := range diagnostics {
			diagnostics[index].Path = path
		}
		return document, diagnostics
	}
	document.FrontmatterRange = parts.FrontmatterRange
	document.BodyRange = parts.BodyRange
	if parts.FrontmatterRange == nil {
		return document, nil
	}
	frontmatter, diagnostics := ParseFrontmatter(path, normalized, *parts.FrontmatterRange)
	if len(diagnostics) != 0 {
		return document, diagnostics
	}
	document.FrontmatterData = frontmatter
	return document, nil
}
