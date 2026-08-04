package serialize

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/akitanabe/gunte/internal/adapter"
	"github.com/akitanabe/gunte/internal/config"
)

type jsonNode struct {
	kind         byte
	start        int
	end          int
	sourceBacked bool
	string       string
	parts        []jsonStringPart
	object       []jsonMember
	array        []*jsonNode
}

type jsonStringPart struct {
	start int
	end   int
	value string
}

type jsonMember struct {
	key              string
	keyStart, keyEnd int
	keyParts         []jsonStringPart
	value            *jsonNode
	start, end       int
}

type jsonParser struct {
	data []byte
	pos  int
}

func serializeJSON(input adapter.Artifact) ([]byte, boundaryMap, []Diagnostic) {
	body := input.Body
	if !utf8.Valid(body) {
		return nil, boundaryMap{}, []Diagnostic{diag("invalid_utf8", "JSON body is not valid UTF-8")}
	}
	originalEmpty := len(strings.TrimSpace(string(body))) == 0
	if originalEmpty {
		body = []byte("{}")
	}
	parser := jsonParser{data: body}
	root, err := parser.parseValue()
	if err == nil {
		parser.skipSpace()
		if parser.pos != len(body) {
			err = fmt.Errorf("trailing JSON data at byte %d", parser.pos)
		}
	}
	if err != nil {
		return nil, boundaryMap{}, []Diagnostic{diag("invalid_json", err.Error())}
	}
	if root.kind != '{' {
		return nil, boundaryMap{}, []Diagnostic{diag("invalid_json", "JSON top-level value must be an object")}
	}
	metadata, err := jsonMetadata(input.Metadata)
	if err != nil {
		return nil, boundaryMap{}, []Diagnostic{diag("invalid_metadata", err.Error())}
	}
	writer := jsonWriter{boundaries: make([]int, len(body)+1)}
	for i := range writer.boundaries {
		writer.boundaries[i] = -1
	}
	writer.writeObject(root, metadata, 0)
	writer.buf = append(writer.buf, '\n')
	if originalEmpty {
		writer.boundaries[0] = len(writer.buf)
	} else {
		writer.boundaries[0] = 0
	}
	writer.boundaries[len(body)] = len(writer.buf)
	last := 0
	for i := range writer.boundaries {
		if writer.boundaries[i] < 0 {
			writer.boundaries[i] = last
		} else {
			last = writer.boundaries[i]
		}
	}
	return writer.buf, boundaryMap{values: writer.boundaries}, nil
}

type jsonMetadataField struct {
	field string
	node  *jsonNode
}

func jsonMetadata(fields []adapter.MetadataField) ([]jsonMetadataField, error) {
	result := make([]jsonMetadataField, 0, len(fields))
	seen := map[string]bool{}
	for _, field := range fields {
		if !validFieldName(field.Field) || seen[field.Field] {
			return nil, fmt.Errorf("invalid or duplicate JSON metadata field %q", field.Field)
		}
		seen[field.Field] = true
		var node *jsonNode
		switch field.Value.Type {
		case config.MetadataString:
			if field.Value.String == "" {
				return nil, fmt.Errorf("JSON string metadata %s is empty", field.Field)
			}
			node = &jsonNode{kind: 's', string: field.Value.String}
		case config.MetadataStringList:
			node = &jsonNode{kind: '[', array: make([]*jsonNode, len(field.Value.Strings))}
			for i, item := range field.Value.Strings {
				node.array[i] = &jsonNode{kind: 's', string: item}
			}
		default:
			return nil, fmt.Errorf("metadata type %q is not supported by JSON", field.Value.Type)
		}
		if !utf8.ValidString(field.Value.String) {
			return nil, fmt.Errorf("metadata field %s is not valid UTF-8", field.Field)
		}
		for _, item := range field.Value.Strings {
			if !utf8.ValidString(item) {
				return nil, fmt.Errorf("metadata field %s is not valid UTF-8", field.Field)
			}
		}
		result = append(result, jsonMetadataField{field: field.Field, node: node})
	}
	return result, nil
}

type jsonWriter struct {
	buf        []byte
	boundaries []int
}

func (w *jsonWriter) writeObject(node *jsonNode, metadata []jsonMetadataField, indent int) {
	startOut := len(w.buf)
	members := make([]jsonMember, len(node.object))
	copy(members, node.object)
	metadataPresent := make(map[string]int, len(metadata))
	for i, field := range metadata {
		metadataPresent[field.field] = i
	}
	used := make([]bool, len(metadata))
	if len(members) == 0 && len(metadata) == 0 {
		w.buf = append(w.buf, '{', '}')
		if node.sourceBacked {
			w.mark(node.start, node.end, startOut, len(w.buf))
		}
		return
	}
	w.buf = append(w.buf, '{', '\n')
	count := 0
	previousMemberEnd := -1
	for i := range members {
		if count > 0 {
			separatorStart := len(w.buf)
			w.buf = append(w.buf, ',', '\n')
			if node.sourceBacked {
				w.mark(previousMemberEnd, members[i].start, separatorStart, len(w.buf))
			}
		}
		lineStart := len(w.buf)
		w.writeIndent(indent + 1)
		if node.sourceBacked {
			w.mark(members[i].start, members[i].keyStart, lineStart, len(w.buf))
		}
		w.writeKey(members[i])
		w.buf = append(w.buf, ':', ' ')
		value := members[i].value
		if metadataIndex, ok := metadataPresent[members[i].key]; ok {
			used[metadataIndex] = true
			valueStart := len(w.buf)
			w.writeNode(metadata[metadataIndex].node, indent+1)
			if node.sourceBacked {
				w.mark(value.start, value.end, valueStart, len(w.buf))
			}
		} else {
			w.writeNode(value, indent+1)
		}
		count++
		previousMemberEnd = members[i].end
	}
	for i, field := range metadata {
		if used[i] {
			continue
		}
		if count > 0 {
			w.buf = append(w.buf, ',', '\n')
		}
		w.writeIndent(indent + 1)
		w.buf = append(w.buf, '"')
		escaped, _ := escapeString(field.field)
		w.buf = append(w.buf, escaped...)
		w.buf = append(w.buf, '"', ':', ' ')
		w.writeNode(field.node, indent+1)
		count++
	}
	w.buf = append(w.buf, '\n')
	closeLineStart := len(w.buf)
	w.writeIndent(indent)
	w.buf = append(w.buf, '}')
	lastSourceEnd := node.start + 1
	if len(node.object) > 0 {
		lastSourceEnd = node.object[len(node.object)-1].end
	}
	if node.sourceBacked {
		w.mark(lastSourceEnd, node.end, closeLineStart, len(w.buf))
		w.mark(node.start, node.end, startOut, len(w.buf))
	}
}

func (w *jsonWriter) writeNode(node *jsonNode, indent int) {
	startOut := len(w.buf)
	switch node.kind {
	case 's':
		w.writeString(node)
	case '[':
		if len(node.array) == 0 {
			w.buf = append(w.buf, '[', ']')
			if node.sourceBacked {
				w.mark(node.start, node.end, startOut, len(w.buf))
			}
			return
		}
		w.buf = append(w.buf, '[', '\n')
		previousEnd := -1
		for i, child := range node.array {
			if i > 0 {
				separatorStart := len(w.buf)
				w.buf = append(w.buf, ',', '\n')
				if node.sourceBacked {
					w.mark(previousEnd, child.start, separatorStart, len(w.buf))
				}
			}
			w.writeIndent(indent + 1)
			w.writeNode(child, indent+1)
			previousEnd = child.end
		}
		w.buf = append(w.buf, '\n')
		w.writeIndent(indent)
		w.buf = append(w.buf, ']')
	case '{':
		w.writeObject(node, nil, indent)
		return
	}
	if node.sourceBacked {
		w.mark(node.start, node.end, startOut, len(w.buf))
	}
}

func (w *jsonWriter) writeString(node *jsonNode) {
	startOut := len(w.buf)
	w.buf = append(w.buf, '"')
	contentStart := len(w.buf)
	if len(node.parts) == 0 {
		escaped, _ := escapeString(node.string)
		w.buf = append(w.buf, escaped...)
	} else {
		for _, part := range node.parts {
			partStart := len(w.buf)
			escaped, _ := escapeString(part.value)
			w.buf = append(w.buf, escaped...)
			if node.sourceBacked {
				w.mark(part.start, part.end, partStart, len(w.buf))
			}
		}
	}
	contentEnd := len(w.buf)
	w.buf = append(w.buf, '"')
	if node.sourceBacked && len(node.parts) == 0 && node.start+1 <= node.end-1 {
		w.mark(node.start+1, node.end-1, contentStart, contentEnd)
	}
	if node.sourceBacked {
		w.mark(node.start, node.start+1, startOut, contentStart)
		w.mark(node.end-1, node.end, contentEnd, len(w.buf))
	}
}

func (w *jsonWriter) writeKey(member jsonMember) {
	startOut := len(w.buf)
	w.buf = append(w.buf, '"')
	if len(member.keyParts) == 0 {
		escaped, _ := escapeString(member.key)
		w.buf = append(w.buf, escaped...)
	} else {
		for _, part := range member.keyParts {
			partStart := len(w.buf)
			escaped, _ := escapeString(part.value)
			w.buf = append(w.buf, escaped...)
			w.mark(part.start, part.end, partStart, len(w.buf))
		}
	}
	w.buf = append(w.buf, '"')
	w.mark(member.keyStart, member.keyEnd, startOut, len(w.buf))
}

func (w *jsonWriter) writeIndent(level int) {
	w.buf = append(w.buf, []byte(strings.Repeat("  ", level))...)
}

func (w *jsonWriter) mark(start, end, outputStart, outputEnd int) {
	if start < 0 || end < start || start >= len(w.boundaries) {
		return
	}
	if end >= len(w.boundaries) {
		end = len(w.boundaries) - 1
	}
	for i := start; i < end; i++ {
		if w.boundaries[i] < 0 {
			w.boundaries[i] = outputStart
		}
	}
	w.boundaries[end] = outputEnd
}

func (p *jsonParser) skipSpace() {
	for p.pos < len(p.data) {
		switch p.data[p.pos] {
		case ' ', '\t', '\r', '\n':
			p.pos++
		default:
			return
		}
	}
}

func (p *jsonParser) parseValue() (*jsonNode, error) {
	p.skipSpace()
	if p.pos >= len(p.data) {
		return nil, fmt.Errorf("JSON value is missing")
	}
	start := p.pos
	switch p.data[p.pos] {
	case '{':
		return p.parseObject(start)
	case '[':
		return p.parseArray(start)
	case '"':
		value, parts, end, err := p.parseString()
		if err != nil {
			return nil, err
		}
		return &jsonNode{kind: 's', start: start, end: end, sourceBacked: true, string: value, parts: parts}, nil
	default:
		return nil, fmt.Errorf("JSON scalar values other than strings are unsupported at byte %d", start)
	}
}

func (p *jsonParser) parseObject(start int) (*jsonNode, error) {
	p.pos++
	node := &jsonNode{kind: '{', start: start, sourceBacked: true}
	seen := map[string]bool{}
	p.skipSpace()
	if p.pos < len(p.data) && p.data[p.pos] == '}' {
		p.pos++
		node.end = p.pos
		return node, nil
	}
	for {
		memberStart := p.pos
		p.skipSpace()
		if p.pos >= len(p.data) || p.data[p.pos] != '"' {
			return nil, fmt.Errorf("JSON object member key is missing at byte %d", p.pos)
		}
		keyStart := p.pos
		key, keyParts, keyEnd, err := p.parseString()
		if err != nil {
			return nil, err
		}
		if seen[key] {
			return nil, fmt.Errorf("duplicate JSON object member %q", key)
		}
		seen[key] = true
		p.skipSpace()
		if p.pos >= len(p.data) || p.data[p.pos] != ':' {
			return nil, fmt.Errorf("JSON object member %q lacks colon", key)
		}
		p.pos++
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		member := jsonMember{key: key, keyStart: keyStart, keyEnd: keyEnd, keyParts: keyParts, value: value, start: memberStart, end: value.end}
		node.object = append(node.object, member)
		p.skipSpace()
		if p.pos >= len(p.data) {
			return nil, fmt.Errorf("JSON object is not closed")
		}
		if p.data[p.pos] == '}' {
			p.pos++
			node.end = p.pos
			return node, nil
		}
		if p.data[p.pos] != ',' {
			return nil, fmt.Errorf("JSON object member separator is missing at byte %d", p.pos)
		}
		p.pos++
	}
}

func (p *jsonParser) parseArray(start int) (*jsonNode, error) {
	p.pos++
	node := &jsonNode{kind: '[', start: start, sourceBacked: true}
	p.skipSpace()
	if p.pos < len(p.data) && p.data[p.pos] == ']' {
		p.pos++
		node.end = p.pos
		return node, nil
	}
	for {
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		node.array = append(node.array, value)
		p.skipSpace()
		if p.pos >= len(p.data) {
			return nil, fmt.Errorf("JSON array is not closed")
		}
		if p.data[p.pos] == ']' {
			p.pos++
			node.end = p.pos
			return node, nil
		}
		if p.data[p.pos] != ',' {
			return nil, fmt.Errorf("JSON array element separator is missing at byte %d", p.pos)
		}
		p.pos++
	}
}

func (p *jsonParser) parseString() (string, []jsonStringPart, int, error) {
	if p.data[p.pos] != '"' {
		return "", nil, p.pos, fmt.Errorf("JSON string is missing at byte %d", p.pos)
	}
	p.pos++
	var value strings.Builder
	parts := make([]jsonStringPart, 0)
	for p.pos < len(p.data) {
		character := p.data[p.pos]
		if character == '"' {
			p.pos++
			return value.String(), parts, p.pos, nil
		}
		if character < 0x20 {
			return "", nil, p.pos, fmt.Errorf("unescaped control character in JSON string at byte %d", p.pos)
		}
		if character == '\\' {
			partStart := p.pos
			decoded, err := p.parseEscape()
			if err != nil {
				return "", nil, p.pos, err
			}
			value.WriteRune(decoded)
			parts = append(parts, jsonStringPart{start: partStart, end: p.pos, value: string(decoded)})
			continue
		}
		partStart := p.pos
		runeValue, width := utf8.DecodeRune(p.data[p.pos:])
		if runeValue == utf8.RuneError && width == 1 {
			return "", nil, p.pos, fmt.Errorf("invalid UTF-8 in JSON string at byte %d", p.pos)
		}
		value.WriteRune(runeValue)
		p.pos += width
		parts = append(parts, jsonStringPart{start: partStart, end: p.pos, value: string(runeValue)})
	}
	return "", nil, p.pos, fmt.Errorf("JSON string is not closed")
}

func (p *jsonParser) parseEscape() (rune, error) {
	start := p.pos
	p.pos++
	if p.pos >= len(p.data) {
		return 0, fmt.Errorf("JSON escape is incomplete")
	}
	switch character := p.data[p.pos]; character {
	case '"', '\\', '/':
		p.pos++
		return rune(character), nil
	case 'b':
		p.pos++
		return '\b', nil
	case 'f':
		p.pos++
		return '\f', nil
	case 'n':
		p.pos++
		return '\n', nil
	case 'r':
		p.pos++
		return '\r', nil
	case 't':
		p.pos++
		return '\t', nil
	case 'u':
		value, err := p.parseUnicodeEscape()
		if err != nil {
			return 0, err
		}
		if value >= 0xd800 && value <= 0xdbff {
			if p.pos+1 >= len(p.data) || p.data[p.pos] != '\\' || p.data[p.pos+1] != 'u' {
				return 0, fmt.Errorf("lone high surrogate at byte %d", start)
			}
			p.pos++
			low, err := p.parseUnicodeEscape()
			if err != nil {
				return 0, err
			}
			if low < 0xdc00 || low > 0xdfff {
				return 0, fmt.Errorf("invalid surrogate pair at byte %d", start)
			}
			return rune(0x10000 + ((value - 0xd800) << 10) + (low - 0xdc00)), nil
		}
		if value >= 0xdc00 && value <= 0xdfff {
			return 0, fmt.Errorf("lone low surrogate at byte %d", start)
		}
		return rune(value), nil
	default:
		return 0, fmt.Errorf("invalid JSON escape \\%c", character)
	}
}

func (p *jsonParser) parseUnicodeEscape() (uint32, error) {
	if p.pos >= len(p.data) || p.data[p.pos] != 'u' || p.pos+4 >= len(p.data) {
		return 0, fmt.Errorf("JSON unicode escape is incomplete")
	}
	var value uint32
	for i := 1; i <= 4; i++ {
		digit, ok := hexDigit(p.data[p.pos+i])
		if !ok {
			return 0, fmt.Errorf("invalid JSON unicode escape")
		}
		value = value*16 + uint32(digit)
	}
	p.pos += 5
	return value, nil
}

func hexDigit(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}
