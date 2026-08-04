package contract

import "bytes"

type patternPart struct {
	whitespace bool
	bytes      []byte
}

// Match reports whether body contains pattern under the contract token rules.
// Matching is byte exact for UTF-8, except that each pattern whitespace run is
// matched by one or more ASCII SP/HTAB/LF bytes.
func Match(body []byte, pattern string) bool {
	parts := parsePattern([]byte(pattern))
	if len(parts) == 0 {
		return false
	}
	firstID := asciiIdentifier(pattern[0])
	last := []byte(pattern)
	lastID := len(last) > 0 && asciiIdentifier(last[len(last)-1])
	for start := 0; start < len(body); start++ {
		if firstID && leftBoundaryBlocked(body, start) {
			continue
		}
		end, ok := matchAt(body, start, parts)
		if !ok {
			continue
		}
		if lastID && rightBoundaryBlocked(body, end) {
			continue
		}
		return true
	}
	return false
}

// The external contract forbids matching an agent-name suffix across a hyphen,
// such as "implementer" inside "senior-implementer".
func leftBoundaryBlocked(body []byte, start int) bool {
	if start == 0 {
		return false
	}
	if asciiIdentifier(body[start-1]) {
		return true
	}
	return body[start-1] == '-' && start > 1 && asciiIdentifier(body[start-2])
}

func rightBoundaryBlocked(body []byte, end int) bool {
	if end >= len(body) {
		return false
	}
	if asciiIdentifier(body[end]) {
		return true
	}
	return body[end] == '-' && end+1 < len(body) && asciiIdentifier(body[end+1])
}

func parsePattern(pattern []byte) []patternPart {
	parts := make([]patternPart, 0, len(pattern))
	for index := 0; index < len(pattern); {
		if asciiWhitespace(pattern[index]) {
			end := index + 1
			for end < len(pattern) && asciiWhitespace(pattern[end]) {
				end++
			}
			parts = append(parts, patternPart{whitespace: true})
			index = end
			continue
		}
		end := index + 1
		for end < len(pattern) && !asciiWhitespace(pattern[end]) {
			end++
		}
		parts = append(parts, patternPart{bytes: append([]byte(nil), pattern[index:end]...)})
		index = end
	}
	return parts
}

func matchAt(body []byte, start int, parts []patternPart) (int, bool) {
	index := start
	for _, part := range parts {
		if part.whitespace {
			if index >= len(body) || !asciiWhitespace(body[index]) {
				return 0, false
			}
			for index < len(body) && asciiWhitespace(body[index]) {
				index++
			}
			continue
		}
		if len(body)-index < len(part.bytes) || !bytes.Equal(body[index:index+len(part.bytes)], part.bytes) {
			return 0, false
		}
		index += len(part.bytes)
	}
	return index, true
}

func asciiWhitespace(value byte) bool { return value == ' ' || value == '\t' || value == '\n' }

func asciiIdentifier(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '_'
}
