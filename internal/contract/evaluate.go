package contract

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/akitanabe/gunte/internal/compile"
	"github.com/akitanabe/gunte/internal/config"
	"github.com/akitanabe/gunte/internal/serialize"
	"github.com/akitanabe/gunte/internal/source"
)

type declarationKind uint8

const (
	contractDeclaration declarationKind = iota + 1
	anchorDeclaration
)

type resolvedDeclaration struct {
	artifactIndex int
	artifact      *serialize.Artifact
	declaration   serialize.Declaration
}

func evaluate(registry config.ContractRegistry, artifacts []serialize.Artifact, selectedTargets []string) (Result, []Diagnostic) {
	selected := targetSelection(selectedTargets)
	validationArtifacts := artifacts
	if len(selected) != 0 {
		validationArtifacts = make([]serialize.Artifact, 0, len(artifacts))
		for _, artifact := range artifacts {
			if selected[artifact.TargetID] {
				validationArtifacts = append(validationArtifacts, artifact)
			}
		}
	}
	diagnostics := validateArtifacts(validationArtifacts)
	if len(diagnostics) > 0 {
		return Result{Violations: make([]Violation, 0)}, diagnostics
	}
	result := Result{Violations: make([]Violation, 0)}
	for _, predicate := range registry.Contracts {
		if predicate.Kind == config.PredicateStructure {
			continue
		}
		if predicate.Pattern == "" && predicate.Kind != config.PredicateOrder {
			diagnostics = append(diagnostics, predicateDiagnostic(predicate, "pattern must be non-empty"))
			continue
		}
		if predicate.Kind == config.PredicateOccurrences && (predicate.Count == nil || *predicate.Count < 0) {
			diagnostics = append(diagnostics, predicateDiagnostic(predicate, "count must be a non-negative integer"))
			continue
		}
		for _, targetID := range predicate.AppliesTo {
			if len(selected) != 0 && !selected[targetID] {
				continue
			}
			violations, targetDiagnostics := evaluatePredicate(predicate, targetID, artifacts)
			diagnostics = append(diagnostics, targetDiagnostics...)
			if len(targetDiagnostics) > 0 {
				if hasInvalidPredicate(targetDiagnostics) {
					break
				}
				continue
			}
			for _, violation := range violations {
				result.Violations = append(result.Violations, violation)
			}
		}
	}
	return result, diagnostics
}

func hasInvalidPredicate(diagnostics []Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Kind == InvalidPredicate {
			return true
		}
	}
	return false
}

func targetSelection(ids []string) map[string]bool {
	if len(ids) == 0 {
		return nil
	}
	result := make(map[string]bool, len(ids))
	for _, id := range ids {
		result[id] = true
	}
	return result
}

func validateArtifacts(artifacts []serialize.Artifact) []Diagnostic {
	var diagnostics []Diagnostic
	for artifactIndex := range artifacts {
		artifact := &artifacts[artifactIndex]
		seen := map[string]declarationKind{}
		for _, declaration := range artifact.Contracts {
			diagnostics = append(diagnostics, validateDeclaration(*artifact, declaration)...)
			if prior, ok := seen[declaration.ID]; ok {
				message := "declaration ID is duplicated"
				if prior != contractDeclaration {
					message = "declaration ID is used by both a contract and anchor"
				}
				diagnostics = append(diagnostics, artifactDiagnostic(*artifact, DuplicateDeclaration, message))
			}
			seen[declaration.ID] = contractDeclaration
		}
		for _, declaration := range artifact.Anchors {
			diagnostics = append(diagnostics, validateDeclaration(*artifact, declaration)...)
			if prior, ok := seen[declaration.ID]; ok {
				message := "declaration ID is duplicated"
				if prior != anchorDeclaration {
					message = "declaration ID is used by both a contract and anchor"
				}
				diagnostics = append(diagnostics, artifactDiagnostic(*artifact, DuplicateDeclaration, message))
			}
			seen[declaration.ID] = anchorDeclaration
		}
	}
	return diagnostics
}

func validateDeclaration(artifact serialize.Artifact, declaration serialize.Declaration) []Diagnostic {
	rangeValue := declaration.ArtifactRange
	valid := rangeValue.Start >= 0 && rangeValue.End >= rangeValue.Start && rangeValue.End <= len(artifact.Bytes)
	if !declaration.Emitted && rangeValue != (source.Range{}) {
		valid = false
	}
	if valid {
		return nil
	}
	return []Diagnostic{{
		Kind:          InvalidArtifactRange,
		TargetID:      artifact.TargetID,
		ArtifactPath:  artifact.Path,
		RelatedSource: []compile.SourcePosition{declaration.Source},
		Message:       fmt.Sprintf("declaration %q has range [%d,%d) outside artifact bytes", declaration.ID, rangeValue.Start, rangeValue.End),
	}}
}

func evaluatePredicate(predicate config.Contract, targetID string, artifacts []serialize.Artifact) ([]Violation, []Diagnostic) {
	switch predicate.Kind {
	case config.PredicateRequires:
		declaration, diagnostics := resolveReference(predicate, targetID, predicate.Slice, false, artifacts)
		if len(diagnostics) > 0 {
			return nil, diagnostics
		}
		body := declaration.artifact.Bytes[declaration.declaration.ArtifactRange.Start:declaration.declaration.ArtifactRange.End]
		if emptySlice(body) || !Match(body, predicate.Pattern) {
			return []Violation{violation(predicate, RequiresViolation, targetID, declaration, nil)}, nil
		}
		return nil, nil
	case config.PredicateForbids:
		if predicate.Slice == "" {
			selected, positiveMatch := selectArtifacts(predicate, targetID, artifacts)
			if predicate.Paths != nil && !positiveMatch {
				return nil, []Diagnostic{predicateDiagnostic(predicate, "selector matched no artifact for target")}
			}
			for _, artifact := range selected {
				if Match(artifact.Bytes, predicate.Pattern) {
					return []Violation{{Kind: ForbidsViolation, PredicateID: predicate.ID, TargetID: targetID, ArtifactPath: artifact.Path, Predicate: predicate.Position}}, nil
				}
			}
			return nil, nil
		}
		declaration, diagnostics := resolveReference(predicate, targetID, predicate.Slice, false, artifacts)
		if len(diagnostics) > 0 {
			return nil, diagnostics
		}
		body := declaration.artifact.Bytes[declaration.declaration.ArtifactRange.Start:declaration.declaration.ArtifactRange.End]
		if emptySlice(body) || Match(body, predicate.Pattern) {
			return []Violation{violation(predicate, ForbidsViolation, targetID, declaration, nil)}, nil
		}
		return nil, nil
	case config.PredicateOccurrences:
		if predicate.Slice != "" {
			declaration, diagnostics := resolveReference(predicate, targetID, predicate.Slice, false, artifacts)
			if len(diagnostics) > 0 {
				return nil, diagnostics
			}
			body := declaration.artifact.Bytes[declaration.declaration.ArtifactRange.Start:declaration.declaration.ArtifactRange.End]
			actual := int64(CountMatches(body, predicate.Pattern))
			if actual != *predicate.Count {
				return []Violation{countViolation(predicate, targetID, declaration.artifact.Path, []compile.SourcePosition{declaration.declaration.Source}, actual)}, nil
			}
			return nil, nil
		}
		if len(predicate.Paths) == 0 {
			return nil, []Diagnostic{predicateDiagnostic(predicate, "paths must contain at least one pattern for artifact occurrences")}
		}
		selected, _ := selectArtifacts(predicate, targetID, artifacts)
		if len(selected) == 0 {
			return nil, []Diagnostic{predicateDiagnostic(predicate, "selector matched no artifact for target")}
		}
		var violations []Violation
		for _, artifact := range selected {
			actual := int64(CountMatches(artifact.Bytes, predicate.Pattern))
			if actual != *predicate.Count {
				var related []compile.SourcePosition
				if artifact.SourcePath != "" {
					related = []compile.SourcePosition{{Path: artifact.SourcePath, Line: 1, Column: 1}}
				}
				violations = append(violations, countViolation(predicate, targetID, artifact.Path, related, actual))
			}
		}
		return violations, nil
	case config.PredicateOrder:
		before, beforeDiagnostics := resolveReference(predicate, targetID, predicate.Before, true, artifacts)
		after, afterDiagnostics := resolveReference(predicate, targetID, predicate.After, true, artifacts)
		diagnostics := append(beforeDiagnostics, afterDiagnostics...)
		if len(diagnostics) > 0 {
			return nil, diagnostics
		}
		if before.artifactIndex != after.artifactIndex || before.declaration.ArtifactRange.Start >= after.declaration.ArtifactRange.Start {
			return []Violation{{Kind: OrderViolation, PredicateID: predicate.ID, TargetID: targetID, ArtifactPath: before.artifact.Path, Predicate: predicate.Position, RelatedSource: []compile.SourcePosition{before.declaration.Source, after.declaration.Source}}}, nil
		}
		return nil, nil
	default:
		return nil, []Diagnostic{predicateDiagnostic(predicate, "unknown predicate kind")}
	}
}

func resolveReference(predicate config.Contract, targetID, id string, allowAnchor bool, artifacts []serialize.Artifact) (resolvedDeclaration, []Diagnostic) {
	var contracts []resolvedDeclaration
	var anchors []resolvedDeclaration
	for index := range artifacts {
		if artifacts[index].TargetID != targetID {
			continue
		}
		artifact := &artifacts[index]
		for _, declaration := range artifact.Contracts {
			if declaration.ID == id {
				contracts = append(contracts, resolvedDeclaration{artifactIndex: index, artifact: artifact, declaration: declaration})
			}
		}
		for _, declaration := range artifact.Anchors {
			if declaration.ID == id {
				anchors = append(anchors, resolvedDeclaration{artifactIndex: index, artifact: artifact, declaration: declaration})
			}
		}
	}
	if len(contracts) == 0 && len(anchors) == 0 {
		return resolvedDeclaration{}, []Diagnostic{referenceDiagnostic(predicate, targetID, UnresolvedReference, nil, "reference "+id+" is not emitted for target")}
	}
	if len(contracts) > 1 || len(anchors) > 1 || (len(contracts) > 0 && len(anchors) > 0) {
		return resolvedDeclaration{}, []Diagnostic{referenceDiagnostic(predicate, targetID, DuplicateDeclaration, nil, "reference "+id+" resolves to multiple declarations")}
	}
	if len(contracts) == 0 && !allowAnchor {
		return resolvedDeclaration{}, []Diagnostic{referenceDiagnostic(predicate, targetID, ReferenceKindMismatch, &anchors[0].declaration.Source, "slice reference must name a contract span")}
	}
	if len(contracts) == 1 {
		if !contracts[0].declaration.Emitted {
			return resolvedDeclaration{}, []Diagnostic{referenceDiagnostic(predicate, targetID, UnresolvedReference, &contracts[0].declaration.Source, "reference "+id+" is not emitted for target")}
		}
		return contracts[0], nil
	}
	if !anchors[0].declaration.Emitted {
		return resolvedDeclaration{}, []Diagnostic{referenceDiagnostic(predicate, targetID, UnresolvedReference, &anchors[0].declaration.Source, "reference "+id+" is not emitted for target")}
	}
	return anchors[0], nil
}

func selectArtifacts(predicate config.Contract, targetID string, artifacts []serialize.Artifact) ([]serialize.Artifact, bool) {
	positive := predicate.Paths != nil
	positiveMatch := false
	excluded := func(path string) bool {
		for _, pattern := range predicate.ExcludePaths {
			if selectorMatch(pattern, path) {
				return true
			}
		}
		return false
	}
	included := func(path string) bool {
		if !positive {
			return true
		}
		for _, pattern := range predicate.Paths {
			if selectorMatch(pattern, path) {
				return true
			}
		}
		return false
	}
	seen := map[string]bool{}
	result := make([]serialize.Artifact, 0)
	for _, artifact := range artifacts {
		if artifact.TargetID != targetID || !included(artifact.Path) {
			continue
		}
		if positive {
			positiveMatch = true
		}
		if excluded(artifact.Path) || seen[artifact.Path] {
			continue
		}
		seen[artifact.Path] = true
		result = append(result, artifact)
	}
	return result, positiveMatch
}

func selectorMatch(pattern, value string) bool {
	var expression strings.Builder
	expression.WriteByte('^')
	for _, character := range pattern {
		if character == '*' {
			expression.WriteString("[^/]*")
			continue
		}
		expression.WriteString(regexp.QuoteMeta(string(character)))
	}
	expression.WriteByte('$')
	return regexp.MustCompile(expression.String()).MatchString(value)
}

func emptySlice(value []byte) bool {
	if len(value) == 0 {
		return true
	}
	for _, byteValue := range value {
		if byteValue != ' ' && byteValue != '\t' && byteValue != '\n' {
			return false
		}
	}
	return true
}

func violation(predicate config.Contract, kind ViolationKind, targetID string, reference resolvedDeclaration, extra []compile.SourcePosition) Violation {
	related := []compile.SourcePosition{reference.declaration.Source}
	related = append(related, extra...)
	return Violation{Kind: kind, PredicateID: predicate.ID, TargetID: targetID, ArtifactPath: reference.artifact.Path, Predicate: predicate.Position, RelatedSource: related}
}

func countViolation(predicate config.Contract, targetID, artifactPath string, related []compile.SourcePosition, actual int64) Violation {
	return Violation{
		Kind:          OccurrencesViolation,
		PredicateID:   predicate.ID,
		TargetID:      targetID,
		ArtifactPath:  artifactPath,
		Predicate:     predicate.Position,
		RelatedSource: related,
		ActualCount:   int64Pointer(actual),
		ExpectedCount: predicate.Count,
	}
}

func int64Pointer(value int64) *int64 { return &value }

func predicateDiagnostic(predicate config.Contract, message string) Diagnostic {
	return Diagnostic{Kind: InvalidPredicate, PredicateID: predicate.ID, Predicate: predicate.Position, Message: message}
}

func artifactDiagnostic(artifact serialize.Artifact, kind DiagnosticKind, message string) Diagnostic {
	return Diagnostic{Kind: kind, TargetID: artifact.TargetID, ArtifactPath: artifact.Path, Message: message}
}

func referenceDiagnostic(predicate config.Contract, targetID string, kind DiagnosticKind, related *compile.SourcePosition, message string) Diagnostic {
	result := Diagnostic{Kind: kind, PredicateID: predicate.ID, TargetID: targetID, Predicate: predicate.Position, Message: message}
	if related != nil {
		result.RelatedSource = []compile.SourcePosition{*related}
	}
	return result
}
