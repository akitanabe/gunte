package config

import (
	"strings"
	"testing"
)

func TestV2ParsesValidatedStructureContractData(t *testing.T) {
	input := []byte(`[contracts.shape]
kind = "structure"
subject = "artifact"
paths = ["out/*.yaml"]
format = "yaml"
applies_to = ["codex"]

[[contracts.shape.assertions]]
path = "policy.enabled"
op = "equals"
value = true
`)
	registry, diagnostics := ParseContractsForSpec("contracts.toml", input, []string{"codex"}, 2)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	got := registry.Contracts[0]
	if got.Subject != StructureArtifact || len(got.Paths) != 1 || got.Format != StructureYAML || len(got.Assertions) != 1 || got.Assertions[0].Value == nil {
		t.Fatalf("contract = %#v", got)
	}
}

func TestV1RejectsStructureAsUnknownKind(t *testing.T) {
	input := []byte("[contracts.shape]\nkind = \"structure\"\nsubject = \"source_frontmatter\"\npaths = [\"src/*.md\"]\nassertions = []\n")
	_, diagnostics := ParseContractsForSpec("contracts.toml", input, []string{"codex"}, 1)
	if len(diagnostics) == 0 {
		t.Fatal("structure contract was accepted by v1")
	}
}

func TestStructureContractRejectsInvalidSelectorsSubjectsAndAssertionOperands(t *testing.T) {
	tests := []struct{ name, body string }{
		{name: "double star", body: `subject="source_frontmatter"\npaths=["src/**"]\nassertions=[]`},
		{name: "source format", body: `subject="source_frontmatter"\npaths=["src/*.md"]\nformat="toml"\nassertions=[]`},
		{name: "artifact target", body: `subject="artifact"\npaths=["out/*.json"]\nformat="json"\napplies_to=[]\nassertions=[]`},
		{name: "extra operand", body: `subject="source_frontmatter"\npaths=["src/*.md"]\n[[contracts.shape.assertions]]\npath="x"\nop="exists"\nvalue=true`},
		{name: "float operand", body: `subject="source_frontmatter"\npaths=["src/*.md"]\n[[contracts.shape.assertions]]\npath="x"\nop="equals"\nvalue=1.5`},
		{name: "duplicate set", body: `subject="source_frontmatter"\npaths=["src/*.md"]\n[[contracts.shape.assertions]]\npath="x"\nop="list_set"\nvalue=[true,true]`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := []byte("[contracts.shape]\nkind=\"structure\"\n" + strings.ReplaceAll(test.body, `\n`, "\n") + "\n")
			_, diagnostics := ParseContractsForSpec("contracts.toml", input, []string{"codex"}, 2)
			if len(diagnostics) == 0 {
				t.Fatalf("accepted invalid contract:\n%s", input)
			}
		})
	}
}

func TestStructureAssertionOperandMatrixReportsStableFacts(t *testing.T) {
	tests := []struct{ name, assertion, want string }{
		{"missing value", `path="x"\nop="equals"`, "value is required for equals"},
		{"extra value", `path="x"\nop="exists"\nvalue=true`, "value is not allowed for exists"},
		{"missing count", `path="x"\nop="cardinality"`, "count is required for cardinality"},
		{"extra count", `path="x"\nop="absent"\ncount=1`, "count is not allowed for absent"},
		{"negative count", `path="x"\nop="cardinality"\ncount=-1`, "count must be a non-negative integer"},
		{"exact keys nonstring", `path="x"\nop="exact_keys"\nvalue=["a",1]`, "exact_keys value must be a list of unique strings"},
		{"exact keys duplicate", `path="x"\nop="exact_keys"\nvalue=["a","a"]`, "exact_keys value must be a list of unique strings"},
		{"list order nonlist", `path="x"\nop="list_order"\nvalue="a"`, "list_order value must be a list"},
		{"list set nested", `path="x"\nop="list_set"\nvalue=[[1]]`, "list_set value must contain only unique scalars"},
		{"unknown op", `path="x"\nop="mystery"`, "unknown assertion op"},
		{"invalid path", `path="a..b"\nop="exists"`, "assertion path must contain dot-separated segments or *"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := []byte("[contracts.shape]\nkind=\"structure\"\nsubject=\"source_frontmatter\"\npaths=[\"src/*.md\"]\n[[contracts.shape.assertions]]\n" + strings.ReplaceAll(test.assertion, `\n`, "\n") + "\n")
			_, diagnostics := ParseContractsForSpec("contracts.toml", input, []string{"codex"}, 2)
			assertDiagnostic(t, diagnostics, test.want)
		})
	}
}
