package config

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
	"github.com/akitanabe/gunte/internal/canonicaljson"
)

// ParseProject parses and validates gunte.toml bytes without performing I/O.
func ParseProject(path string, input []byte) (ProjectConfig, []Diagnostic) {
	root, order, diagnostic := parseDocument(path, input)
	if diagnostic != nil {
		return ProjectConfig{}, []Diagnostic{*diagnostic}
	}
	v := validator{path: path, input: input}
	config := v.project(root, order)
	return config, v.diagnostics
}

// ParseContracts parses and validates contracts.toml bytes without performing I/O.
func ParseContracts(path string, input []byte, knownTargets []string) (ContractRegistry, []Diagnostic) {
	return ParseContractsForSpec(path, input, knownTargets, 1)
}

// ParseContractsForSpec rejects v2-only fields and predicates for v1 and enables them for v2.
func ParseContractsForSpec(path string, input []byte, knownTargets []string, specVersion int) (ContractRegistry, []Diagnostic) {
	root, order, diagnostic := parseDocument(path, input)
	if diagnostic != nil {
		return ContractRegistry{}, []Diagnostic{*diagnostic}
	}
	v := validator{path: path, input: input, specVersion: specVersion}
	registry := v.contracts(root, order, knownTargets)
	if specVersion == 2 {
		v.validateV2PredicateIDs(registry)
	}
	return registry, v.diagnostics
}

// ParseContractDocuments validates selected documents and folds their registries in normative order.
func ParseContractDocuments(documents []ContractDocument, knownTargets []string, specVersion int) (ContractRegistry, []Diagnostic) {
	registry := ContractRegistry{}
	seen := map[string]ContractPosition{}
	var diagnostics []Diagnostic
	for _, document := range documents {
		parsed, current := ParseContractsForSpec(document.Path, document.Bytes, knownTargets, specVersion)
		if onlyDuplicateParseFailure(current) {
			candidateSeen := clonePositions(seen)
			duplicates := duplicateDiagnosticsFromOccurrences(document, candidateSeen)
			if len(duplicates) != 0 {
				seen = candidateSeen
				diagnostics = append(diagnostics, duplicates...)
				continue
			}
		}
		diagnostics = append(diagnostics, current...)
		for _, predicate := range parsed.Contracts {
			if original, exists := seen[predicate.ID]; exists {
				diagnostics = append(diagnostics, Diagnostic{Path: predicate.Position.Path, Line: predicate.Position.Line, Column: predicate.Position.Column, Related: []ContractPosition{original}, Message: "duplicate predicate " + predicate.ID})
				continue
			}
			seen[predicate.ID] = predicate.Position
			registry.Contracts = append(registry.Contracts, predicate)
		}
	}
	return registry, diagnostics
}

func clonePositions(input map[string]ContractPosition) map[string]ContractPosition {
	result := make(map[string]ContractPosition, len(input))
	for id, position := range input {
		result[id] = position
	}
	return result
}

func duplicateDiagnosticsFromOccurrences(document ContractDocument, seen map[string]ContractPosition) []Diagnostic {
	concrete := map[string]bool{}
	for id := range seen {
		concrete[id] = true
	}
	implicitHere := map[string]bool{}
	explicitHere := map[string]bool{}
	var diagnostics []Diagnostic
	for _, occurrence := range predicateOccurrences(document.Path, document.Bytes) {
		original, exists := seen[occurrence.id]
		if !exists {
			seen[occurrence.id] = occurrence.position
			original = occurrence.position
		}
		becomesConcrete := occurrence.concrete || (occurrence.implicit && occurrence.field == "kind")
		firstExplicitization := occurrence.concrete && implicitHere[occurrence.id] && !explicitHere[occurrence.id]
		if becomesConcrete && concrete[occurrence.id] && !firstExplicitization {
			diagnostics = append(diagnostics, Diagnostic{Path: occurrence.position.Path, Line: occurrence.position.Line, Column: occurrence.position.Column, Related: []ContractPosition{original}, Message: "duplicate predicate " + occurrence.id})
			continue
		}
		if occurrence.implicit && occurrence.field == "kind" && !concrete[occurrence.id] {
			implicitHere[occurrence.id] = true
		}
		if occurrence.concrete {
			explicitHere[occurrence.id] = true
		}
		if becomesConcrete {
			concrete[occurrence.id] = true
		}
	}
	return diagnostics
}

func onlyDuplicateParseFailure(diagnostics []Diagnostic) bool {
	return len(diagnostics) == 1 && strings.Contains(diagnostics[0].Message, "has already been defined")
}

// NormalizeVersionFile resolves version-file bytes without performing I/O.
func NormalizeVersionFile(path string, input []byte) (string, *Diagnostic) {
	if !utf8.Valid(input) {
		return "", &Diagnostic{Path: path, Line: 1, Column: 1, Message: "version file must be valid UTF-8"}
	}
	value := input
	if bytes.HasPrefix(value, []byte{0xef, 0xbb, 0xbf}) {
		value = value[3:]
	}
	value = bytes.ReplaceAll(value, []byte("\r\n"), []byte("\n"))
	value = bytes.ReplaceAll(value, []byte("\r"), []byte("\n"))
	if bytes.HasSuffix(value, []byte("\n")) {
		value = value[:len(value)-1]
	}
	if len(value) == 0 {
		return "", &Diagnostic{Path: path, Line: 1, Column: 1, Message: "version file must be non-empty"}
	}
	if bytes.Contains(value, []byte("\n")) {
		return "", &Diagnostic{Path: path, Line: 1, Column: 1, Message: "version file must contain exactly one line"}
	}
	if bytes.HasPrefix(value, []byte{0xef, 0xbb, 0xbf}) {
		return "", &Diagnostic{Path: path, Line: 1, Column: 1, Message: "version file may contain at most one leading BOM"}
	}
	return string(value), nil
}

func (v *validator) validateV2PredicateIDs(registry ContractRegistry) {
	for _, predicate := range registry.Contracts {
		if predicate.Slice == "" || (predicate.Kind != PredicateRequires && predicate.Kind != PredicateForbids && predicate.Kind != PredicateOccurrences) {
			continue
		}
		dash := strings.LastIndexByte(predicate.ID, '-')
		prefix := predicate.ID
		if dash > 0 {
			prefix = predicate.ID[:dash]
		}
		sum := sha256.Sum256(slicedPredicatePreimage(predicate))
		want := fmt.Sprintf("%s-%x", prefix, sum[:6])
		if predicate.ID != want {
			v.diagnostics = append(v.diagnostics, Diagnostic{Path: predicate.Position.Path, Line: predicate.Position.Line, Column: predicate.Position.Column, Message: "predicate ID must be " + want})
		}
	}
}

func slicedPredicatePreimage(predicate Contract) []byte {
	return []byte(fmt.Sprintf("{\"kind\":%s,\"slice\":%s,\"pattern\":%s,\"applies_to\":%s}\n",
		canonicaljson.String(string(predicate.Kind)), canonicaljson.String(predicate.Slice), canonicaljson.String(predicate.Pattern), canonicaljson.StringArray(predicate.AppliesTo)))
}

func parseDocument(path string, input []byte) (map[string]any, []toml.Key, *Diagnostic) {
	var root map[string]any
	metadata, err := toml.Decode(string(input), &root)
	if err == nil {
		return root, metadata.Keys(), nil
	}
	diagnostic := Diagnostic{Path: path, Line: 1, Column: 1, Message: err.Error()}
	var parseError toml.ParseError
	if errors.As(err, &parseError) {
		diagnostic.Line = parseError.Position.Line
		diagnostic.Column = parseError.Position.Col
		diagnostic.Message = parseError.Message
	}
	return nil, nil, &diagnostic
}

type validator struct {
	path        string
	input       []byte
	specVersion int
	diagnostics []Diagnostic
}

func (v *validator) add(keyPath, message string) {
	line, column, ok := locateContractField(v.input, keyPath)
	if !ok {
		line, column = locate(v.input, keyPath)
	}
	v.diagnostics = append(v.diagnostics, Diagnostic{Path: v.path, Line: line, Column: column, Message: message})
}

func locate(input []byte, keyPath string) (int, int) {
	leaf := keyPath
	if dot := strings.LastIndexByte(leaf, '.'); dot >= 0 {
		leaf = leaf[dot+1:]
	}
	if bracket := strings.IndexByte(leaf, '['); bracket >= 0 {
		leaf = leaf[:bracket]
	}
	lines := strings.Split(string(input), "\n")
	if line, column, ok := locateRuleField(lines, keyPath, leaf); ok {
		return line, column
	}
	for i, line := range lines {
		if column := strings.Index(line, leaf); column >= 0 {
			return i + 1, column + 1
		}
	}
	return 1, 1
}

func locateRuleField(lines []string, keyPath, leaf string) (int, int, bool) {
	rules := strings.Index(keyPath, ".rules[")
	if rules < 0 {
		return 0, 0, false
	}
	indexStart := rules + len(".rules[")
	indexEnd := strings.IndexByte(keyPath[indexStart:], ']')
	if indexEnd < 0 {
		return 0, 0, false
	}
	ruleIndex, err := strconv.Atoi(keyPath[indexStart : indexStart+indexEnd])
	if err != nil {
		return 0, 0, false
	}
	marker := "[[" + keyPath[:rules] + ".rules]]"
	current := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == marker {
			current++
			continue
		}
		if current != ruleIndex {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "[[targets.") {
			return 0, 0, false
		}
		if column := strings.Index(line, leaf); column >= 0 {
			return i + 1, column + 1, true
		}
	}
	return 0, 0, false
}

func table(value any) (map[string]any, bool) {
	result, ok := value.(map[string]any)
	return result, ok
}

func array(value any) ([]any, bool) {
	result, ok := value.([]map[string]any)
	if ok {
		values := make([]any, len(result))
		for i := range result {
			values[i] = result[i]
		}
		return values, true
	}
	values, ok := value.([]any)
	return values, ok
}

func stringValue(value any) (string, bool) {
	result, ok := value.(string)
	return result, ok
}

func boolValue(value any) (bool, bool) {
	result, ok := value.(bool)
	return result, ok
}

func orderedChildNames(order []toml.Key, prefix string) []string {
	seen := map[string]bool{}
	var values []string
	for _, key := range order {
		if len(key) >= 2 && key[0] == prefix && !seen[key[1]] {
			seen[key[1]] = true
			values = append(values, key[1])
		}
	}
	return values
}

func (v *validator) unknownKeys(prefix string, values map[string]any, allowed ...string) {
	set := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		set[key] = true
	}
	for _, key := range sortedKeys(values) {
		if !set[key] {
			full := key
			if prefix != "" {
				full = prefix + "." + key
			}
			v.add(full, "unknown key "+full)
		}
	}
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func requiredString(v *validator, values map[string]any, keyPath, key string) string {
	value, exists := values[key]
	if !exists {
		v.add(keyPath, keyPath+" is required")
		return ""
	}
	result, ok := stringValue(value)
	if !ok {
		v.add(keyPath, keyPath+" must be a string")
		return ""
	}
	return result
}

func validateNonemptySingleLine(v *validator, keyPath, value string) {
	if value == "" {
		v.add(keyPath, keyPath+" must be non-empty")
	}
	if strings.ContainsAny(value, "\r\n") {
		v.add(keyPath, keyPath+" must be a single-line string")
	}
}

func formatIndex(prefix string, index int) string {
	return fmt.Sprintf("%s[%d]", prefix, index)
}
