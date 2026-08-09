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

func TestV2StructureAssertionAllowsDocumentRootPath(t *testing.T) {
	input := []byte(`[contracts.shape]
kind = "structure"
subject = "artifact"
paths = ["out/*.md"]
format = "yaml"
applies_to = ["codex"]

[[contracts.shape.assertions]]
path = ""
op = "exact_keys"
value = ["policy"]
`)
	registry, diagnostics := ParseContractsForSpec("contracts.toml", input, []string{"codex"}, 2)
	if len(diagnostics) != 0 || len(registry.Contracts) != 1 || registry.Contracts[0].Assertions[0].Path != "" {
		t.Fatalf("registry = %#v, diagnostics = %#v", registry, diagnostics)
	}
}

func TestV1RejectsStructureAsUnknownKind(t *testing.T) {
	input := []byte("[contracts.shape]\nkind = \"structure\"\nsubject = \"source_frontmatter\"\npaths = [\"src/*.md\"]\nassertions = []\n")
	_, diagnostics := ParseContractsForSpec("contracts.toml", input, []string{"codex"}, 1)
	if len(diagnostics) == 0 {
		t.Fatal("structure contract was accepted by v1")
	}
}

func TestV2ParsesOccurrenceAndScopedForbidsSelectors(t *testing.T) {
	input := []byte(`[contracts.ban]
kind = "forbids"
pattern = "deprecated"
paths = ["out/*.md"]
exclude_paths = ["out/skip.md"]
applies_to = ["codex"]
[contracts.count]
kind = "occurrences"
pattern = "reviewer"
paths = ["out/*.md"]
count = 2
applies_to = ["codex"]
`)
	registry, diagnostics := ParseContractsForSpec("contracts.toml", input, []string{"codex"}, 2)
	if len(diagnostics) != 0 || len(registry.Contracts) != 2 {
		t.Fatalf("registry = %#v, diagnostics = %#v", registry, diagnostics)
	}
	if registry.Contracts[0].Paths == nil || registry.Contracts[0].ExcludePaths == nil || registry.Contracts[1].Count == nil || *registry.Contracts[1].Count != 2 {
		t.Fatalf("contract fields = %#v", registry.Contracts)
	}
}

func TestV2RejectsInvalidOccurrenceSelectorsAndSliceSelectors(t *testing.T) {
	inputs := []string{
		`[contracts.item]
kind="occurrences"
slice="span"
pattern="x"
count=1
paths=["out/*.md"]
applies_to=["codex"]
`,
		`[contracts.item]
kind="forbids"
pattern="x"
paths=[]
applies_to=["codex"]
`,
		`[contracts.item]
kind="occurrences"
pattern="x"
count=-1
paths=["out/**"]
applies_to=["codex"]
`,
	}
	for _, input := range inputs {
		_, diagnostics := ParseContractsForSpec("contracts.toml", []byte(input), []string{"codex"}, 2)
		if len(diagnostics) == 0 {
			t.Fatalf("accepted invalid occurrence contract:\n%s", input)
		}
	}
}

func TestV2RejectsSelectorsOnRequiresAndOrder(t *testing.T) {
	input := `[contracts.need]
kind = "requires"
slice = "span"
pattern = "x"
paths = ["out/*.md"]
applies_to = ["codex"]
[contracts.order]
kind = "order"
before = "first"
after = "second"
exclude_paths = ["out/*.md"]
applies_to = ["codex"]
`
	_, diagnostics := ParseContractsForSpec("contracts.toml", []byte(input), []string{"codex"}, 2)
	if len(diagnostics) < 2 {
		t.Fatalf("selectors on non-selector predicates diagnostics = %#v", diagnostics)
	}
}

func TestV1RejectsOccurrencesAndSelectorFields(t *testing.T) {
	input := `[contracts.count]
kind = "occurrences"
pattern = "x"
paths = ["out/*.md"]
count = 1
applies_to = ["codex"]
`
	_, diagnostics := ParseContractsForSpec("contracts.toml", []byte(input), []string{"codex"}, 1)
	if len(diagnostics) == 0 {
		t.Fatal("v1 accepted occurrences")
	}
	for _, field := range []string{"paths", "exclude_paths"} {
		input := "[contracts.ban]\nkind = \"forbids\"\npattern = \"x\"\n" + field + " = [\"out/*.md\"]\napplies_to = [\"codex\"]\n"
		_, diagnostics := ParseContractsForSpec("contracts.toml", []byte(input), []string{"codex"}, 1)
		found := false
		for _, diagnostic := range diagnostics {
			fieldColumn := strings.Index(strings.Split(input, "\n")[3], field) + 1
			if strings.Contains(diagnostic.Message, "contracts.ban."+field) && diagnostic.Line == 4 && diagnostic.Column == fieldColumn {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("v1 accepted forbids %s or did not own its diagnostic: %#v", field, diagnostics)
		}
	}
}

func TestV2SelectorGrammarRejectsFiniteNegativeCasesForTextScopes(t *testing.T) {
	patterns := []string{"/out/*.md", `out\*.md`, "out/\\u0000.md", "out//*.md", "out/./*.md", "out/../*.md", "out/**"}
	for _, kind := range []string{"forbids", "occurrences"} {
		for _, pattern := range patterns {
			t.Run(kind+"_"+strings.ReplaceAll(pattern, "/", "_"), func(t *testing.T) {
				input := "[contracts.item]\nkind = \"" + kind + "\"\npattern = \"x\"\npaths = [\"" + pattern + "\"]\n"
				if kind == "occurrences" {
					input += "count = 0\n"
				}
				input += "applies_to = [\"codex\"]\n"
				_, diagnostics := ParseContractsForSpec("contracts.toml", []byte(input), []string{"codex"}, 2)
				if len(diagnostics) == 0 {
					t.Fatalf("accepted invalid selector %q", pattern)
				}
			})
		}
	}
	for _, field := range []string{"paths", "exclude_paths"} {
		input := "[contracts.ban]\nkind=\"forbids\"\npattern=\"x\"\n" + field + "=[]\napplies_to=[\"codex\"]\n"
		_, diagnostics := ParseContractsForSpec("contracts.toml", []byte(input), []string{"codex"}, 2)
		if len(diagnostics) == 0 {
			t.Fatalf("accepted explicit empty %s", field)
		}
	}
}

func TestV2OccurrenceSliceUsesCurrentHashIDRule(t *testing.T) {
	validID := "count-8cb475554348"
	input := "[contracts." + validID + "]\nkind=\"occurrences\"\nslice=\"span\"\npattern=\"reviewer\"\ncount=1\napplies_to=[\"codex\"]\n"
	registry, diagnostics := ParseContractsForSpec("contracts.toml", []byte(input), []string{"codex"}, 2)
	if len(diagnostics) != 0 || len(registry.Contracts) != 1 {
		t.Fatalf("valid occurrence hash ID = %#v, %#v", registry, diagnostics)
	}
	badInput := strings.Replace(input, validID, "count-wrong", 1)
	_, diagnostics = ParseContractsForSpec("contracts.toml", []byte(badInput), []string{"codex"}, 2)
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, validID) {
		t.Fatalf("mismatched occurrence hash ID diagnostics = %#v", diagnostics)
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
