package config

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
)

func (v *validator) contracts(root map[string]any, order []toml.Key, knownTargets []string) ContractRegistry {
	v.unknownKeys("", root, "contracts")
	rawContracts, ok := table(root["contracts"])
	if root["contracts"] == nil {
		v.add("contracts", "contracts table is required")
		return ContractRegistry{}
	}
	if !ok {
		v.add("contracts", "contracts must be a table")
		return ContractRegistry{}
	}
	targets := make(map[string]bool, len(knownTargets))
	for _, target := range knownTargets {
		targets[target] = true
	}
	ids := orderedChildNames(order, "contracts")
	registry := ContractRegistry{Contracts: make([]Contract, 0, len(ids))}
	for _, id := range ids {
		prefix := "contracts." + id
		if !generalIDPattern.MatchString(id) {
			v.add(prefix, "invalid predicate ID "+id)
		}
		values, ok := table(rawContracts[id])
		if !ok {
			v.add(prefix, "predicate "+id+" must be a table")
			continue
		}
		line, column := locateContractTable(v.input, id)
		contract := Contract{
			ID:       id,
			Kind:     PredicateKind(requiredString(v, values, prefix+".kind", "kind")),
			Position: ContractPosition{Path: v.path, Line: line, Column: column},
		}
		if contract.Kind == PredicateStructure && v.specVersion == 2 {
			v.parseStructure(&contract, values, prefix, targets)
			registry.Contracts = append(registry.Contracts, contract)
			continue
		}
		v.unknownKeys(prefix, values, "kind", "slice", "pattern", "before", "after", "applies_to")
		contract.Slice = optionalString(v, values, prefix, "slice")
		contract.Pattern = optionalString(v, values, prefix, "pattern")
		contract.Before = optionalString(v, values, prefix, "before")
		contract.After = optionalString(v, values, prefix, "after")
		contract.AppliesTo = v.appliesTo(values, prefix, targets)
		v.validateContract(contract, values, prefix)
		registry.Contracts = append(registry.Contracts, contract)
	}
	return registry
}

func (v *validator) parseStructure(contract *Contract, values map[string]any, prefix string, targets map[string]bool) {
	v.unknownKeys(prefix, values, "kind", "subject", "paths", "format", "applies_to", "assertions")
	contract.Subject = StructureSubject(requiredString(v, values, prefix+".subject", "subject"))
	if contract.Subject != StructureSourceFrontmatter && contract.Subject != StructureArtifact {
		v.add(prefix+".subject", "subject must be source_frontmatter or artifact")
	}
	contract.Paths = v.structurePaths(values, prefix)
	if contract.Subject == StructureSourceFrontmatter {
		if _, ok := values["format"]; ok {
			v.add(prefix+".format", "format is not allowed for source_frontmatter")
		}
		if _, ok := values["applies_to"]; ok {
			v.add(prefix+".applies_to", "applies_to is not allowed for source_frontmatter")
		}
	} else {
		contract.Format = StructureFormat(requiredString(v, values, prefix+".format", "format"))
		if contract.Format != StructureYAML && contract.Format != StructureTOML && contract.Format != StructureJSON {
			v.add(prefix+".format", "format must be yaml, toml, or json")
		}
		contract.AppliesTo = v.appliesTo(values, prefix, targets)
	}
	rawValue, exists := values["assertions"]
	if !exists {
		v.add(prefix+".assertions", "assertions is required")
		return
	}
	raw, ok := array(rawValue)
	if !ok {
		v.add(prefix+".assertions", "assertions must be an array")
		return
	}
	for index, item := range raw {
		contract.Assertions = append(contract.Assertions, v.structureAssertion(item, formatIndex(prefix+".assertions", index)))
	}
}

func (v *validator) structurePaths(values map[string]any, prefix string) []string {
	raw, ok := array(values["paths"])
	if !ok || len(raw) == 0 {
		v.add(prefix+".paths", "paths must contain at least one pattern")
		return nil
	}
	result := make([]string, 0, len(raw))
	for i, item := range raw {
		key := formatIndex(prefix+".paths", i)
		pattern, ok := stringValue(item)
		if !ok {
			v.add(key, "paths entries must be strings")
			continue
		}
		result = append(result, pattern)
		if pattern == "" || strings.HasPrefix(pattern, "/") || strings.Contains(pattern, `\`) || strings.Contains(pattern, "**") || strings.ContainsRune(pattern, 0) || !utf8.ValidString(pattern) {
			v.add(key, "path pattern must be non-empty project-relative and must not contain **")
		}
		for _, segment := range strings.Split(pattern, "/") {
			if segment == "" || segment == "." || segment == ".." {
				v.add(key, "path pattern must be anchored and slash-separated")
			}
		}
	}
	return result
}

func (v *validator) structureAssertion(raw any, prefix string) StructureAssertion {
	values, ok := table(raw)
	if !ok {
		v.add(prefix, "assertion must be a table")
		return StructureAssertion{}
	}
	v.unknownKeys(prefix, values, "path", "op", "value", "count")
	a := StructureAssertion{Path: requiredString(v, values, prefix+".path", "path"), Op: AssertionOp(requiredString(v, values, prefix+".op", "op"))}
	if !validAssertionPath(a.Path) {
		v.add(prefix+".path", "assertion path must contain dot-separated segments or *")
	}
	_, hasValue := values["value"]
	_, hasCount := values["count"]
	needsValue := a.Op == AssertEquals || a.Op == AssertExactKeys || a.Op == AssertListOrder || a.Op == AssertListSet
	needsCount := a.Op == AssertCardinality
	if needsValue != hasValue {
		if needsValue {
			v.add(prefix+".value", "value is required for "+string(a.Op))
		} else if hasValue {
			v.add(prefix+".value", "value is not allowed for "+string(a.Op))
		}
	}
	if needsCount != hasCount {
		if needsCount {
			v.add(prefix+".count", "count is required for cardinality")
		} else if hasCount {
			v.add(prefix+".count", "count is not allowed for "+string(a.Op))
		}
	}
	if hasValue {
		if value, ok := typedValue(values["value"]); ok {
			a.Value = &value
		} else {
			v.add(prefix+".value", "value must contain only string, int64, bool, list, or map")
		}
	}
	if hasCount {
		if count, ok := values["count"].(int64); ok && count >= 0 {
			a.Count = &count
		} else {
			v.add(prefix+".count", "count must be a non-negative integer")
		}
	}
	switch a.Op {
	case AssertExists, AssertAbsent, AssertCardinality, AssertEquals, AssertExactKeys, AssertListOrder, AssertListSet:
	default:
		v.add(prefix+".op", "unknown assertion op")
	}
	if a.Value != nil {
		validateAssertionOperand(v, prefix+".value", a.Op, *a.Value)
	}
	return a
}

func validAssertionPath(value string) bool {
	if value == "" {
		return true
	}
	for _, segment := range strings.Split(value, ".") {
		if segment == "" {
			return false
		}
	}
	return true
}

func typedValue(raw any) (TypedValue, bool) {
	switch value := raw.(type) {
	case string:
		return TypedValue{Kind: TypedString, String: value}, true
	case int64:
		return TypedValue{Kind: TypedInt, Int: value}, true
	case bool:
		return TypedValue{Kind: TypedBool, Bool: value}, true
	case []any:
		out := TypedValue{Kind: TypedList}
		for _, item := range value {
			child, ok := typedValue(item)
			if !ok {
				return TypedValue{}, false
			}
			out.List = append(out.List, child)
		}
		return out, true
	case []map[string]any:
		out := TypedValue{Kind: TypedList}
		for _, item := range value {
			child, ok := typedValue(item)
			if !ok {
				return TypedValue{}, false
			}
			out.List = append(out.List, child)
		}
		return out, true
	case map[string]any:
		out := TypedValue{Kind: TypedMap}
		for _, key := range sortedKeys(value) {
			child, ok := typedValue(value[key])
			if !ok {
				return TypedValue{}, false
			}
			out.Map = append(out.Map, TypedEntry{Key: key, Value: child})
		}
		return out, true
	default:
		return TypedValue{}, false
	}
}

func validateAssertionOperand(v *validator, key string, op AssertionOp, value TypedValue) {
	if op == AssertExactKeys {
		if value.Kind != TypedList {
			v.add(key, "exact_keys value must be a list of unique strings")
			return
		}
		seen := map[string]bool{}
		for _, item := range value.List {
			if item.Kind != TypedString || seen[item.String] {
				v.add(key, "exact_keys value must be a list of unique strings")
				return
			}
			seen[item.String] = true
		}
	}
	if op == AssertListOrder || op == AssertListSet {
		if value.Kind != TypedList {
			v.add(key, string(op)+" value must be a list")
			return
		}
	}
	if op == AssertListSet {
		seen := map[string]bool{}
		for _, item := range value.List {
			if item.Kind == TypedList || item.Kind == TypedMap {
				v.add(key, "list_set value must contain only unique scalars")
				return
			}
			token := fmt.Sprint(item.Kind, item.String, item.Int, item.Bool)
			if seen[token] {
				v.add(key, "list_set value must contain only unique scalars")
				return
			}
			seen[token] = true
		}
	}
}

func locateContractTable(input []byte, id string) (int, int) {
	for _, occurrence := range predicateOccurrences("", input) {
		if occurrence.id == id {
			return occurrence.position.Line, occurrence.position.Column
		}
	}
	for lineNumber, line := range strings.Split(string(input), "\n") {
		trimmed := strings.TrimSpace(line)
		const prefix = "[contracts."
		if !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		closeBracket := strings.IndexByte(trimmed[len(prefix):], ']')
		if closeBracket < 0 {
			continue
		}
		closeBracket += len(prefix)
		remainder := strings.TrimSpace(trimmed[closeBracket+1:])
		if remainder != "" && !strings.HasPrefix(remainder, "#") {
			continue
		}
		candidate := trimmed[len(prefix):closeBracket]
		if unquoted, err := strconv.Unquote(candidate); err == nil {
			candidate = unquoted
		} else if len(candidate) >= 2 && candidate[0] == '\'' && candidate[len(candidate)-1] == '\'' {
			candidate = candidate[1 : len(candidate)-1]
		}
		if candidate != id {
			continue
		}
		column := strings.Index(line, "[")
		if column < 0 {
			column = 0
		}
		return lineNumber + 1, column + 1
	}
	return 1, 1
}

func optionalString(v *validator, values map[string]any, prefix, key string) string {
	raw, exists := values[key]
	if !exists {
		return ""
	}
	value, ok := stringValue(raw)
	if !ok {
		v.add(prefix+"."+key, key+" must be a string")
	}
	return value
}

func (v *validator) appliesTo(values map[string]any, prefix string, targets map[string]bool) []string {
	raw, exists := values["applies_to"]
	if !exists {
		v.add(prefix+".applies_to", "applies_to is required")
		return nil
	}
	items, ok := array(raw)
	if !ok {
		v.add(prefix+".applies_to", "applies_to must be an array of strings")
		return nil
	}
	if len(items) == 0 {
		v.add(prefix+".applies_to", "applies_to must contain at least one target")
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(items))
	for i, item := range items {
		target, ok := stringValue(item)
		if !ok {
			v.add(formatIndex(prefix+".applies_to", i), "applies_to entries must be strings")
			continue
		}
		result = append(result, target)
		if !targets[target] {
			v.add(formatIndex(prefix+".applies_to", i), "unknown target "+target)
		}
		if seen[target] {
			v.add(formatIndex(prefix+".applies_to", i), "duplicate target "+target)
		}
		seen[target] = true
	}
	return result
}

func (v *validator) validateContract(contract Contract, values map[string]any, prefix string) {
	allowed := map[PredicateKind]map[string]bool{
		PredicateRequires: {"kind": true, "slice": true, "pattern": true, "applies_to": true},
		PredicateForbids:  {"kind": true, "slice": true, "pattern": true, "applies_to": true},
		PredicateOrder:    {"kind": true, "before": true, "after": true, "applies_to": true},
	}
	fields, known := allowed[contract.Kind]
	if !known {
		v.add(prefix+".kind", "kind must be one of requires, forbids, order")
		return
	}
	for _, field := range sortedKeys(values) {
		if !fields[field] {
			v.add(prefix+"."+field, field+" is not allowed for "+string(contract.Kind))
		}
	}
	required := []string{"kind", "applies_to"}
	if contract.Kind == PredicateRequires {
		required = append(required, "slice", "pattern")
	}
	if contract.Kind == PredicateForbids {
		required = append(required, "pattern")
	}
	if contract.Kind == PredicateOrder {
		required = append(required, "before", "after")
	}
	for _, field := range required {
		if _, exists := values[field]; !exists {
			v.add(prefix+"."+field, field+" is required for "+string(contract.Kind))
		}
	}
	if contract.Pattern != "" {
		if strings.TrimFunc(contract.Pattern, unicode.IsSpace) != contract.Pattern {
			v.add(prefix+".pattern", "pattern must not have leading or trailing whitespace")
		}
	} else if contract.Kind != PredicateOrder && values["pattern"] != nil {
		v.add(prefix+".pattern", "pattern must be non-empty")
	}
	validateReferenceID(v, prefix+".slice", "slice", contract.Slice, values["slice"] != nil)
	validateReferenceID(v, prefix+".before", "before", contract.Before, values["before"] != nil)
	validateReferenceID(v, prefix+".after", "after", contract.After, values["after"] != nil)
}

func validateReferenceID(v *validator, key, label, value string, present bool) {
	if present && !generalIDPattern.MatchString(value) {
		v.add(key, "invalid "+label+" ID "+value)
	}
}
