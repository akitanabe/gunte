package config

import (
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
		contract := Contract{ID: id, Kind: PredicateKind(requiredString(v, values, prefix+".kind", "kind"))}
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
