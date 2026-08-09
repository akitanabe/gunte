package structure

import (
	"strings"
	"testing"

	"github.com/akitanabe/gunte/internal/config"
	"github.com/akitanabe/gunte/internal/serialize"
	"github.com/akitanabe/gunte/internal/typeddata"
)

func TestSourceWildcardUsesNestedMappingDeclarationOrder(t *testing.T) {
	root := typeddata.Value{Kind: typeddata.Map, Map: []typeddata.Entry{{Key: "policy", Value: typeddata.Value{Kind: typeddata.Map, Map: []typeddata.Entry{{Key: "second", Value: scalar("b")}, {Key: "first", Value: scalar("a")}}}}}}
	expected := typeddata.Value{Kind: typeddata.String, String: "b"}
	contract := structureContract(config.StructureSourceFrontmatter, config.StructureFormat(""), []config.StructureAssertion{{Path: "policy.*", Op: config.AssertEquals, Value: &expected}})
	failures := EvaluateSources(config.ContractRegistry{Contracts: []config.Contract{contract}}, []SourceDocument{{Path: "src/a.md", Node: &root}})
	if len(failures) != 1 || !strings.Contains(failures[0].Message, "2 matching nodes") {
		t.Fatalf("failures = %#v", failures)
	}
}

func TestArtifactYAMLRejectsDuplicateKeyBeforeLastWin(t *testing.T) {
	contract := structureContract(config.StructureArtifact, config.StructureYAML, []config.StructureAssertion{{Path: "policy.enabled", Op: config.AssertExists}})
	contract.AppliesTo = []string{"codex"}
	contract.Paths = []string{"out/*.md"}
	artifact := serialize.Artifact{TargetID: "codex", SourcePath: "src/a.md", Path: "out/a.md", Profile: config.ProfileYAML, Bytes: []byte("---\npolicy:\n  enabled: true\n  enabled: false\n---\nbody\n---\n")}
	failures := EvaluateArtifacts(config.ContractRegistry{Contracts: []config.Contract{contract}}, []serialize.Artifact{artifact}, nil)
	if len(failures) != 1 || !strings.Contains(failures[0].Message, "duplicate YAML mapping key at policy.enabled") {
		t.Fatalf("failures = %#v", failures)
	}
}

func TestArtifactYAMLReadsOnlyLeadingFrontmatter(t *testing.T) {
	expected := typeddata.Value{Kind: typeddata.Bool, Bool: true}
	contract := structureContract(config.StructureArtifact, config.StructureYAML, []config.StructureAssertion{{Path: "enabled", Op: config.AssertEquals, Value: &expected}})
	contract.AppliesTo = []string{"codex"}
	contract.Paths = []string{"out/*.md"}
	artifact := serialize.Artifact{TargetID: "codex", Path: "out/a.md", Profile: config.ProfileYAML, Bytes: []byte("---\nenabled: true\n---\nbody\n---\nenabled: false\n")}
	if failures := EvaluateArtifacts(config.ContractRegistry{Contracts: []config.Contract{contract}}, []serialize.Artifact{artifact}, nil); len(failures) != 0 {
		t.Fatalf("failures = %#v", failures)
	}
}

func TestArtifactYAMLValidatesEntireMarkdownArtifactAtDocumentRoot(t *testing.T) {
	keys := typeddata.Value{Kind: typeddata.List, List: []typeddata.Value{scalar("policy")}}
	contract := structureContract(config.StructureArtifact, config.StructureYAML, []config.StructureAssertion{{Path: "", Op: config.AssertExactKeys, Value: &keys}})
	contract.AppliesTo = []string{"codex"}
	contract.Paths = []string{"out/*.md"}
	artifact := serialize.Artifact{
		TargetID:   "codex",
		SourcePath: "src/a.md",
		Path:       "out/a.md",
		Profile:    config.ProfileMarkdown,
		Bytes:      []byte("policy:\n  enabled: true\n"),
	}
	if failures := EvaluateArtifacts(config.ContractRegistry{Contracts: []config.Contract{contract}}, []serialize.Artifact{artifact}, nil); len(failures) != 0 {
		t.Fatalf("failures = %#v", failures)
	}
}

func TestArtifactYAMLRejectsMultipleRawMarkdownDocuments(t *testing.T) {
	contract := structureContract(config.StructureArtifact, config.StructureYAML, []config.StructureAssertion{{Path: "", Op: config.AssertExists}})
	contract.AppliesTo = []string{"codex"}
	contract.Paths = []string{"out/*.md"}
	artifact := serialize.Artifact{TargetID: "codex", SourcePath: "src/a.md", Path: "out/a.md", Profile: config.ProfileMarkdown, Bytes: []byte("enabled: true\n---\nenabled: false\n")}
	if failures := EvaluateArtifacts(config.ContractRegistry{Contracts: []config.Contract{contract}}, []serialize.Artifact{artifact}, nil); len(failures) != 1 || !strings.Contains(failures[0].Message, "exactly one YAML document") {
		t.Fatalf("failures = %#v", failures)
	}
}

func TestArtifactYAMLRawRejectsNestedDuplicateStringKey(t *testing.T) {
	contract := structureContract(config.StructureArtifact, config.StructureYAML, []config.StructureAssertion{{Path: "policy.enabled", Op: config.AssertExists}})
	contract.AppliesTo = []string{"codex"}
	contract.Paths = []string{"out/*.md"}
	artifact := serialize.Artifact{TargetID: "codex", SourcePath: "src/a.md", Path: "out/a.md", Profile: config.ProfileMarkdown, Bytes: []byte("policy:\n  enabled: true\n  enabled: false\n")}
	if failures := EvaluateArtifacts(config.ContractRegistry{Contracts: []config.Contract{contract}}, []serialize.Artifact{artifact}, nil); len(failures) != 1 || !strings.Contains(failures[0].Message, "duplicate YAML mapping key at policy.enabled") {
		t.Fatalf("failures = %#v", failures)
	}
}

func TestArtifactYAMLFailureKeepsPredicateArtifactAndSourceOwnership(t *testing.T) {
	keys := typeddata.Value{Kind: typeddata.List, List: []typeddata.Value{scalar("policy")}}
	contract := structureContract(config.StructureArtifact, config.StructureYAML, []config.StructureAssertion{{Path: "", Op: config.AssertExactKeys, Value: &keys}})
	contract.Position = config.ContractPosition{Path: "contracts.toml", Line: 7, Column: 3}
	contract.AppliesTo = []string{"codex"}
	contract.Paths = []string{"out/*.md"}
	artifact := serialize.Artifact{
		TargetID:   "codex",
		SourcePath: "src/a.md",
		Path:       "out/a.md",
		Profile:    config.ProfileMarkdown,
		Bytes:      []byte("policy:\n  enabled: true\nextra: false\n"),
	}
	failures := EvaluateArtifacts(config.ContractRegistry{Contracts: []config.Contract{contract}}, []serialize.Artifact{artifact}, nil)
	if len(failures) != 1 {
		t.Fatalf("failures = %#v", failures)
	}
	failure := failures[0]
	if failure.Contract != contract.Position || failure.Path != artifact.Path || failure.RelatedPath != artifact.SourcePath {
		t.Fatalf("failure ownership = %#v", failure)
	}
}

func TestArtifactDecodersRejectUnsupportedOrAmbiguousTypedDocuments(t *testing.T) {
	tests := []struct {
		name    string
		format  config.StructureFormat
		profile config.Profile
		body    string
		want    string
	}{
		{name: "yaml delimiter", format: config.StructureYAML, profile: config.ProfileYAML, body: "enabled: true\n---\n"},
		{name: "json duplicate", format: config.StructureJSON, profile: config.ProfileJSON, body: `{"enabled":true,"enabled":false}`},
		{name: "json float", format: config.StructureJSON, profile: config.ProfileJSON, body: `{"enabled":1.5}`},
		{name: "json null", format: config.StructureJSON, profile: config.ProfileJSON, body: `{"enabled":null}`},
		{name: "toml datetime", format: config.StructureTOML, profile: config.ProfileTOML, body: "enabled = 1979-05-27T07:32:00Z\n"},
		{name: "nested toml duplicate", format: config.StructureTOML, profile: config.ProfileTOML, body: "[policy]\nenabled = true\nenabled = false\n", want: "already been defined"},
		{name: "profile mismatch", format: config.StructureJSON, profile: config.ProfileTOML, body: "enabled = true\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contract := structureContract(config.StructureArtifact, test.format, []config.StructureAssertion{{Path: "enabled", Op: config.AssertExists}})
			contract.AppliesTo = []string{"codex"}
			contract.Paths = []string{"out/*"}
			artifact := serialize.Artifact{TargetID: "codex", Path: "out/a", Profile: test.profile, Bytes: []byte(test.body)}
			if failures := EvaluateArtifacts(config.ContractRegistry{Contracts: []config.Contract{contract}}, []serialize.Artifact{artifact}, nil); len(failures) != 1 || failures[0].Path != "out/a" || (test.want != "" && !strings.Contains(failures[0].Message, test.want)) {
				t.Fatalf("failures = %#v", failures)
			}
		})
	}
}

func TestAssertionFailuresCoverTypedAndCardinalityBoundaries(t *testing.T) {
	root := typeddata.Value{Kind: typeddata.Map, Map: []typeddata.Entry{
		{Key: "scalar", Value: typeddata.Value{Kind: typeddata.String, String: "1"}},
		{Key: "mapping", Value: typeddata.Value{Kind: typeddata.Map, Map: []typeddata.Entry{{Key: "a", Value: scalar("x")}}}},
		{Key: "ordered", Value: typeddata.Value{Kind: typeddata.List, List: []typeddata.Value{scalar("a"), scalar("b")}}},
		{Key: "typedset", Value: typeddata.Value{Kind: typeddata.List, List: []typeddata.Value{scalar("1")}}},
		{Key: "many", Value: typeddata.Value{Kind: typeddata.List, List: []typeddata.Value{scalar("a"), scalar("b")}}},
	}}
	boolTrue := typeddata.Value{Kind: typeddata.Bool, Bool: true}
	keys := typeddata.Value{Kind: typeddata.List, List: []typeddata.Value{scalar("a"), scalar("b")}}
	reversed := typeddata.Value{Kind: typeddata.List, List: []typeddata.Value{scalar("b"), scalar("a")}}
	wrongSet := typeddata.Value{Kind: typeddata.List, List: []typeddata.Value{scalar("a"), scalar("c")}}
	typedSet := typeddata.Value{Kind: typeddata.List, List: []typeddata.Value{{Kind: typeddata.Int, Int: 1}}}
	zero, one := int64(0), int64(1)
	tests := []struct {
		name      string
		assertion config.StructureAssertion
	}{
		{"exists zero", config.StructureAssertion{Path: "missing", Op: config.AssertExists}},
		{"absent multiple", config.StructureAssertion{Path: "many.*", Op: config.AssertAbsent}},
		{"cardinality zero mismatch", config.StructureAssertion{Path: "missing", Op: config.AssertCardinality, Count: &one}},
		{"cardinality multiple mismatch", config.StructureAssertion{Path: "many.*", Op: config.AssertCardinality, Count: &zero}},
		{"equals typed mismatch", config.StructureAssertion{Path: "scalar", Op: config.AssertEquals, Value: &boolTrue}},
		{"equals multiple", config.StructureAssertion{Path: "many.*", Op: config.AssertEquals, Value: &boolTrue}},
		{"exact keys wrong set", config.StructureAssertion{Path: "mapping", Op: config.AssertExactKeys, Value: &keys}},
		{"exact keys wrong node kind", config.StructureAssertion{Path: "scalar", Op: config.AssertExactKeys, Value: &keys}},
		{"list order mismatch", config.StructureAssertion{Path: "ordered", Op: config.AssertListOrder, Value: &reversed}},
		{"list order wrong node kind", config.StructureAssertion{Path: "scalar", Op: config.AssertListOrder, Value: &reversed}},
		{"list set mismatch", config.StructureAssertion{Path: "ordered", Op: config.AssertListSet, Value: &wrongSet}},
		{"list set typed mismatch", config.StructureAssertion{Path: "typedset", Op: config.AssertListSet, Value: &typedSet}},
		{"list set wrong node kind", config.StructureAssertion{Path: "scalar", Op: config.AssertListSet, Value: &wrongSet}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contract := structureContract(config.StructureSourceFrontmatter, "", []config.StructureAssertion{test.assertion})
			failures := EvaluateSources(config.ContractRegistry{Contracts: []config.Contract{contract}}, []SourceDocument{{Path: "src/a.md", Node: &root}})
			if len(failures) != 1 || !strings.Contains(failures[0].Message, string(test.assertion.Op)) {
				t.Fatalf("failures = %#v", failures)
			}
		})
	}
}

func TestSelectorsReportOnceForZeroMatchAndOverlappingPatterns(t *testing.T) {
	assertion := config.StructureAssertion{Path: "enabled", Op: config.AssertExists}
	sourceContract := structureContract(config.StructureSourceFrontmatter, "", []config.StructureAssertion{assertion})
	sourceContract.Paths = []string{"missing/*.md"}
	if failures := EvaluateSources(config.ContractRegistry{Contracts: []config.Contract{sourceContract}}, nil); len(failures) != 1 {
		t.Fatalf("source zero-match = %#v", failures)
	}
	sourceContract.Paths = []string{"src/*.md", "src/a.*"}
	root := typeddata.Value{Kind: typeddata.Map}
	if failures := EvaluateSources(config.ContractRegistry{Contracts: []config.Contract{sourceContract}}, []SourceDocument{{Path: "src/a.md", Node: &root}}); len(failures) != 1 {
		t.Fatalf("source overlap = %#v", failures)
	}

	artifactContract := structureContract(config.StructureArtifact, config.StructureJSON, []config.StructureAssertion{assertion})
	artifactContract.AppliesTo = []string{"codex"}
	artifactContract.Paths = []string{"missing/*.json"}
	if failures := EvaluateArtifacts(config.ContractRegistry{Contracts: []config.Contract{artifactContract}}, nil, nil); len(failures) != 1 {
		t.Fatalf("artifact zero-match = %#v", failures)
	}
	artifactContract.Paths = []string{"out/*.json", "out/a.*"}
	artifact := serialize.Artifact{TargetID: "codex", Path: "out/a.json", Profile: config.ProfileJSON, Bytes: []byte(`{}`)}
	if failures := EvaluateArtifacts(config.ContractRegistry{Contracts: []config.Contract{artifactContract}}, []serialize.Artifact{artifact}, nil); len(failures) != 1 {
		t.Fatalf("artifact overlap = %#v", failures)
	}
}

func TestTOMLWildcardResolverPreservesNestedDeclarationOrder(t *testing.T) {
	node, err := decodeTOML([]byte("[policy]\nsecond = 2\nfirst = 1\n[policy.nested]\nb = true\na = false\n"))
	if err != nil {
		t.Fatal(err)
	}
	policy := resolve(node, []string{"policy"})[0]
	if got := []string{policy.Map[0].Key, policy.Map[1].Key, policy.Map[2].Key}; strings.Join(got, ",") != "second,first,nested" {
		t.Fatalf("keys = %v", got)
	}
}

func TestArtifactSelectorFailsOnceForEachSelectedTarget(t *testing.T) {
	contract := structureContract(config.StructureArtifact, config.StructureJSON, []config.StructureAssertion{{Path: "enabled", Op: config.AssertExists}})
	contract.AppliesTo = []string{"codex", "claude"}
	contract.Paths = []string{"out/*.json"}
	failures := EvaluateArtifacts(config.ContractRegistry{Contracts: []config.Contract{contract}}, nil, []string{"codex"})
	if len(failures) != 1 || failures[0].TargetID != "codex" {
		t.Fatalf("failures = %#v", failures)
	}
}

func TestAssertionsCheckExactKeysListOrderListSetAndCardinalityAsTypedData(t *testing.T) {
	root := typeddata.Value{Kind: typeddata.Map, Map: []typeddata.Entry{
		{Key: "policy", Value: typeddata.Value{Kind: typeddata.Map, Map: []typeddata.Entry{{Key: "enabled", Value: typeddata.Value{Kind: typeddata.Bool, Bool: true}}, {Key: "name", Value: scalar("x")}}}},
		{Key: "ordered", Value: typeddata.Value{Kind: typeddata.List, List: []typeddata.Value{scalar("a"), scalar("b")}}},
		{Key: "set", Value: typeddata.Value{Kind: typeddata.List, List: []typeddata.Value{scalar("b"), scalar("a")}}},
	}}
	keys := typeddata.Value{Kind: typeddata.List, List: []typeddata.Value{scalar("name"), scalar("enabled")}}
	order := typeddata.Value{Kind: typeddata.List, List: []typeddata.Value{scalar("a"), scalar("b")}}
	set := typeddata.Value{Kind: typeddata.List, List: []typeddata.Value{scalar("a"), scalar("b")}}
	count := int64(2)
	contract := structureContract(config.StructureSourceFrontmatter, "", []config.StructureAssertion{
		{Path: "policy", Op: config.AssertExactKeys, Value: &keys},
		{Path: "ordered", Op: config.AssertListOrder, Value: &order},
		{Path: "set", Op: config.AssertListSet, Value: &set},
		{Path: "policy.*", Op: config.AssertCardinality, Count: &count},
		{Path: "missing", Op: config.AssertAbsent},
	})
	if failures := EvaluateSources(config.ContractRegistry{Contracts: []config.Contract{contract}}, []SourceDocument{{Path: "src/a.md", Node: &root}}); len(failures) != 0 {
		t.Fatalf("failures = %#v", failures)
	}
	root.Map[2].Value.List = []typeddata.Value{scalar("a"), scalar("a")}
	if failures := EvaluateSources(config.ContractRegistry{Contracts: []config.Contract{contract}}, []SourceDocument{{Path: "src/a.md", Node: &root}}); len(failures) != 1 || !strings.Contains(failures[0].Message, "list_set") {
		t.Fatalf("duplicate set failures = %#v", failures)
	}
}

func structureContract(subject config.StructureSubject, format config.StructureFormat, assertions []config.StructureAssertion) config.Contract {
	return config.Contract{ID: "shape", Kind: config.PredicateStructure, Subject: subject, Format: format, Paths: []string{"src/*.md"}, Assertions: assertions, Position: config.ContractPosition{Path: "contracts.toml", Line: 1, Column: 1}}
}
func scalar(value string) typeddata.Value {
	return typeddata.Value{Kind: typeddata.String, String: value}
}
