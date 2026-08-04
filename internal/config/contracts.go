package config

import (
	"strconv"
	"strings"
	"unicode"

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
		v.unknownKeys(prefix, values, "kind", "slice", "pattern", "before", "after", "applies_to")
		line, column := locateContractTable(v.input, id)
		contract := Contract{
			ID:       id,
			Kind:     PredicateKind(requiredString(v, values, prefix+".kind", "kind")),
			Position: ContractPosition{Path: v.path, Line: line, Column: column},
		}
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

func locateContractTable(input []byte, id string) (int, int) {
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
