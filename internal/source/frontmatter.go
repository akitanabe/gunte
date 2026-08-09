package source

import (
	"errors"

	"github.com/BurntSushi/toml"
	"github.com/akitanabe/gunte/internal/typeddata"
)

// ParseFrontmatter parses only the TOML bytes between a source's +++ delimiters.
func ParseFrontmatter(path string, buffer []byte, frontmatter Range) (map[string]any, []Diagnostic) {
	values, _, diagnostics := parseFrontmatter(path, buffer, frontmatter)
	return values, diagnostics
}

func parseFrontmatter(path string, buffer []byte, frontmatter Range) (map[string]any, *typeddata.Value, []Diagnostic) {
	if frontmatter.Start < 0 || frontmatter.End < frontmatter.Start || frontmatter.End > len(buffer) {
		return nil, nil, []Diagnostic{withPath(path, diagnosticAt(buffer, frontmatter.Start, "invalid frontmatter range"))}
	}
	openingEnd := indexLineEnd(buffer, frontmatter.Start)
	if openingEnd < 0 || openingEnd >= frontmatter.End {
		return nil, nil, []Diagnostic{withPath(path, diagnosticAt(buffer, frontmatter.Start, "invalid frontmatter delimiter"))}
	}
	closingStart, _, found := findClosingDelimiter(buffer, openingEnd+1)
	if !found || closingStart >= frontmatter.End {
		return nil, nil, []Diagnostic{withPath(path, diagnosticAt(buffer, frontmatter.Start, "invalid frontmatter delimiter"))}
	}

	content := buffer[openingEnd+1 : closingStart]
	values := map[string]any{}
	metadata, err := toml.Decode(string(content), &values)
	if err == nil {
		node, ok := typeddata.OrderedTOMLValue(values, nil, metadata.Keys())
		if !ok {
			return values, nil, nil
		}
		return values, &node, nil
	}
	diagnostic := diagnosticAt(buffer, openingEnd+1, "invalid TOML frontmatter")
	var parseError toml.ParseError
	if errors.As(err, &parseError) {
		offset := openingEnd + 1 + parseError.Position.Start
		diagnostic = diagnosticAt(buffer, offset, parseError.Message)
	}
	diagnostic.Path = path
	return nil, nil, []Diagnostic{diagnostic}
}

func indexLineEnd(buffer []byte, start int) int {
	if start < 0 || start >= len(buffer) {
		return -1
	}
	for index := start; index < len(buffer); index++ {
		if buffer[index] == '\n' {
			return index
		}
	}
	return -1
}

func withPath(path string, diagnostic Diagnostic) Diagnostic {
	diagnostic.Path = path
	return diagnostic
}
