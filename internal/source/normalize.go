package source

import "unicode/utf8"

// Normalize removes one leading BOM, canonicalizes line endings, and leaves exactly one LF.
func Normalize(input []byte) ([]byte, []Diagnostic) {
	start := 0
	if len(input) >= 3 && input[0] == 0xef && input[1] == 0xbb && input[2] == 0xbf {
		start = 3
	}

	normalized := make([]byte, 0, len(input)+1)
	for i := start; i < len(input); i++ {
		if input[i] == '\r' {
			normalized = append(normalized, '\n')
			if i+1 < len(input) && input[i+1] == '\n' {
				i++
			}
			continue
		}
		normalized = append(normalized, input[i])
	}
	for len(normalized) > 0 && normalized[len(normalized)-1] == '\n' {
		normalized = normalized[:len(normalized)-1]
	}
	normalized = append(normalized, '\n')

	if utf8.Valid(normalized) {
		return normalized, nil
	}
	for offset := 0; offset < len(normalized); {
		r, size := utf8.DecodeRune(normalized[offset:])
		if r == utf8.RuneError && size == 1 {
			return normalized, []Diagnostic{diagnosticAt(normalized, offset, "invalid UTF-8")}
		}
		offset += size
	}
	return normalized, []Diagnostic{diagnosticAt(normalized, 0, "invalid UTF-8")}
}

func diagnosticAt(buffer []byte, offset int, message string) Diagnostic {
	if offset < 0 {
		offset = 0
	}
	if offset > len(buffer) {
		offset = len(buffer)
	}
	line, column := lineColumn(buffer, offset)
	return Diagnostic{Offset: offset, Line: line, Column: column, Message: message}
}

func lineColumn(buffer []byte, offset int) (int, int) {
	line := 1
	lineStart := 0
	for index := 0; index < offset; index++ {
		if buffer[index] == '\n' {
			line++
			lineStart = index + 1
		}
	}
	return line, offset - lineStart + 1
}
