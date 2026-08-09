package compile

import (
	"strings"
	"testing"

	"github.com/akitanabe/gunte/internal/config"
)

func TestSpecVersionTwoRequiresEverySourceDeclarationToBeSyntacticallyReferenced(t *testing.T) {
	unit := parsedUnit(t, "src/a.md", []byte("<!-- @contract unused -->\nbody\n<!-- @/contract -->\n<!-- @anchor orphan -->\n"))
	registry := config.ContractRegistry{Contracts: []config.Contract{{ID: "other", Kind: config.PredicateForbids, Pattern: "x", AppliesTo: []string{"one"}, Position: config.ContractPosition{Path: "contracts.toml", Line: 1, Column: 1}}}}
	diagnostics := ValidateRegistryIntegrity(2, registry, []SourceUnit{unit})
	if len(diagnostics) != 2 || !strings.Contains(diagnostics[0].Message, "unused contract span unused") || !strings.Contains(diagnostics[1].Message, "unused anchor orphan") {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if diagnostics[0].Path != "src/a.md" || diagnostics[0].Line != 1 || diagnostics[1].Line != 4 {
		t.Fatalf("positions = %#v", diagnostics)
	}
}

func TestSpecVersionTwoRegistryReferencesAreCheckedAcrossAllTargets(t *testing.T) {
	unit := parsedUnit(t, "src/a.md", []byte("<!-- @contract span -->\nbody\n<!-- @/contract -->\n<!-- @anchor before -->\n<!-- @anchor after -->\n"))
	registry := config.ContractRegistry{Contracts: []config.Contract{
		{ID: "need", Kind: config.PredicateRequires, Slice: "span"},
		{ID: "order", Kind: config.PredicateOrder, Before: "before", After: "after"},
	}}
	if diagnostics := ValidateRegistryIntegrity(2, registry, []SourceUnit{unit}); len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if diagnostics := ValidateRegistryIntegrity(1, config.ContractRegistry{}, []SourceUnit{unit}); len(diagnostics) != 0 {
		t.Fatalf("v1 diagnostics = %#v", diagnostics)
	}
}

func TestSpecVersionTwoOccurrenceSliceUsesSpanRegistryIntegrity(t *testing.T) {
	unit := parsedUnit(t, "src/a.md", []byte("<!-- @contract span -->\nbody\n<!-- @/contract -->\n"))
	registry := config.ContractRegistry{Contracts: []config.Contract{{ID: "count", Kind: config.PredicateOccurrences, Slice: "span"}}}
	if diagnostics := ValidateRegistryIntegrity(2, registry, []SourceUnit{unit}); len(diagnostics) != 0 {
		t.Fatalf("occurrence span diagnostics = %#v", diagnostics)
	}
	registry.Contracts[0].Slice = "missing"
	diagnostics := ValidateRegistryIntegrity(2, registry, []SourceUnit{unit})
	if len(diagnostics) != 2 || !strings.Contains(diagnostics[0].Message, "slice reference must name a declared contract span missing") {
		t.Fatalf("missing occurrence span diagnostics = %#v", diagnostics)
	}
}
