package lexer

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/akitanabe/gunte/internal/source"
)

type openBlock struct {
	kind  MarkerKind
	index int
}

type fence struct {
	character byte
	length    int
}

// Lex recognizes directives and term uses in bodyRange. It is a pure
// calculation over a normalized source buffer; all returned ranges and
// positions refer to that buffer rather than to pre-normalized input.
func Lex(path string, buffer []byte, bodyRange source.Range) (IR, []source.Diagnostic) {
	if bodyRange.Start < 0 || bodyRange.End < bodyRange.Start || bodyRange.End > len(buffer) {
		return IR{}, []source.Diagnostic{diagnostic(path, buffer, clamp(bodyRange.Start, 0, len(buffer)), "body range is outside the normalized buffer")}
	}

	var ir IR
	var diagnostics []source.Diagnostic
	var activeFence *fence
	var stack []openBlock

	for lineStart := bodyRange.Start; lineStart < bodyRange.End; {
		lineEnd := bodyRange.End
		if relativeLF := bytes.IndexByte(buffer[lineStart:bodyRange.End], '\n'); relativeLF >= 0 {
			lineEnd = lineStart + relativeLF + 1
		}
		line := buffer[lineStart:lineEnd]
		lineWithoutLF := line
		if len(lineWithoutLF) > 0 && lineWithoutLF[len(lineWithoutLF)-1] == '\n' {
			lineWithoutLF = lineWithoutLF[:len(lineWithoutLF)-1]
		}

		if activeFence != nil {
			if isFenceClose(lineWithoutLF, *activeFence) {
				activeFence = nil
			}
			lineStart = lineEnd
			continue
		}
		if opening, ok := fenceOpen(lineWithoutLF); ok {
			activeFence = &opening
			lineStart = lineEnd
			continue
		}

		kind, token, ok := parseDirective(lineWithoutLF)
		if ok {
			marker := Marker{
				Kind:     kind,
				Token:    token,
				Range:    source.Range{Start: lineStart, End: lineEnd},
				Position: positionAt(buffer, lineStart),
			}
			ir.Markers = append(ir.Markers, marker)
			diagnostics = append(diagnostics, applyMarker(path, marker, &ir, &stack)...)
			lineStart = lineEnd
			continue
		}

		ir.TermUses = append(ir.TermUses, scanTerms(buffer, lineStart, lineEnd)...)
		lineStart = lineEnd
	}

	for index := len(stack) - 1; index >= 0; index-- {
		open := stack[index]
		block := blockAt(&ir, open)
		block.ContentRange.End = bodyRange.End
		name := "contract"
		if open.kind == OnlyOpen {
			name = "only block"
		}
		diagnostics = append(diagnostics, diagnosticAtMarker(path, block.Open, fmt.Sprintf("%s is not closed before EOF", name)))
	}

	return ir, diagnostics
}

func applyMarker(path string, marker Marker, ir *IR, stack *[]openBlock) []source.Diagnostic {
	switch marker.Kind {
	case AnchorMarker:
		ir.Anchors = append(ir.Anchors, Anchor{Token: marker.Token, Range: marker.Range, Position: marker.Position})
	case ContractOpen:
		if len(*stack) != 0 {
			if (*stack)[len(*stack)-1].kind == ContractOpen {
				return []source.Diagnostic{diagnosticAtMarker(path, marker, "contract cannot nest inside contract")}
			}
			return []source.Diagnostic{diagnosticAtMarker(path, marker, "contract cannot open inside only block")}
		}
		ir.ContractSpans = append(ir.ContractSpans, Block{Token: marker.Token, Open: marker, ContentRange: source.Range{Start: marker.Range.End}})
		*stack = append(*stack, openBlock{kind: ContractOpen, index: len(ir.ContractSpans) - 1})
	case OnlyOpen:
		if len(*stack) != 0 && (*stack)[len(*stack)-1].kind == OnlyOpen {
			return []source.Diagnostic{diagnosticAtMarker(path, marker, "only block cannot nest inside only block")}
		}
		ir.OnlyBlocks = append(ir.OnlyBlocks, Block{Token: marker.Token, Open: marker, ContentRange: source.Range{Start: marker.Range.End}})
		*stack = append(*stack, openBlock{kind: OnlyOpen, index: len(ir.OnlyBlocks) - 1})
	case ContractClose:
		return closeBlock(path, marker, ContractOpen, "contract", ir, stack)
	case OnlyClose:
		return closeBlock(path, marker, OnlyOpen, "only block", ir, stack)
	}
	return nil
}

func closeBlock(path string, marker Marker, want MarkerKind, name string, ir *IR, stack *[]openBlock) []source.Diagnostic {
	if len(*stack) == 0 {
		return []source.Diagnostic{diagnosticAtMarker(path, marker, fmt.Sprintf("%s close appears without an open %s", name, name))}
	}
	open := (*stack)[len(*stack)-1]
	if open.kind != want {
		other := "contract"
		if open.kind == OnlyOpen {
			other = "only block"
		}
		return []source.Diagnostic{diagnosticAtMarker(path, marker, fmt.Sprintf("%s close crosses an open %s", name, other))}
	}
	block := blockAt(ir, open)
	closeMarker := marker
	block.Close = &closeMarker
	block.ContentRange.End = marker.Range.Start
	*stack = (*stack)[:len(*stack)-1]
	return nil
}

func blockAt(ir *IR, open openBlock) *Block {
	if open.kind == ContractOpen {
		return &ir.ContractSpans[open.index]
	}
	return &ir.OnlyBlocks[open.index]
}

func parseDirective(line []byte) (MarkerKind, string, bool) {
	const prefix = "<!--"
	if !bytes.HasPrefix(line, []byte(prefix)) {
		return "", "", false
	}
	rest := line[len(prefix):]
	leading := countLeadingWhitespace(rest)
	if leading == 0 {
		return "", "", false
	}
	rest = rest[leading:]
	trimmedEnd := len(rest)
	for trimmedEnd > 0 && isWhitespace(rest[trimmedEnd-1]) {
		trimmedEnd--
	}
	trimmed := rest[:trimmedEnd]
	if !bytes.HasSuffix(trimmed, []byte("-->")) {
		return "", "", false
	}
	bodyWithWhitespace := trimmed[:len(trimmed)-len("-->")]
	bodyEnd := len(bodyWithWhitespace)
	for bodyEnd > 0 && isWhitespace(bodyWithWhitespace[bodyEnd-1]) {
		bodyEnd--
	}
	if bodyEnd == len(bodyWithWhitespace) {
		return "", "", false
	}
	body := string(bodyWithWhitespace[:bodyEnd])
	switch body {
	case "@/contract":
		return ContractClose, "", true
	case "@/only":
		return OnlyClose, "", true
	}
	for _, candidate := range []struct {
		keyword string
		kind    MarkerKind
	}{
		{keyword: "@contract", kind: ContractOpen},
		{keyword: "@anchor", kind: AnchorMarker},
		{keyword: "@only", kind: OnlyOpen},
	} {
		if strings.HasPrefix(body, candidate.keyword) {
			rest := []byte(body[len(candidate.keyword):])
			separatorLength := countLeadingWhitespace(rest)
			if separatorLength == 0 {
				continue
			}
			token := string(rest[separatorLength:])
			if token != "" && !strings.ContainsAny(token, " \t\r\n") {
				return candidate.kind, token, true
			}
		}
	}
	return "", "", false
}

func fenceOpen(line []byte) (fence, bool) {
	if len(line) < 3 || (line[0] != '`' && line[0] != '~') {
		return fence{}, false
	}
	length := countRun(line, line[0])
	if length < 3 {
		return fence{}, false
	}
	if line[0] == '`' && bytes.IndexByte(line[length:], '`') >= 0 {
		return fence{}, false
	}
	return fence{character: line[0], length: length}, true
}

func isFenceClose(line []byte, active fence) bool {
	trimmed := bytes.Trim(line, " \t")
	length := countRun(trimmed, active.character)
	if length < active.length {
		return false
	}
	return length == len(trimmed)
}

func scanTerms(buffer []byte, lineStart, lineEnd int) []TermUse {
	contentEnd := lineEnd
	if contentEnd > lineStart && buffer[contentEnd-1] == '\n' {
		contentEnd--
	}
	var uses []TermUse
	for offset := lineStart; offset+3 < contentEnd; {
		relativeOpen := bytes.Index(buffer[offset:contentEnd], []byte("{{"))
		if relativeOpen < 0 {
			break
		}
		start := offset + relativeOpen
		relativeClose := bytes.Index(buffer[start+2:contentEnd], []byte("}}"))
		if relativeClose < 0 {
			break
		}
		end := start + 2 + relativeClose + 2
		name := buffer[start+2 : end-2]
		if validTermName(name) {
			uses = append(uses, TermUse{Name: string(name), Range: source.Range{Start: start, End: end}, Position: positionAt(buffer, start)})
			offset = end
			continue
		}
		offset = start + 1
	}
	return uses
}

func validTermName(name []byte) bool {
	if len(name) == 0 || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	previousUnderscore := false
	for _, character := range name[1:] {
		switch {
		case character >= 'a' && character <= 'z', character >= '0' && character <= '9':
			previousUnderscore = false
		case character == '_' && !previousUnderscore:
			previousUnderscore = true
		default:
			return false
		}
	}
	return !previousUnderscore
}

func positionAt(buffer []byte, offset int) Position {
	line, column := source.LineColumn(buffer, offset)
	return Position{Offset: offset, Line: line, Column: column}
}

func diagnostic(path string, buffer []byte, offset int, message string) source.Diagnostic {
	position := positionAt(buffer, offset)
	return source.Diagnostic{Path: path, Offset: position.Offset, Line: position.Line, Column: position.Column, Message: message}
}

func diagnosticAtMarker(path string, marker Marker, message string) source.Diagnostic {
	return source.Diagnostic{Path: path, Offset: marker.Position.Offset, Line: marker.Position.Line, Column: marker.Position.Column, Message: message}
}

func countLeadingWhitespace(input []byte) int {
	count := 0
	for count < len(input) && isWhitespace(input[count]) {
		count++
	}
	return count
}

func countRun(input []byte, character byte) int {
	count := 0
	for count < len(input) && input[count] == character {
		count++
	}
	return count
}

func isWhitespace(character byte) bool {
	return character == ' ' || character == '\t'
}

func clamp(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
