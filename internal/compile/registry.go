package compile

import (
	"github.com/akitanabe/gunte/internal/config"
	"github.com/akitanabe/gunte/internal/lexer"
	"github.com/akitanabe/gunte/internal/source"
)

// ValidateRegistryIntegrity checks v2 source declarations against syntactic registry references.
func ValidateRegistryIntegrity(specVersion int, registry config.ContractRegistry, units []SourceUnit) []source.Diagnostic {
	if specVersion != 2 {
		return nil
	}
	type declaration struct {
		kind     lexer.MarkerKind
		position SourcePosition
	}
	declarations := map[string]declaration{}
	for _, unit := range units {
		for _, marker := range unit.IR.Markers {
			if marker.Kind != lexer.ContractOpen && marker.Kind != lexer.AnchorMarker {
				continue
			}
			declarations[marker.Token] = declaration{kind: marker.Kind, position: sourcePosition(unit.Path, marker.Position)}
		}
	}
	usedSpans := map[string]bool{}
	usedAnchors := map[string]bool{}
	var diagnostics []source.Diagnostic
	for _, predicate := range registry.Contracts {
		switch predicate.Kind {
		case config.PredicateRequires, config.PredicateForbids:
			if predicate.Slice == "" {
				continue
			}
			decl, exists := declarations[predicate.Slice]
			if !exists || decl.kind != lexer.ContractOpen {
				diagnostics = append(diagnostics, source.Diagnostic{Path: predicate.Position.Path, Line: predicate.Position.Line, Column: predicate.Position.Column, Message: "slice reference must name a declared contract span " + predicate.Slice})
				continue
			}
			usedSpans[predicate.Slice] = true
		case config.PredicateOrder:
			for _, id := range []string{predicate.Before, predicate.After} {
				decl, exists := declarations[id]
				if !exists {
					diagnostics = append(diagnostics, source.Diagnostic{Path: predicate.Position.Path, Line: predicate.Position.Line, Column: predicate.Position.Column, Message: "order reference must name a declared span or anchor " + id})
					continue
				}
				if decl.kind == lexer.AnchorMarker {
					usedAnchors[id] = true
				}
			}
		}
	}
	for _, unit := range units {
		for _, marker := range unit.IR.Markers {
			position := sourcePosition(unit.Path, marker.Position)
			switch marker.Kind {
			case lexer.ContractOpen:
				if !usedSpans[marker.Token] {
					diagnostics = append(diagnostics, diagnosticAt(position, "unused contract span "+marker.Token))
				}
			case lexer.AnchorMarker:
				if !usedAnchors[marker.Token] {
					diagnostics = append(diagnostics, diagnosticAt(position, "unused anchor "+marker.Token))
				}
			}
		}
	}
	return diagnostics
}
