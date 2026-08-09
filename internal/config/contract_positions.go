package config

import (
	"strconv"
	"strings"
)

type predicateOccurrence struct {
	id       string
	position ContractPosition
	concrete bool
	field    string
	implicit bool
}

type inlineAssignment struct {
	key         string
	keyParts    []string
	partOffsets []int
	keyOffset   int
	valueStart  int
	valueEnd    int
}

type inlinePredicatePosition struct {
	id     string
	offset int
	fields map[string]int
}

func predicateOccurrences(path string, input []byte) []predicateOccurrence {
	var result []predicateOccurrence
	var tablePath []string
	inMultiline := ""
	for index, raw := range strings.Split(string(input), "\n") {
		line, nextMultiline := tomlCode(raw, inMultiline)
		inMultiline = nextMultiline
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && !strings.HasPrefix(trimmed, "[[") {
			close := strings.IndexByte(trimmed, ']')
			if close > 0 {
				tablePath = tomlKeySegments(trimmed[1:close])
				if len(tablePath) == 2 && tablePath[0] == "contracts" {
					result = append(result, predicateOccurrence{id: tablePath[1], concrete: true, position: ContractPosition{Path: path, Line: index + 1, Column: strings.Index(raw, "[") + 1}})
				}
			}
			continue
		}
		equal := strings.IndexByte(trimmed, '=')
		if equal < 0 {
			continue
		}
		keyText := strings.TrimSpace(trimmed[:equal])
		keySegments := tomlKeySegments(keyText)
		segments := append(append([]string(nil), tablePath...), keySegments...)
		if len(tablePath) == 0 && len(keySegments) == 1 && keySegments[0] == "contracts" {
			rawEqual := strings.IndexByte(raw, '=')
			for _, predicate := range topLevelInlinePredicates(raw, rawEqual+1) {
				result = append(result, predicateOccurrence{id: predicate.id, concrete: true, position: ContractPosition{Path: path, Line: index + 1, Column: predicate.offset + 1}})
			}
			continue
		}
		if len(segments) < 2 || segments[0] != "contracts" {
			continue
		}
		concrete := len(segments) == 2
		field := ""
		if len(segments) >= 3 {
			field = segments[2]
		}
		column := strings.Index(raw, strings.TrimSpace(keyText)) + 1
		implicit := len(tablePath) < 2 && len(segments) >= 3
		result = append(result, predicateOccurrence{id: segments[1], concrete: concrete, field: field, implicit: implicit, position: ContractPosition{Path: path, Line: index + 1, Column: column}})
	}
	return result
}

func topLevelInlinePredicates(line string, valueStart int) []inlinePredicatePosition {
	if valueStart < 0 || valueStart > len(line) {
		return nil
	}
	assignments := inlineAssignments(line[valueStart:])
	result := make([]inlinePredicatePosition, 0, len(assignments))
	dotted := map[string]int{}
	for _, assignment := range assignments {
		if len(assignment.keyParts) >= 2 {
			index, exists := dotted[assignment.keyParts[0]]
			if !exists {
				index = len(result)
				dotted[assignment.keyParts[0]] = index
				result = append(result, inlinePredicatePosition{id: assignment.keyParts[0], offset: valueStart + assignment.keyOffset + assignment.partOffsets[0], fields: map[string]int{}})
			}
			result[index].fields[assignment.keyParts[1]] = valueStart + assignment.keyOffset + assignment.partOffsets[1]
			continue
		}
		fields := map[string]int{}
		predicateStart := valueStart + assignment.valueStart
		predicateValue := line[predicateStart : valueStart+assignment.valueEnd]
		for _, field := range inlineAssignments(predicateValue) {
			fields[field.key] = predicateStart + field.keyOffset
			if field.key == "assertions" {
				arrayValue := predicateValue[field.valueStart:field.valueEnd]
				for index, element := range inlineArrayTableFields(arrayValue) {
					for nestedField, offset := range element {
						fields[formatIndex("assertions", index)+"."+nestedField] = predicateStart + field.valueStart + offset
					}
				}
			}
		}
		result = append(result, inlinePredicatePosition{id: assignment.key, offset: valueStart + assignment.keyOffset, fields: fields})
	}
	return result
}

func inlineAssignments(value string) []inlineAssignment {
	open := strings.IndexByte(value, '{')
	if open < 0 {
		return nil
	}
	var result []inlineAssignment
	for cursor := open + 1; cursor < len(value); {
		for cursor < len(value) && (value[cursor] == ' ' || value[cursor] == '\t' || value[cursor] == ',') {
			cursor++
		}
		if cursor >= len(value) || value[cursor] == '}' {
			break
		}
		keyStart := cursor
		equal := inlineKeyEqual(value, cursor)
		if equal < 0 {
			break
		}
		keyText := strings.TrimSpace(value[keyStart:equal])
		segments := tomlKeySegments(keyText)
		if len(segments) != 1 {
			if len(segments) == 0 {
				break
			}
		}
		keyOffset := keyStart + strings.Index(value[keyStart:equal], strings.TrimSpace(keyText))
		partOffsets := tomlKeySegmentOffsets(keyText)
		valueStart := equal + 1
		for valueStart < len(value) && (value[valueStart] == ' ' || value[valueStart] == '\t') {
			valueStart++
		}
		valueEnd := inlineValueEnd(value, valueStart)
		result = append(result, inlineAssignment{key: segments[0], keyParts: segments, partOffsets: partOffsets, keyOffset: keyOffset, valueStart: valueStart, valueEnd: valueEnd})
		cursor = valueEnd
	}
	return result
}

func tomlKeySegmentOffsets(key string) []int {
	var result []int
	segmentStart := 0
	quote := byte(0)
	escaped := false
	appendOffset := func(end int) {
		for segmentStart < end && (key[segmentStart] == ' ' || key[segmentStart] == '\t') {
			segmentStart++
		}
		result = append(result, segmentStart)
	}
	for index := 0; index < len(key); index++ {
		character := key[index]
		if quote != 0 {
			if quote == '"' && character == '\\' && !escaped {
				escaped = true
				continue
			}
			if character == quote && !escaped {
				quote = 0
			}
			escaped = false
			continue
		}
		if character == '"' || character == '\'' {
			quote = character
			continue
		}
		if character == '.' {
			appendOffset(index)
			segmentStart = index + 1
		}
	}
	appendOffset(len(key))
	return result
}

func inlineKeyEqual(value string, start int) int {
	quote := byte(0)
	escaped := false
	for index := start; index < len(value); index++ {
		character := value[index]
		if quote != 0 {
			if quote == '"' && character == '\\' && !escaped {
				escaped = true
				continue
			}
			if character == quote && !escaped {
				quote = 0
			}
			escaped = false
			continue
		}
		if character == '"' || character == '\'' {
			quote = character
			continue
		}
		if character == '=' {
			return index
		}
		if character == ',' || character == '}' {
			return -1
		}
	}
	return -1
}

func inlineValueEnd(value string, start int) int {
	braces, brackets := 0, 0
	quote := byte(0)
	escaped := false
	for index := start; index < len(value); index++ {
		character := value[index]
		if quote != 0 {
			if quote == '"' && character == '\\' && !escaped {
				escaped = true
				continue
			}
			if character == quote && !escaped {
				quote = 0
			}
			escaped = false
			continue
		}
		if character == '"' || character == '\'' {
			quote = character
			continue
		}
		switch character {
		case '{':
			braces++
		case '}':
			if braces == 0 && brackets == 0 {
				return index
			}
			braces--
		case '[':
			brackets++
		case ']':
			brackets--
		case ',':
			if braces == 0 && brackets == 0 {
				return index
			}
		}
	}
	return len(value)
}

func locateContractField(input []byte, keyPath string) (int, int, bool) {
	segments := strings.Split(keyPath, ".")
	if len(segments) < 3 || segments[0] != "contracts" {
		return 0, 0, false
	}
	id, field := segments[1], strings.Join(segments[2:], ".")
	if len(segments) == 4 {
		if array, index, ok := indexedKeySegment(segments[2]); ok {
			if offset, found := locateInlineArrayElementField(input, id, array, index, segments[3]); found {
				return offsetLineColumn(input, offset)
			}
		}
	}
	var tablePath []string
	arrayIndexes := map[string]int{}
	inMultiline := ""
	for index, raw := range strings.Split(string(input), "\n") {
		line, nextMultiline := tomlCode(raw, inMultiline)
		inMultiline = nextMultiline
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[[") {
			if close := strings.Index(trimmed, "]]"); close > 1 {
				tablePath = tomlKeySegments(trimmed[2:close])
				key := strings.Join(tablePath, ".")
				index := arrayIndexes[key]
				arrayIndexes[key] = index + 1
				last := len(tablePath) - 1
				tablePath[last] = formatIndex(tablePath[last], index)
			}
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			if close := strings.IndexByte(trimmed, ']'); close > 0 {
				tablePath = tomlKeySegments(trimmed[1:close])
			}
			continue
		}
		equal := strings.IndexByte(trimmed, '=')
		if equal < 0 {
			continue
		}
		keyText := strings.TrimSpace(trimmed[:equal])
		keySegments := tomlKeySegments(keyText)
		full := append(append([]string(nil), tablePath...), keySegments...)
		if len(tablePath) == 0 && len(keySegments) == 1 && keySegments[0] == "contracts" {
			rawEqual := strings.IndexByte(raw, '=')
			for _, predicate := range topLevelInlinePredicates(raw, rawEqual+1) {
				if predicate.id == id {
					if column, exists := predicate.fields[field]; exists {
						return index + 1, column + 1, true
					}
				}
			}
		}
		if sameKeyPath(full, segments) {
			return index + 1, strings.Index(raw, keyText) + 1, true
		}
		if len(segments) == 3 && len(full) >= 3 && full[0] == "contracts" && full[1] == id && full[2] == field {
			return index + 1, strings.Index(raw, keyText) + 1, true
		}
		if len(full) == 2 && full[0] == "contracts" && full[1] == id {
			right := raw[strings.IndexByte(raw, '=')+1:]
			if column := inlineFieldColumn(right, field); column >= 0 {
				return index + 1, strings.IndexByte(raw, '=') + 2 + column, true
			}
		}
	}
	return 0, 0, false
}

func indexedKeySegment(segment string) (string, int, bool) {
	open := strings.LastIndexByte(segment, '[')
	if open <= 0 || !strings.HasSuffix(segment, "]") {
		return "", 0, false
	}
	index, err := strconv.Atoi(segment[open+1 : len(segment)-1])
	return segment[:open], index, err == nil
}

func locateInlineArrayElementField(input []byte, id, array string, wantedIndex int, field string) (int, bool) {
	text := string(input)
	var tablePath []string
	lineOffset := 0
	inMultiline := ""
	for _, raw := range strings.Split(text, "\n") {
		line, nextMultiline := tomlCode(raw, inMultiline)
		inMultiline = nextMultiline
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && !strings.HasPrefix(trimmed, "[[") {
			if close := strings.IndexByte(trimmed, ']'); close > 0 {
				tablePath = tomlKeySegments(trimmed[1:close])
			}
		} else if len(tablePath) == 2 && tablePath[0] == "contracts" && tablePath[1] == id {
			if equal := strings.IndexByte(trimmed, '='); equal >= 0 {
				key := strings.TrimSpace(trimmed[:equal])
				if parts := tomlKeySegments(key); len(parts) == 1 && parts[0] == array {
					rawEqual := strings.IndexByte(raw, '=')
					valueStart := lineOffset + rawEqual + 1
					elements := inlineArrayTableFields(text[valueStart:])
					if wantedIndex < len(elements) {
						if relative, exists := elements[wantedIndex][field]; exists {
							return valueStart + relative, true
						}
					}
				}
			}
		}
		lineOffset += len(raw) + 1
	}
	return 0, false
}

func inlineArrayTableFields(value string) []map[string]int {
	open := strings.IndexByte(value, '[')
	if open < 0 {
		return nil
	}
	var result []map[string]int
	for cursor := open + 1; cursor < len(value); cursor++ {
		if value[cursor] == '#' {
			if newline := strings.IndexByte(value[cursor:], '\n'); newline >= 0 {
				cursor += newline
				continue
			}
			break
		}
		if value[cursor] == ']' {
			break
		}
		if value[cursor] != '{' {
			continue
		}
		end := matchingInlineTableEnd(value, cursor)
		if end < 0 {
			return result
		}
		fields := map[string]int{}
		for _, assignment := range inlineAssignments(value[cursor : end+1]) {
			fields[assignment.key] = cursor + assignment.keyOffset
		}
		result = append(result, fields)
		cursor = end
	}
	return result
}

func matchingInlineTableEnd(value string, start int) int {
	depth := 0
	quote := byte(0)
	escaped := false
	for index := start; index < len(value); index++ {
		character := value[index]
		if quote != 0 {
			if quote == '"' && character == '\\' && !escaped {
				escaped = true
				continue
			}
			if character == quote && !escaped {
				quote = 0
			}
			escaped = false
			continue
		}
		if character == '"' || character == '\'' {
			quote = character
			continue
		}
		if character == '#' {
			if newline := strings.IndexByte(value[index:], '\n'); newline >= 0 {
				index += newline
				continue
			}
			return -1
		}
		if character == '{' {
			depth++
		}
		if character == '}' {
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func offsetLineColumn(input []byte, offset int) (int, int, bool) {
	if offset < 0 || offset > len(input) {
		return 0, 0, false
	}
	prefix := string(input[:offset])
	line := strings.Count(prefix, "\n") + 1
	lastNewline := strings.LastIndexByte(prefix, '\n')
	return line, offset - lastNewline, true
}

func sameKeyPath(first, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}

func inlineFieldColumn(value, field string) int {
	for offset := 0; offset < len(value); {
		found := strings.Index(value[offset:], field)
		if found < 0 {
			return -1
		}
		found += offset
		beforeOK := found == 0 || strings.ContainsRune("{, \t", rune(value[found-1]))
		after := strings.TrimLeft(value[found+len(field):], " \t")
		if beforeOK && strings.HasPrefix(after, "=") {
			return found
		}
		offset = found + len(field)
	}
	return -1
}

func tomlCode(line, multiline string) (string, string) {
	var result strings.Builder
	quote := byte(0)
	escaped := false
	for index := 0; index < len(line); index++ {
		if multiline != "" {
			if strings.HasPrefix(line[index:], multiline) {
				index += 2
				multiline = ""
			}
			continue
		}
		character := line[index]
		if quote != 0 {
			result.WriteByte(character)
			if quote == '"' && character == '\\' && !escaped {
				escaped = true
				continue
			}
			if character == quote && !escaped {
				quote = 0
			}
			escaped = false
			continue
		}
		if strings.HasPrefix(line[index:], `"""`) || strings.HasPrefix(line[index:], `'''`) {
			multiline = line[index : index+3]
			index += 2
			continue
		}
		if character == '#' {
			break
		}
		if character == '"' || character == '\'' {
			quote = character
		}
		result.WriteByte(character)
	}
	return result.String(), multiline
}

func tomlKeySegments(key string) []string {
	var result []string
	var current strings.Builder
	quote := byte(0)
	escaped := false
	flush := func() {
		value := strings.TrimSpace(current.String())
		current.Reset()
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		} else if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
			value = value[1 : len(value)-1]
		}
		result = append(result, value)
	}
	for index := 0; index < len(key); index++ {
		character := key[index]
		if quote == 0 && character == '.' {
			flush()
			continue
		}
		current.WriteByte(character)
		if quote != 0 {
			if quote == '"' && character == '\\' && !escaped {
				escaped = true
				continue
			}
			if character == quote && !escaped {
				quote = 0
			}
			escaped = false
		} else if character == '"' || character == '\'' {
			quote = character
		}
	}
	flush()
	return result
}
