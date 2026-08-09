// Package structure evaluates small typed structural contracts without I/O.
package structure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/akitanabe/gunte/internal/config"
	"github.com/akitanabe/gunte/internal/serialize"
	"github.com/akitanabe/gunte/internal/typeddata"
	"gopkg.in/yaml.v3"
)

type Failure struct {
	Contract    config.ContractPosition
	ContractID  string
	TargetID    string
	Path        string
	RelatedPath string
	Message     string
}

func EvaluateSources(registry config.ContractRegistry, documents []SourceDocument) []Failure {
	var failures []Failure
	for _, contract := range registry.Contracts {
		if contract.Kind != config.PredicateStructure || contract.Subject != config.StructureSourceFrontmatter {
			continue
		}
		matched := 0
		for _, document := range documents {
			if !matchesAny(contract.Paths, document.Path) {
				continue
			}
			matched++
			if document.Node == nil {
				failures = append(failures, failure(contract, "", "", document.Path, "source has no supported TOML frontmatter"))
				continue
			}
			failures = append(failures, evaluateAssertions(contract, "", "", document.Path, *document.Node)...)
		}
		if matched == 0 {
			failures = append(failures, failure(contract, "", "", "", "selector matched no source document"))
		}
	}
	return failures
}

type SourceDocument struct {
	Path string
	Node *typeddata.Value
}

func EvaluateArtifacts(registry config.ContractRegistry, artifacts []serialize.Artifact, selectedTargets []string) []Failure {
	selected := map[string]bool{}
	for _, id := range selectedTargets {
		selected[id] = true
	}
	var failures []Failure
	for _, contract := range registry.Contracts {
		if contract.Kind != config.PredicateStructure || contract.Subject != config.StructureArtifact {
			continue
		}
		for _, target := range contract.AppliesTo {
			if len(selected) > 0 && !selected[target] {
				continue
			}
			matched := 0
			for _, artifact := range artifacts {
				if artifact.TargetID != target || !matchesAny(contract.Paths, artifact.Path) {
					continue
				}
				matched++
				if !profileMatches(contract.Format, artifact.Profile) {
					failures = append(failures, failure(contract, target, artifact.Path, artifact.SourcePath, "artifact profile does not match structure format"))
					continue
				}
				node, err := decode(contract.Format, artifact.Profile, artifact.Bytes)
				if err != nil {
					failures = append(failures, failure(contract, target, artifact.Path, artifact.SourcePath, err.Error()))
					continue
				}
				failures = append(failures, evaluateAssertions(contract, target, artifact.Path, artifact.SourcePath, node)...)
			}
			if matched == 0 {
				failures = append(failures, failure(contract, target, "", "", "selector matched no artifact for target"))
			}
		}
	}
	return failures
}

func failure(c config.Contract, target, artifact, related, message string) Failure {
	return Failure{Contract: c.Position, ContractID: c.ID, TargetID: target, Path: artifact, RelatedPath: related, Message: message}
}
func matchesAny(patterns []string, value string) bool {
	for _, pattern := range patterns {
		if globMatch(pattern, value) {
			return true
		}
	}
	return false
}

func globMatch(pattern, value string) bool {
	var expression strings.Builder
	expression.WriteByte('^')
	for _, character := range pattern {
		if character == '*' {
			expression.WriteString("[^/]*")
		} else {
			expression.WriteString(regexp.QuoteMeta(string(character)))
		}
	}
	expression.WriteByte('$')
	return regexp.MustCompile(expression.String()).MatchString(value)
}

func evaluateAssertions(c config.Contract, target, artifact, related string, root typeddata.Value) []Failure {
	var failures []Failure
	for _, assertion := range c.Assertions {
		nodes := resolveAssertionPath(root, assertion.Path)
		ok := assertionTrue(assertion, nodes)
		if !ok {
			failures = append(failures, failure(c, target, artifact, related, fmt.Sprintf("assertion %s %s failed with %d matching nodes", assertion.Path, assertion.Op, len(nodes))))
		}
	}
	return failures
}

func resolveAssertionPath(root typeddata.Value, pathValue string) []typeddata.Value {
	if pathValue == "" {
		return []typeddata.Value{root}
	}
	return resolve(root, strings.Split(pathValue, "."))
}

func resolve(root typeddata.Value, segments []string) []typeddata.Value {
	current := []typeddata.Value{root}
	for _, segment := range segments {
		next := []typeddata.Value{}
		for _, node := range current {
			if segment == "*" {
				if node.Kind == typeddata.Map {
					for _, entry := range node.Map {
						next = append(next, entry.Value)
					}
				} else if node.Kind == typeddata.List {
					next = append(next, node.List...)
				}
			} else if node.Kind == typeddata.Map {
				for _, entry := range node.Map {
					if entry.Key == segment {
						next = append(next, entry.Value)
						break
					}
				}
			}
		}
		current = next
	}
	return current
}

func assertionTrue(a config.StructureAssertion, nodes []typeddata.Value) bool {
	switch a.Op {
	case config.AssertExists:
		return len(nodes) >= 1
	case config.AssertAbsent:
		return len(nodes) == 0
	case config.AssertCardinality:
		return a.Count != nil && int64(len(nodes)) == *a.Count
	case config.AssertEquals:
		return len(nodes) == 1 && a.Value != nil && typeddata.Equal(nodes[0], *a.Value)
	case config.AssertExactKeys:
		if len(nodes) != 1 || nodes[0].Kind != typeddata.Map || a.Value == nil {
			return false
		}
		actual := map[string]bool{}
		for _, e := range nodes[0].Map {
			if actual[e.Key] {
				return false
			}
			actual[e.Key] = true
		}
		if len(actual) != len(a.Value.List) {
			return false
		}
		for _, v := range a.Value.List {
			if v.Kind != typeddata.String || !actual[v.String] {
				return false
			}
		}
		return true
	case config.AssertListOrder:
		return len(nodes) == 1 && nodes[0].Kind == typeddata.List && a.Value != nil && typeddata.Equal(nodes[0], *a.Value)
	case config.AssertListSet:
		if len(nodes) != 1 || nodes[0].Kind != typeddata.List || a.Value == nil {
			return false
		}
		return scalarSetEqual(nodes[0].List, a.Value.List)
	default:
		return false
	}
}

func scalarSetEqual(a, b []typeddata.Value) bool {
	if len(a) != len(b) {
		return false
	}
	am := map[string]bool{}
	for _, v := range a {
		k, ok := scalarKey(v)
		if !ok || am[k] {
			return false
		}
		am[k] = true
	}
	for _, v := range b {
		k, ok := scalarKey(v)
		if !ok || !am[k] {
			return false
		}
	}
	return true
}
func scalarKey(v typeddata.Value) (string, bool) {
	switch v.Kind {
	case typeddata.String:
		return "s:" + v.String, true
	case typeddata.Int:
		return "i:" + strconv.FormatInt(v.Int, 10), true
	case typeddata.Bool:
		return "b:" + strconv.FormatBool(v.Bool), true
	}
	return "", false
}

func profileMatches(format config.StructureFormat, profile config.Profile) bool {
	switch format {
	case config.StructureYAML:
		return profile == config.ProfileYAML || profile == config.ProfileMarkdown
	case config.StructureTOML:
		return profile == config.ProfileTOML
	case config.StructureJSON:
		return profile == config.ProfileJSON
	default:
		return false
	}
}

func decode(format config.StructureFormat, profile config.Profile, data []byte) (typeddata.Value, error) {
	switch format {
	case config.StructureYAML:
		return decodeYAML(profile, data)
	case config.StructureTOML:
		return decodeTOML(data)
	case config.StructureJSON:
		return decodeJSON(data)
	}
	return typeddata.Value{}, fmt.Errorf("unsupported structure format")
}

func decodeYAML(profile config.Profile, data []byte) (typeddata.Value, error) {
	yamlBytes := data
	valueName := "artifact"
	if profile == config.ProfileYAML {
		valueName = "frontmatter"
		if !bytes.HasPrefix(data, []byte("---\n")) {
			return typeddata.Value{}, fmt.Errorf("YAML artifact must start with exact frontmatter delimiter")
		}
		rest := data[4:]
		end := -1
		offset := 0
		for _, line := range bytes.SplitAfter(rest, []byte("\n")) {
			trim := bytes.TrimSuffix(line, []byte("\n"))
			if bytes.Equal(trim, []byte("---")) {
				end = offset
				break
			}
			offset += len(line)
		}
		if end < 0 {
			return typeddata.Value{}, fmt.Errorf("YAML artifact has no exact closing frontmatter delimiter")
		}
		yamlBytes = rest[:end]
	}
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(yamlBytes))
	if err := decoder.Decode(&document); err != nil {
		return typeddata.Value{}, fmt.Errorf("invalid YAML %s: %w", valueName, err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if profile == config.ProfileYAML {
			return typeddata.Value{}, fmt.Errorf("YAML frontmatter must contain exactly one document")
		}
		return typeddata.Value{}, fmt.Errorf("YAML artifact must contain exactly one YAML document")
	}
	if len(document.Content) != 1 {
		if profile == config.ProfileYAML {
			return typeddata.Value{}, fmt.Errorf("YAML frontmatter must contain one document")
		}
		return typeddata.Value{}, fmt.Errorf("YAML artifact must contain one document")
	}
	return yamlValue(document.Content[0], "")
}
func yamlValue(node *yaml.Node, pathValue string) (typeddata.Value, error) {
	switch node.Kind {
	case yaml.MappingNode:
		seen := map[string]bool{}
		out := typeddata.Value{Kind: typeddata.Map}
		for i := 0; i < len(node.Content); i += 2 {
			k := node.Content[i]
			if k.Kind != yaml.ScalarNode || k.Tag != "!!str" {
				return typeddata.Value{}, fmt.Errorf("YAML mapping key must be string at %s", pathValue)
			}
			if seen[k.Value] {
				return typeddata.Value{}, fmt.Errorf("duplicate YAML mapping key at %s", joinPath(pathValue, k.Value))
			}
			seen[k.Value] = true
			v, err := yamlValue(node.Content[i+1], joinPath(pathValue, k.Value))
			if err != nil {
				return typeddata.Value{}, err
			}
			out.Map = append(out.Map, typeddata.Entry{Key: k.Value, Value: v})
		}
		return out, nil
	case yaml.SequenceNode:
		out := typeddata.Value{Kind: typeddata.List}
		for _, child := range node.Content {
			v, err := yamlValue(child, pathValue)
			if err != nil {
				return typeddata.Value{}, err
			}
			out.List = append(out.List, v)
		}
		return out, nil
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!str":
			return typeddata.Value{Kind: typeddata.String, String: node.Value}, nil
		case "!!bool":
			v, err := strconv.ParseBool(node.Value)
			return typeddata.Value{Kind: typeddata.Bool, Bool: v}, err
		case "!!int":
			v, err := strconv.ParseInt(node.Value, 0, 64)
			return typeddata.Value{Kind: typeddata.Int, Int: v}, err
		default:
			return typeddata.Value{}, fmt.Errorf("unsupported YAML value at %s", pathValue)
		}
	}
	return typeddata.Value{}, fmt.Errorf("unsupported YAML node at %s", pathValue)
}
func joinPath(a, b string) string {
	if a == "" {
		return b
	}
	return a + "." + b
}

func decodeTOML(data []byte) (typeddata.Value, error) {
	var root map[string]any
	metadata, err := toml.Decode(string(data), &root)
	if err != nil {
		return typeddata.Value{}, fmt.Errorf("invalid TOML artifact: %w", err)
	}
	value, ok := typeddata.OrderedTOMLValue(root, nil, metadata.Keys())
	if !ok {
		return typeddata.Value{}, fmt.Errorf("TOML artifact contains unsupported typed value")
	}
	return value, nil
}

func decodeJSON(data []byte) (typeddata.Value, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	value, err := jsonTokenValue(decoder)
	if err != nil {
		return typeddata.Value{}, fmt.Errorf("invalid JSON artifact: %w", err)
	}
	if token, err := decoder.Token(); err != io.EOF {
		return typeddata.Value{}, fmt.Errorf("invalid JSON artifact trailing token %v", token)
	}
	return value, nil
}
func jsonTokenValue(decoder *json.Decoder) (typeddata.Value, error) {
	token, err := decoder.Token()
	if err != nil {
		return typeddata.Value{}, err
	}
	switch value := token.(type) {
	case string:
		return typeddata.Value{Kind: typeddata.String, String: value}, nil
	case bool:
		return typeddata.Value{Kind: typeddata.Bool, Bool: value}, nil
	case json.Number:
		i, err := strconv.ParseInt(string(value), 10, 64)
		if err != nil {
			return typeddata.Value{}, fmt.Errorf("numbers must be int64")
		}
		return typeddata.Value{Kind: typeddata.Int, Int: i}, nil
	case nil:
		return typeddata.Value{}, fmt.Errorf("null is not supported")
	case json.Delim:
		if value == '[' {
			out := typeddata.Value{Kind: typeddata.List}
			for decoder.More() {
				child, err := jsonTokenValue(decoder)
				if err != nil {
					return typeddata.Value{}, err
				}
				out.List = append(out.List, child)
			}
			_, err = decoder.Token()
			return out, err
		}
		if value == '{' {
			out := typeddata.Value{Kind: typeddata.Map}
			seen := map[string]bool{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return typeddata.Value{}, err
				}
				key := keyToken.(string)
				if seen[key] {
					return typeddata.Value{}, fmt.Errorf("duplicate JSON mapping key at %s", key)
				}
				seen[key] = true
				child, err := jsonTokenValue(decoder)
				if err != nil {
					return typeddata.Value{}, err
				}
				out.Map = append(out.Map, typeddata.Entry{Key: key, Value: child})
			}
			_, err = decoder.Token()
			return out, err
		}
	}
	return typeddata.Value{}, fmt.Errorf("unsupported JSON value")
}
