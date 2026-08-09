package adapter

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/akitanabe/gunte/internal/compile"
	"github.com/akitanabe/gunte/internal/config"
	"github.com/akitanabe/gunte/internal/lockfile"
	"github.com/akitanabe/gunte/internal/outputpath"
)

type matchDecision struct {
	ruleIndex int
	captures  []string
}

// UnmatchedSourcePaths returns source paths that do not match any rule in any
// target. It is a pure coverage calculation used by the v2 check boundary;
// target selection must not hide an unmatched source in another target.
func UnmatchedSourcePaths(project config.ProjectConfig, sources []Source) []string {
	result := make([]string, 0)
	for _, source := range sources {
		matched := false
		for _, target := range project.Targets {
			for _, rule := range target.Rules {
				if _, ok := matchPath(rule.Match, sourcePath(source)); ok {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			result = append(result, sourcePath(source))
		}
	}
	sort.Strings(result)
	return result
}

// Adapt resolves target rules, path templates, and metadata mappings. It is a
// pure calculation: all returned byte slices and lists are independent copies
// of the supplied input data.
func Adapt(project config.ProjectConfig, sources []Source) (Result, []Diagnostic) {
	plan, diagnostics := Preflight(project, sources)
	result := Result{Artifacts: make([]Artifact, 0)}
	for targetIndex := range project.Targets {
		adapted, targetDiagnostics := AdaptTarget(project, targetIndex, sources, plan)
		result.Artifacts = append(result.Artifacts, adapted.Artifacts...)
		diagnostics = append(diagnostics, targetDiagnostics...)
	}
	return result, diagnostics
}

// Preflight resolves rule matches and validates every expanded output path for
// every target without resolving metadata or serializing profiles.
func Preflight(project config.ProjectConfig, sources []Source) (PathPlan, []Diagnostic) {
	decisions := make([][]*matchDecision, len(sources))
	diagnostics := make([]Diagnostic, 0)
	plan := PathPlan{Artifacts: make([]PlannedArtifact, 0)}
	for sourceIndex, source := range sources {
		decisions[sourceIndex] = make([]*matchDecision, len(project.Targets))
		matchedTarget := false
		sourcePath := sourcePath(source)
		for targetIndex, target := range project.Targets {
			matches := make([]matchDecision, 0, 1)
			for ruleIndex, rule := range target.Rules {
				captures, matched := matchPath(rule.Match, sourcePath)
				if matched {
					matches = append(matches, matchDecision{ruleIndex: ruleIndex, captures: captures})
				}
			}
			if len(matches) > 1 {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "multiple_rule_match",
					Severity: SeverityError,
					Source:   sourcePath,
					Target:   target.ID,
					Message:  fmt.Sprintf("source matches %d rules in target", len(matches)),
				})
				matchedTarget = true
				continue
			}
			if len(matches) == 1 {
				decisions[sourceIndex][targetIndex] = &matches[0]
				matchedTarget = true
			}
		}
		if !matchedTarget {
			plan.UnmatchedSources = append(plan.UnmatchedSources, sourcePath)
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "unmatched_source",
				Severity: SeverityWarning,
				Source:   sourcePath,
				Message:  "source does not match a rule in any target",
			})
		}
	}

	sort.Strings(plan.UnmatchedSources)
	seenOutputPaths := make(map[string]int)
	semanticInputs := semanticInputPaths(project)
	for targetIndex, target := range project.Targets {
		for sourceIndex, decision := range decisions {
			if decision[targetIndex] == nil {
				continue
			}
			ruleIndex := decision[targetIndex].ruleIndex
			rule := target.Rules[ruleIndex]
			source := sources[sourceIndex]
			sourcePath := sourcePath(source)
			artifactPath, ok := expandPathTemplate(rule.Path, decision[targetIndex].captures)
			if !ok {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "invalid_path_template",
					Severity: SeverityError,
					Source:   sourcePath,
					Target:   target.ID,
					Rule:     ruleIndex,
					Message:  "path template contains an invalid or unavailable capture",
				})
				continue
			}
			if !validRelativePath(artifactPath) {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "invalid_artifact_path",
					Severity: SeverityError,
					Source:   sourcePath,
					Target:   target.ID,
					Rule:     ruleIndex,
					Message:  "expanded artifact path is not a relative slash-separated path",
				})
				continue
			}
			fullPath := outputpath.Join(target.OutputRoot, artifactPath)
			_, duplicate := seenOutputPaths[fullPath]
			if duplicate || semanticInputs[fullPath] || lockfile.Reserves(fullPath) {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "path_collision",
					Severity: SeverityError,
					Source:   sourcePath,
					Target:   target.ID,
					Rule:     ruleIndex,
					Message:  "artifact path collides with another artifact or semantic input",
				})
				continue
			}
			if !duplicate {
				seenOutputPaths[fullPath] = targetIndex
			}
			plan.Artifacts = append(plan.Artifacts, PlannedArtifact{TargetIndex: targetIndex, SourceIndex: sourceIndex, RuleIndex: ruleIndex, Path: fullPath})
		}
	}
	return plan, diagnostics
}

// AdaptTarget resolves metadata for one target using only preflight decisions.
func AdaptTarget(project config.ProjectConfig, targetIndex int, sources []Source, plan PathPlan) (Result, []Diagnostic) {
	result := Result{Artifacts: make([]Artifact, 0)}
	diagnostics := make([]Diagnostic, 0)
	seenPaths := make(map[string]struct{})
	if targetIndex < 0 || targetIndex >= len(project.Targets) {
		return result, []Diagnostic{{Code: "missing_target", Severity: SeverityError, Message: "target index is outside project targets"}}
	}
	target := project.Targets[targetIndex]
	for _, decision := range plan.Artifacts {
		if decision.TargetIndex != targetIndex {
			continue
		}
		if decision.SourceIndex < 0 || decision.SourceIndex >= len(sources) || decision.RuleIndex < 0 || decision.RuleIndex >= len(target.Rules) {
			diagnostics = append(diagnostics, Diagnostic{Code: "invalid_path_plan", Severity: SeverityError, Target: target.ID, Message: "path plan index is outside adapter input"})
			continue
		}
		source := sources[decision.SourceIndex]
		sourcePath := sourcePath(source)
		artifact, metadataDiagnostics, ok := buildArtifact(project, target, target.Rules[decision.RuleIndex], decision.RuleIndex, source, sourcePath, decision.Path)
		diagnostics = append(diagnostics, metadataDiagnostics...)
		if !ok {
			continue
		}
		if _, duplicate := seenPaths[decision.Path]; duplicate {
			diagnostics = append(diagnostics, Diagnostic{Code: "path_collision", Severity: SeverityError, Source: sourcePath, Target: target.ID, Rule: decision.RuleIndex, Message: "artifact path collides with another artifact or semantic input"})
			continue
		}
		seenPaths[decision.Path] = struct{}{}
		result.Artifacts = append(result.Artifacts, artifact)
	}
	return result, diagnostics
}

func sourcePath(source Source) string {
	return source.Projection.Path
}

func matchPath(pattern, value string) ([]string, bool) {
	var expression strings.Builder
	expression.WriteByte('^')
	for _, character := range pattern {
		if character == '*' {
			expression.WriteString("([^/]*)")
			continue
		}
		expression.WriteString(regexp.QuoteMeta(string(character)))
	}
	expression.WriteByte('$')
	matched, err := regexp.Compile(expression.String())
	if err != nil {
		return nil, false
	}
	groups := matched.FindStringSubmatch(value)
	if groups == nil {
		return nil, false
	}
	return groups[1:], true
}

func expandPathTemplate(template string, captures []string) (string, bool) {
	var output strings.Builder
	for index := 0; index < len(template); {
		if template[index] != '{' && template[index] != '}' {
			output.WriteByte(template[index])
			index++
			continue
		}
		if template[index] != '{' {
			return "", false
		}
		close := strings.IndexByte(template[index+1:], '}')
		if close < 0 {
			return "", false
		}
		close += index + 1
		variable := template[index+1 : close]
		if len(variable) != 1 || variable[0] < '1' || variable[0] > '9' {
			return "", false
		}
		captureIndex := int(variable[0] - '1')
		if captureIndex >= len(captures) {
			return "", false
		}
		output.WriteString(captures[captureIndex])
		index = close + 1
	}
	return output.String(), true
}

func validRelativePath(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, `\`) || strings.ContainsRune(value, 0) || !utf8.ValidString(value) {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func semanticInputPaths(project config.ProjectConfig) map[string]bool {
	paths := map[string]bool{}
	for _, file := range config.SemanticInputPaths(project) {
		paths[file] = true
	}
	return paths
}

func buildArtifact(project config.ProjectConfig, target config.Target, rule config.Rule, ruleIndex int, source Source, sourcePath, fullPath string) (Artifact, []Diagnostic, bool) {
	artifact := Artifact{
		TargetID:   target.ID,
		SourcePath: sourcePath,
		Path:       fullPath,
		Profile:    rule.Profile,
		Header:     rule.Header,
		BodyField:  rule.BodyField,
		Body:       append([]byte(nil), source.Projection.Bytes...),
		Metadata:   make([]MetadataField, 0, len(rule.Metadata)),
		Contracts:  append([]compile.ProjectedDeclaration(nil), source.Projection.Contracts...),
		Anchors:    append([]compile.ProjectedDeclaration(nil), source.Projection.Anchors...),
	}
	diagnostics := make([]Diagnostic, 0)
	for _, entry := range rule.Metadata {
		value, found, err := resolveFrom(entry.From, project, sourcePath, source.Frontmatter)
		if err != nil {
			diagnostics = append(diagnostics, metadataDiagnostic("metadata_source", sourcePath, target.ID, ruleIndex, entry.Field, err.Error()))
			continue
		}
		mapped, ok, message := mapMetadataValue(value, found, entry)
		if !ok {
			diagnostics = append(diagnostics, metadataDiagnostic(metadataCode(message), sourcePath, target.ID, ruleIndex, entry.Field, message))
			continue
		}
		if !found && !entry.Required {
			continue
		}
		artifact.Metadata = append(artifact.Metadata, MetadataField{Field: entry.Field, Value: mapped})
	}
	if rule.Profile == config.ProfilePlainText {
		value, found, err := resolveFrom(rule.ValueFrom, project, sourcePath, source.Frontmatter)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{Code: "value_from_invalid", Severity: SeverityError, Source: sourcePath, Target: target.ID, Rule: ruleIndex, Message: err.Error()})
		} else if !found {
			diagnostics = append(diagnostics, Diagnostic{Code: "value_from_invalid", Severity: SeverityError, Source: sourcePath, Target: target.ID, Rule: ruleIndex, Message: "value_from did not resolve a value"})
		} else if stringValue, ok := value.(string); !ok || stringValue == "" || strings.ContainsAny(stringValue, "\r\n") {
			diagnostics = append(diagnostics, Diagnostic{Code: "value_from_invalid", Severity: SeverityError, Source: sourcePath, Target: target.ID, Rule: ruleIndex, Message: "value_from must resolve to a non-empty single-line string"})
		} else {
			artifact.Value = &MetadataValue{Type: config.MetadataString, String: stringValue}
		}
	}
	return artifact, diagnostics, len(diagnostics) == 0
}

func metadataDiagnostic(code, source, target string, rule int, field, message string) Diagnostic {
	return Diagnostic{Code: code, Severity: SeverityError, Source: source, Target: target, Rule: rule, Field: field, Message: message}
}

func metadataCode(message string) string {
	if strings.Contains(message, "missing") {
		return "metadata_missing"
	}
	return "metadata_invalid"
}

func resolveFrom(from string, project config.ProjectConfig, sourcePath string, frontmatter map[string]any) (any, bool, error) {
	switch {
	case from == "core:name":
		name := path.Base(sourcePath)
		if name == "." || name == "/" || name == "" {
			return nil, false, nil
		}
		extension := strings.LastIndexByte(name, '.')
		if name[0] == '.' {
			if nestedExtension := strings.LastIndexByte(name[1:], '.'); nestedExtension >= 0 {
				extension = nestedExtension + 1
			} else {
				extension = -1
			}
		}
		if extension >= 0 {
			name = name[:extension]
		}
		return name, true, nil
	case from == "project:version":
		return project.Project.Version, project.Project.Version != "", nil
	case strings.HasPrefix(from, "literal:"):
		return strings.TrimPrefix(from, "literal:"), true, nil
	case strings.HasPrefix(from, "frontmatter:"):
		keyPath := strings.TrimPrefix(from, "frontmatter:")
		segments := strings.Split(keyPath, ".")
		if keyPath == "" || len(segments) == 0 {
			return nil, false, fmt.Errorf("frontmatter path is empty")
		}
		for _, segment := range segments {
			if !validFrontmatterSegment(segment) {
				return nil, false, fmt.Errorf("invalid frontmatter path %q", keyPath)
			}
		}
		var current any = frontmatter
		for _, segment := range segments {
			table, ok := current.(map[string]any)
			if !ok {
				return nil, false, nil
			}
			value, ok := table[segment]
			if !ok {
				return nil, false, nil
			}
			current = value
		}
		return current, true, nil
	default:
		return nil, false, fmt.Errorf("unsupported value source %q", from)
	}
}

func validFrontmatterSegment(segment string) bool {
	if segment == "" || (segment[0] < 'A' || segment[0] > 'Z') && (segment[0] < 'a' || segment[0] > 'z') {
		return false
	}
	for _, character := range segment[1:] {
		if (character < 'A' || character > 'Z') && (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func mapMetadataValue(raw any, found bool, entry config.MetadataEntry) (MetadataValue, bool, string) {
	if !found {
		if !entry.Required {
			return MetadataValue{}, true, ""
		}
		return MetadataValue{}, false, "metadata value is missing"
	}
	switch entry.Type {
	case config.MetadataString:
		value, ok := raw.(string)
		if !ok {
			return MetadataValue{}, false, "metadata value type mismatch: expected string"
		}
		if value == "" {
			return MetadataValue{}, false, "metadata string value is empty"
		}
		return MetadataValue{Type: entry.Type, String: value}, true, ""
	case config.MetadataStringList:
		values, ok := stringList(raw)
		if !ok {
			return MetadataValue{}, false, "metadata value type mismatch: expected string list"
		}
		return MetadataValue{Type: entry.Type, Strings: values}, true, ""
	case config.MetadataCommaList:
		var parts []string
		switch value := raw.(type) {
		case string:
			if strings.TrimSpace(value) == "" {
				return MetadataValue{}, false, "metadata comma list value is empty"
			}
			parts = strings.Split(value, ",")
			for index := range parts {
				parts[index] = strings.TrimSpace(parts[index])
			}
		case []string:
			parts = append([]string(nil), value...)
		case []any:
			values, ok := stringList(value)
			if !ok {
				return MetadataValue{}, false, "metadata value type mismatch: expected comma list string or list"
			}
			parts = values
		default:
			return MetadataValue{}, false, "metadata value type mismatch: expected comma list string or list"
		}
		if len(parts) == 0 {
			return MetadataValue{}, false, "metadata comma list value is empty"
		}
		for index := range parts {
			parts[index] = strings.TrimSpace(parts[index])
		}
		return MetadataValue{Type: entry.Type, Strings: parts}, true, ""
	case config.MetadataPlainToken:
		value, ok := raw.(string)
		if !ok {
			return MetadataValue{}, false, "metadata value type mismatch: expected plain token string"
		}
		if value == "" {
			return MetadataValue{}, false, "metadata plain token value is empty"
		}
		return MetadataValue{Type: entry.Type, String: value}, true, ""
	default:
		return MetadataValue{}, false, "metadata type is unsupported"
	}
}

func stringList(raw any) ([]string, bool) {
	switch values := raw.(type) {
	case []string:
		return append([]string(nil), values...), true
	case []any:
		result := make([]string, len(values))
		for index, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil, false
			}
			result[index] = text
		}
		return result, true
	default:
		return nil, false
	}
}
