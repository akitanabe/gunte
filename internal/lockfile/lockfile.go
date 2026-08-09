// Package lockfile calculates and writes the Spec-Version 2 semantic lock.
package lockfile

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/akitanabe/gunte/internal/canonicaljson"
	"github.com/akitanabe/gunte/internal/compile"
	"github.com/akitanabe/gunte/internal/config"
	"github.com/akitanabe/gunte/internal/lexer"
)

// SemanticInputPaths returns the normative, duplicate-free input order.
func SemanticInputPaths(project config.ProjectConfig) []string {
	return config.SemanticInputPaths(project)
}

// CanonicalBytes calculates the normative lock bytes without I/O.
func CanonicalBytes(project config.ProjectConfig, registry config.ContractRegistry, units []compile.SourceUnit) []byte {
	var result strings.Builder
	result.WriteString("{\n  \"spec_version\": 2,\n  \"semantic_inputs\": [")
	inputs := SemanticInputPaths(project)
	for index, input := range inputs {
		separator(&result, index)
		result.WriteString("    " + canonicaljson.String(input))
	}
	closeArray(&result, len(inputs))
	result.WriteString(",\n  \"contracts\": [")
	for index, predicate := range registry.Contracts {
		separator(&result, index)
		sum := sha256.Sum256(ContractPreimage(predicate))
		fmt.Fprintf(&result, "    {\n      \"id\": %s,\n      \"sha256\": \"%x\"\n    }", canonicaljson.String(predicate.ID), sum)
	}
	closeArray(&result, len(registry.Contracts))
	result.WriteString(",\n  \"declarations\": [")
	declarationIndex := 0
	for _, unit := range units {
		for _, marker := range unit.IR.Markers {
			kind := ""
			switch marker.Kind {
			case lexer.ContractOpen:
				kind = "span"
			case lexer.AnchorMarker:
				kind = "anchor"
			default:
				continue
			}
			separator(&result, declarationIndex)
			fmt.Fprintf(&result, "    {\n      \"kind\": %s,\n      \"id\": %s,\n      \"path\": %s\n    }", canonicaljson.String(kind), canonicaljson.String(marker.Token), canonicaljson.String(unit.Path))
			declarationIndex++
		}
	}
	closeArray(&result, declarationIndex)
	result.WriteString("\n}\n")
	return []byte(result.String())
}

func separator(result *strings.Builder, index int) {
	if index == 0 {
		result.WriteByte('\n')
	} else {
		result.WriteString(",\n")
	}
}

func closeArray(result *strings.Builder, count int) {
	if count > 0 {
		result.WriteByte('\n')
	}
	result.WriteString("  ]")
}

// ContractPreimage serializes validated text predicate Data for hashing.
func ContractPreimage(predicate config.Contract) []byte {
	if predicate.Kind == config.PredicateStructure {
		return structurePreimage(predicate)
	}
	result := "{\"type\":\"text\",\"id\":" + canonicaljson.String(predicate.ID) +
		",\"kind\":" + canonicaljson.String(string(predicate.Kind)) +
		",\"slice\":" + optional(predicate.Slice) +
		",\"pattern\":" + optional(predicate.Pattern) +
		",\"before\":" + optional(predicate.Before) +
		",\"after\":" + optional(predicate.After) +
		",\"applies_to\":" + canonicaljson.StringArray(predicate.AppliesTo)
	if predicate.Kind == config.PredicateOccurrences || predicate.Paths != nil || predicate.ExcludePaths != nil {
		result += ",\"paths\":" + canonicaljson.StringArray(predicate.Paths) +
			",\"exclude_paths\":" + canonicaljson.StringArray(predicate.ExcludePaths)
	}
	if predicate.Kind == config.PredicateOccurrences {
		if predicate.Count == nil {
			result += ",\"count\":null"
		} else {
			result += ",\"count\":" + strconv.FormatInt(*predicate.Count, 10)
		}
	}
	return []byte(result + "}\n")
}

func structurePreimage(contract config.Contract) []byte {
	var result strings.Builder
	result.WriteString("{\"type\":\"structure\",\"id\":" + canonicaljson.String(contract.ID) + ",\"subject\":" + canonicaljson.String(string(contract.Subject)) + ",\"paths\":" + canonicaljson.StringArray(contract.Paths) + ",\"format\":")
	if contract.Format == "" {
		result.WriteString("null")
	} else {
		result.WriteString(canonicaljson.String(string(contract.Format)))
	}
	result.WriteString(",\"applies_to\":" + canonicaljson.StringArray(contract.AppliesTo) + ",\"assertions\":[")
	for i, assertion := range contract.Assertions {
		if i > 0 {
			result.WriteByte(',')
		}
		result.WriteString("{\"path\":" + canonicaljson.String(assertion.Path) + ",\"op\":" + canonicaljson.String(string(assertion.Op)) + ",\"value\":")
		if assertion.Value == nil {
			result.WriteString("null")
		} else {
			writeTyped(&result, *assertion.Value)
		}
		result.WriteString(",\"count\":")
		if assertion.Count == nil {
			result.WriteString("null")
		} else {
			result.WriteString(strconv.FormatInt(*assertion.Count, 10))
		}
		result.WriteByte('}')
	}
	result.WriteString("]}\n")
	return []byte(result.String())
}

func writeTyped(result *strings.Builder, value config.TypedValue) {
	switch value.Kind {
	case config.TypedString:
		result.WriteString(canonicaljson.String(value.String))
	case config.TypedInt:
		result.WriteString(strconv.FormatInt(value.Int, 10))
	case config.TypedBool:
		result.WriteString(strconv.FormatBool(value.Bool))
	case config.TypedList:
		result.WriteByte('[')
		for i, item := range value.List {
			if i > 0 {
				result.WriteByte(',')
			}
			writeTyped(result, item)
		}
		result.WriteByte(']')
	case config.TypedMap:
		entries := append([]config.TypedEntry(nil), value.Map...)
		sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
		result.WriteByte('{')
		for i, entry := range entries {
			if i > 0 {
				result.WriteByte(',')
			}
			result.WriteString(canonicaljson.String(entry.Key))
			result.WriteByte(':')
			writeTyped(result, entry.Value)
		}
		result.WriteByte('}')
	}
}

func optional(value string) string {
	if value == "" {
		return "null"
	}
	return canonicaljson.String(value)
}
