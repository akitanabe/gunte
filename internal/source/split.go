package source

import "bytes"

// Split locates exact +++ delimiter lines in a normalized source buffer.
func Split(buffer []byte) (Parts, []Diagnostic) {
	parts := Parts{BodyRange: Range{Start: 0, End: len(buffer)}}
	if !bytes.HasPrefix(buffer, []byte("+++\n")) {
		return parts, nil
	}

	openingEnd := len([]byte("+++\n"))
	closingStart, closingEnd, found := findClosingDelimiter(buffer, openingEnd)
	if !found {
		return Parts{}, []Diagnostic{diagnosticAt(buffer, 0, "frontmatter delimiter is not closed")}
	}

	frontmatterEnd := closingEnd
	if frontmatterEnd < len(buffer) && buffer[frontmatterEnd] == '\n' {
		frontmatterEnd++
	}
	parts.FrontmatterRange = &Range{Start: 0, End: frontmatterEnd}
	parts.BodyRange = Range{Start: frontmatterEnd, End: len(buffer)}
	_ = closingStart
	return parts, nil
}

func findClosingDelimiter(buffer []byte, start int) (int, int, bool) {
	for lineStart := start; lineStart <= len(buffer); {
		lineEnd := bytes.IndexByte(buffer[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(buffer)
		} else {
			lineEnd += lineStart
		}
		if bytes.Equal(buffer[lineStart:lineEnd], []byte("+++")) {
			closingEnd := lineEnd
			if lineEnd < len(buffer) && buffer[lineEnd] == '\n' {
				closingEnd++
			}
			return lineStart, closingEnd, true
		}
		if lineEnd >= len(buffer) {
			break
		}
		lineStart = lineEnd + 1
	}
	return 0, 0, false
}
