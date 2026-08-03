package config

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
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
	root, order, diagnostic := parseDocument(path, input)
	if diagnostic != nil {
		return ContractRegistry{}, []Diagnostic{*diagnostic}
	}
	v := validator{path: path, input: input}
	registry := v.contracts(root, order, knownTargets)
	return registry, v.diagnostics
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
	diagnostics []Diagnostic
}

func (v *validator) add(keyPath, message string) {
	line, column := locate(v.input, keyPath)
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
