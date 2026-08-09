// Package contract evaluates registry predicates against serialized artifacts.
// Evaluation is a pure calculation and performs no filesystem I/O.
package contract

import (
	"github.com/akitanabe/gunte/internal/compile"
	"github.com/akitanabe/gunte/internal/config"
	"github.com/akitanabe/gunte/internal/serialize"
)

// ViolationKind identifies contract predicate failures.
type ViolationKind string

const (
	RequiresViolation    ViolationKind = "requires_violation"
	ForbidsViolation     ViolationKind = "forbids_violation"
	OccurrencesViolation ViolationKind = "occurrences_violation"
	OrderViolation       ViolationKind = "order_violation"
)

// DiagnosticKind identifies an invalid evaluator input or an unresolved
// registry reference. These diagnostics are distinct from predicate failures.
type DiagnosticKind string

const (
	InvalidArtifactRange  DiagnosticKind = "invalid_artifact_range"
	InvalidPredicate      DiagnosticKind = "invalid_predicate"
	UnresolvedReference   DiagnosticKind = "unresolved_reference"
	ReferenceKindMismatch DiagnosticKind = "reference_kind_mismatch"
	DuplicateDeclaration  DiagnosticKind = "duplicate_declaration"
)

// Violation records one failed predicate × target evaluation. Related source
// positions are ordered as the predicate's references (before/slice/after).
type Violation struct {
	Kind          ViolationKind
	PredicateID   string
	TargetID      string
	ArtifactPath  string
	Predicate     config.ContractPosition
	RelatedSource []compile.SourcePosition
	ActualCount   *int64
	ExpectedCount *int64
}

// Diagnostic records an evaluator input/reference problem without pretending
// that the corresponding predicate has a semantic violation.
type Diagnostic struct {
	Kind          DiagnosticKind
	PredicateID   string
	TargetID      string
	ArtifactPath  string
	Predicate     config.ContractPosition
	RelatedSource []compile.SourcePosition
	Message       string
}

// Result contains deterministic contract failures in registry declaration and
// applies_to order.
type Result struct {
	Violations []Violation
}

// Evaluate checks every registry predicate against all of its applies_to
// targets. It is equivalent to EvaluateTargets with no target restriction.
func Evaluate(registry config.ContractRegistry, artifacts []serialize.Artifact) (Result, []Diagnostic) {
	return EvaluateTargets(registry, artifacts, nil)
}

// EvaluateTargets checks only selected target IDs. A nil or empty selection
// means all targets named by each predicate; selection does not alter registry
// validation and never turns an unselected target into an unresolved reference.
func EvaluateTargets(registry config.ContractRegistry, artifacts []serialize.Artifact, selectedTargets []string) (Result, []Diagnostic) {
	return evaluate(registry, artifacts, selectedTargets)
}
