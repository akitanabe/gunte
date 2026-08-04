package serialize

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/akitanabe/gunte/internal/adapter"
	"github.com/akitanabe/gunte/internal/compile"
	"github.com/akitanabe/gunte/internal/config"
	"github.com/akitanabe/gunte/internal/source"
)

var tokenPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

var yamlReservedWords = map[string]bool{
	"true": true, "false": true, "null": true, "yes": true, "no": true,
	"on": true, "off": true, "y": true, "n": true,
}

// Serialize converts one logical adapter artifact to canonical UTF-8 bytes.
// The returned declaration ranges refer to the returned artifact bytes.
func Serialize(input adapter.Artifact) (Artifact, []Diagnostic) {
	result := Artifact{
		TargetID:   input.TargetID,
		SourcePath: input.SourcePath,
		Path:       input.Path,
		Contracts:  declarations(input.Contracts),
		Anchors:    declarations(input.Anchors),
	}
	var diagnostics []Diagnostic
	var body []byte
	var mapping boundaryMap
	switch input.Profile {
	case config.ProfileMarkdown:
		if !validUTF8(input.Body) {
			diagnostics = append(diagnostics, diag("invalid_utf8", "markdown body is not valid UTF-8"))
			break
		}
		body, mapping = normalizeBody(input.Body)
		if input.Header != "" && (strings.ContainsAny(input.Header, "\r\n") || !validUTF8String(input.Header)) {
			diagnostics = append(diagnostics, diag("invalid_header", "markdown header must be a single-line string"))
			break
		}
		prefix := []byte(nil)
		if input.Header != "" {
			prefix = []byte(input.Header + "\n\n")
		}
		result.Bytes = append(prefix, body...)
		mapDeclarations(&result, input, func(offset int) int { return len(prefix) + mapping.at(offset) })
	case config.ProfileYAML:
		if !validUTF8(input.Body) {
			diagnostics = append(diagnostics, diag("invalid_utf8", "YAML body is not valid UTF-8"))
			break
		}
		if err := validateHeader(input.Header); err != nil {
			diagnostics = append(diagnostics, diag("invalid_header", err.Error()))
			break
		}
		if len(input.Metadata) == 0 {
			body, mapping = normalizeBody(input.Body)
			frontEnd, ok := yamlFrontmatterEnd(body)
			if !ok {
				diagnostics = append(diagnostics, diag("invalid_yaml_frontmatter", "YAML preserve branch requires a leading frontmatter block"))
				break
			}
			prefix := 0
			if input.Header != "" {
				prefix = len(input.Header) + 2
			}
			suffix := body[frontEnd:]
			trimmed := 0
			// Only the canonical separator may be collapsed; further blank lines are
			// source content and stripping them would change preserved projection bytes.
			if input.Header != "" && len(suffix) > 0 && suffix[0] == '\n' {
				trimmed = 1
				suffix = suffix[trimmed:]
			}
			result.Bytes = make([]byte, 0, len(body)+prefix)
			result.Bytes = append(result.Bytes, body[:frontEnd]...)
			if input.Header != "" {
				result.Bytes = append(result.Bytes, []byte(input.Header+"\n\n")...)
			}
			result.Bytes = append(result.Bytes, suffix...)
			mapDeclarations(&result, input, func(offset int) int {
				mapped := mapping.at(offset)
				if mapped >= frontEnd {
					if trimmed != 0 && mapped == frontEnd {
						// This removed separator boundary belongs after the inserted
						// header; ordinary trimming would point back into its last byte.
						return mapped + prefix
					}
					return mapped + prefix - trimmed
				}
				return mapped
			})
			break
		}
		metadata, err := renderYAMLMetadata(input.Metadata)
		if err != nil {
			diagnostics = append(diagnostics, diag("invalid_metadata", err.Error()))
			break
		}
		body, mapping = normalizeBody(input.Body)
		prefix := append([]byte("---\n"), metadata...)
		prefix = append(prefix, []byte("---\n")...)
		if input.Header != "" {
			prefix = append(prefix, []byte(input.Header+"\n\n")...)
		}
		result.Bytes = append(prefix, body...)
		mapDeclarations(&result, input, func(offset int) int { return len(prefix) + mapping.at(offset) })
	case config.ProfileTOML:
		if err := validateHeader(input.Header); err != nil {
			diagnostics = append(diagnostics, diag("invalid_header", err.Error()))
			break
		}
		if input.BodyField != "" && !validFieldName(input.BodyField) {
			diagnostics = append(diagnostics, diag("invalid_metadata", fmt.Sprintf("invalid TOML body field %q", input.BodyField)))
			break
		}
		for _, field := range input.Metadata {
			if field.Field == input.BodyField && input.BodyField != "" {
				diagnostics = append(diagnostics, diag("invalid_metadata", fmt.Sprintf("TOML body field %q conflicts with metadata", input.BodyField)))
				break
			}
		}
		if len(diagnostics) > 0 {
			break
		}
		metadata, err := renderTOMLMetadata(input.Metadata)
		if err != nil {
			diagnostics = append(diagnostics, diag("invalid_metadata", err.Error()))
			break
		}
		if !validUTF8(input.Body) {
			diagnostics = append(diagnostics, diag("invalid_utf8", "TOML body is not valid UTF-8"))
			break
		}
		body, mapping = normalizeBody(input.Body)
		var escaped []byte
		var bodyMap boundaryMap
		if input.BodyField != "" {
			escaped, bodyMap, err = escapeTOMLBody(body)
			if err != nil {
				diagnostics = append(diagnostics, diag("invalid_utf8", err.Error()))
				break
			}
		}
		prefix := []byte(nil)
		if input.Header != "" {
			prefix = []byte("# " + input.Header + "\n")
		}
		prefix = append(prefix, metadata...)
		if input.BodyField != "" {
			prefix = append(prefix, []byte(input.BodyField+" = \"\"\"\n")...)
			result.Bytes = append(prefix, escaped...)
			result.Bytes = append(result.Bytes, []byte("\"\"\"\n")...)
			mapDeclarations(&result, input, func(offset int) int {
				return len(prefix) + bodyMap.at(mapping.at(offset))
			})
		} else {
			result.Bytes = prefix
			hideDeclarations(&result)
		}
	case config.ProfileJSON:
		result.Bytes, mapping, diagnostics = serializeJSON(input)
		if len(diagnostics) == 0 {
			mapDeclarations(&result, input, func(offset int) int { return mapping.at(offset) })
		}
	case config.ProfilePlainText:
		if input.Value == nil || input.Value.Type != config.MetadataString || input.Value.String == "" || strings.ContainsAny(input.Value.String, "\r\n") {
			diagnostics = append(diagnostics, diag("invalid_value", "plain-text value must be a non-empty single-line string"))
			break
		}
		if !validUTF8String(input.Value.String) {
			diagnostics = append(diagnostics, diag("invalid_utf8", "plain-text value is not valid UTF-8"))
			break
		}
		result.Bytes = []byte(input.Value.String + "\n")
		hideDeclarations(&result)
	default:
		diagnostics = append(diagnostics, diag("unsupported_profile", fmt.Sprintf("unsupported profile %q", input.Profile)))
	}
	if len(diagnostics) > 0 {
		result.Bytes = nil
	}
	for index := range diagnostics {
		diagnostics[index].Path = input.Path
	}
	return result, diagnostics
}

func diag(code, message string) Diagnostic { return Diagnostic{Code: code, Message: message} }

func validateHeader(header string) error {
	if strings.ContainsAny(header, "\r\n") {
		return fmt.Errorf("header must be a single-line string")
	}
	if !validUTF8String(header) {
		return fmt.Errorf("header is not valid UTF-8")
	}
	return nil
}

func validUTF8(value []byte) bool { return utf8.Valid(value) }

func validUTF8String(value string) bool { return utf8.ValidString(value) }

func declarations(input []compile.ProjectedDeclaration) []Declaration {
	result := make([]Declaration, len(input))
	for index, declaration := range input {
		result[index] = Declaration{ID: declaration.ID, Source: declaration.Source, Emitted: declaration.Emitted}
	}
	return result
}

type boundaryMap struct{ values []int }

func (m boundaryMap) at(offset int) int {
	if offset < 0 {
		return 0
	}
	if offset >= len(m.values) {
		return m.values[len(m.values)-1]
	}
	return m.values[offset]
}

func normalizeBody(input []byte) ([]byte, boundaryMap) {
	if !utf8.Valid(input) {
		return nil, boundaryMap{values: []int{0}}
	}
	output := make([]byte, 0, len(input)+1)
	boundaries := make([]int, len(input)+1)
	start := 0
	if len(input) >= 3 && input[0] == 0xef && input[1] == 0xbb && input[2] == 0xbf {
		boundaries[0], boundaries[1], boundaries[2], boundaries[3] = 0, 0, 0, 0
		start = 3
	}
	for i := start; i < len(input); {
		boundaries[i] = len(output)
		if input[i] == '\r' {
			if i+1 < len(input) && input[i+1] == '\n' {
				boundaries[i+1] = len(output)
				i++
			}
			output = append(output, '\n')
			i++
			continue
		}
		output = append(output, input[i])
		i++
	}
	boundaries[len(input)] = len(output)
	for len(output) > 0 && output[len(output)-1] == '\n' {
		output = output[:len(output)-1]
	}
	output = append(output, '\n')
	for i := range boundaries {
		if boundaries[i] > len(output) {
			boundaries[i] = len(output)
		}
	}
	return output, boundaryMap{values: boundaries}
}

func mapDeclarations(result *Artifact, input adapter.Artifact, mapOffset func(int) int) {
	for i := range result.Contracts {
		if !input.Contracts[i].Emitted {
			continue
		}
		result.Contracts[i] = Declaration{
			ID:            input.Contracts[i].ID,
			Source:        input.Contracts[i].Source,
			Emitted:       true,
			ArtifactRange: mapRange(input.Contracts[i].ProjectedRange, mapOffset),
		}
	}
	for i := range result.Anchors {
		if !input.Anchors[i].Emitted {
			continue
		}
		result.Anchors[i] = Declaration{
			ID:            input.Anchors[i].ID,
			Source:        input.Anchors[i].Source,
			Emitted:       true,
			ArtifactRange: mapRange(input.Anchors[i].ProjectedRange, mapOffset),
		}
	}
}

func hideDeclarations(result *Artifact) {
	for index := range result.Contracts {
		result.Contracts[index].Emitted = false
		result.Contracts[index].ArtifactRange = source.Range{}
	}
	for index := range result.Anchors {
		result.Anchors[index].Emitted = false
		result.Anchors[index].ArtifactRange = source.Range{}
	}
}

func mapRange(value source.Range, mapOffset func(int) int) source.Range {
	return source.Range{Start: mapOffset(value.Start), End: mapOffset(value.End)}
}

func escapeString(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", fmt.Errorf("string is not valid UTF-8")
	}
	var output strings.Builder
	for _, character := range value {
		switch character {
		case '"':
			output.WriteString(`\"`)
		case '\\':
			output.WriteString(`\\`)
		case '\b':
			output.WriteString(`\b`)
		case '\t':
			output.WriteString(`\t`)
		case '\n':
			output.WriteString(`\n`)
		case '\f':
			output.WriteString(`\f`)
		case '\r':
			output.WriteString(`\r`)
		default:
			if (character >= 0 && character <= 0x1f) || character == 0x7f {
				var encoded [2]byte
				hex.Encode(encoded[:], []byte{byte(character)})
				output.WriteString(`\u00`)
				output.Write(encoded[:])
			} else {
				output.WriteRune(character)
			}
		}
	}
	return output.String(), nil
}

func escapeTOMLBody(input []byte) ([]byte, boundaryMap, error) {
	output := make([]byte, 0, len(input))
	boundaries := make([]int, len(input)+1)
	for i := 0; i < len(input); {
		character, width := utf8.DecodeRune(input[i:])
		if character == utf8.RuneError && width == 1 {
			return nil, boundaryMap{}, fmt.Errorf("body is not valid UTF-8")
		}
		outStart := len(output)
		switch character {
		case '\\':
			output = append(output, '\\', '\\')
		case '"':
			output = append(output, '\\', '"')
		case '\n':
			output = append(output, '\n')
		default:
			if character < 0x20 || character == 0x7f {
				escaped, _ := escapeString(string(character))
				output = append(output, escaped...)
			} else {
				output = append(output, input[i:i+width]...)
			}
		}
		for index := 0; index < width; index++ {
			boundaries[i+index] = outStart
		}
		boundaries[i+width] = len(output)
		i += width
	}
	return output, boundaryMap{values: boundaries}, nil
}
