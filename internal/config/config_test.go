package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestParseOracleConfiguration(t *testing.T) {
	projectBytes, err := os.ReadFile("../../testdata/oracle/input/gunte.toml")
	if err != nil {
		t.Fatal(err)
	}

	project, diagnostics := ParseProject("gunte.toml", projectBytes)
	if len(diagnostics) != 0 {
		t.Fatalf("ParseProject() diagnostics = %v", diagnostics)
	}
	if project.SpecVersion != 1 || project.Project.ID != "tugite" || project.Project.Version != "4.5.0" {
		t.Fatalf("ParseProject() project = %#v", project)
	}
	if len(project.Targets) != 2 || project.Targets[0].ID != "claude" || project.Targets[1].ID != "codex" {
		t.Fatalf("ParseProject() targets = %#v", project.Targets)
	}
	if len(project.Sources.Files) != 40 || project.Sources.Files[0] != "shared/agents/implementer.md" || project.Sources.Files[39] != "shared/VERSION" {
		t.Fatalf("ParseProject() sources = %#v", project.Sources.Files)
	}
	claudeAgent := project.Targets[0].Rules[0]
	if claudeAgent.Profile != ProfileYAML || claudeAgent.Metadata[1].From != "frontmatter:claude.description" || claudeAgent.Metadata[2].Type != MetadataPlainToken {
		t.Fatalf("ParseProject() dotted/plain_token rule = %#v", claudeAgent)
	}
	if project.Targets[0].Rules[3].Profile != ProfileJSON || len(project.Targets[0].Rules[3].Metadata) != 1 {
		t.Fatalf("ParseProject() source-backed json rule = %#v", project.Targets[0].Rules[3])
	}
	if project.Targets[1].Rules[4].Profile != ProfilePlainText || project.Targets[1].Rules[4].ValueFrom != "project:version" {
		t.Fatalf("ParseProject() plain-text rule = %#v", project.Targets[1].Rules[4])
	}
	profiles := map[Profile]bool{}
	for _, target := range project.Targets {
		for _, rule := range target.Rules {
			profiles[rule.Profile] = true
		}
	}
	for _, profile := range []Profile{ProfileMarkdown, ProfileYAML, ProfileTOML, ProfileJSON, ProfilePlainText} {
		if !profiles[profile] {
			t.Errorf("oracle did not preserve profile %q", profile)
		}
	}
	if project.Targets[0].Rules[2].Path != "skills/{1}/references/{2}.md" {
		t.Fatalf("path template was not preserved: %q", project.Targets[0].Rules[2].Path)
	}
	if !claudeAgent.Metadata[0].Required {
		t.Fatal("metadata required must default to true")
	}

	contractsBytes, err := os.ReadFile("../../testdata/oracle/input/contracts.toml")
	if err != nil {
		t.Fatal(err)
	}
	registry, diagnostics := ParseContracts("contracts.toml", contractsBytes, project.TargetIDs())
	if len(diagnostics) != 0 || len(registry.Contracts) != 0 {
		t.Fatalf("ParseContracts() = (%#v, %v)", registry, diagnostics)
	}
}

func TestParseContractsPreservesAppliesToOrder(t *testing.T) {
	input := []byte(`[contracts.both]
kind = "forbids"
pattern = "x"
applies_to = ["two", "one"]
`)
	registry, diagnostics := ParseContracts("contracts.toml", input, []string{"one", "two"})
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	got := registry.Contracts[0].AppliesTo
	if len(got) != 2 || got[0] != "two" || got[1] != "one" {
		t.Fatalf("applies_to order = %v", got)
	}
}

func TestParseProjectValidatesClosedSchemaAndValues(t *testing.T) {
	base := `spec_version = 1
[project]
id = "p"
version = "v"
[sources]
files = ["src/a.md"]
[terms.word]
one = "one"
[targets.one]
output_root = "out"
`
	tests := []struct {
		name    string
		input   string
		message string
	}{
		{"spec version required", strings.Replace(base, "spec_version = 1\n", "", 1), "spec_version is required"},
		{"spec version integer", strings.Replace(base, "spec_version = 1", `spec_version = "1"`, 1), "spec_version must be an integer"},
		{"supported spec version", strings.Replace(base, "spec_version = 1", "spec_version = 3", 1), "spec_version must be 1 or 2"},
		{"project id required", strings.Replace(base, "id = \"p\"\n", "", 1), "project.id is required"},
		{"opaque string nonempty", strings.Replace(base, `version = "v"`, `version = ""`, 1), "project.version must be non-empty"},
		{"opaque string one line", strings.Replace(base, `version = "v"`, `version = """v\n2"""`, 1), "project.version must be a single-line string"},
		{"targets nonempty", strings.Replace(base, "[targets.one]\noutput_root = \"out\"\n", "", 1), "targets must contain at least one target"},
		{"target id", strings.Replace(base, "[targets.one]", "[targets.Bad]", 1), "invalid target ID"},
		{"output root required", strings.Replace(base, "output_root = \"out\"\n", "", 1), "targets.one.output_root is required"},
		{"source files nonempty", strings.Replace(base, `["src/a.md"]`, `[]`, 1), "sources.files must contain at least one path"},
		{"source duplicate", strings.Replace(base, `["src/a.md"]`, `["src/a.md", "src/a.md"]`, 1), "duplicate source path"},
		{"term name", strings.Replace(base, "[terms.word]", "[terms.Bad]", 1), "invalid term name"},
		{"term missing target", strings.Replace(base, "one = \"one\"\n[targets.one]", "[targets.one]", 1), "term word is missing target one"},
		{"term unknown target", strings.Replace(base, "one = \"one\"", "one = \"one\"\ntwo = \"two\"", 1), "term word has unknown target two"},
		{"term value nonempty", strings.Replace(base, `one = "one"`, `one = ""`, 1), "term word.one must be non-empty"},
		{"top unknown", "unknown = true\n" + base, "unknown key unknown"},
		{"project unknown", strings.Replace(base, "id = \"p\"", "id = \"p\"\nunknown = true", 1), "unknown key project.unknown"},
		{"sources unknown", strings.Replace(base, "files = [\"src/a.md\"]", "files = [\"src/a.md\"]\nunknown = true", 1), "unknown key sources.unknown"},
		{"target unknown", base + "unknown = true\n", "unknown key targets.one.unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, diagnostics := ParseProject("case.toml", []byte(tt.input))
			assertDiagnostic(t, diagnostics, tt.message)
		})
	}
}

func TestParseProjectValidatesPaths(t *testing.T) {
	base := `spec_version = 1
[project]
id = "p"
version = "v"
[sources]
files = ["SOURCE"]
[targets.one]
output_root = "ROOT"
[[targets.one.rules]]
match = "src/*.md"
path = "PATH"
profile = "markdown-v1"
`
	tests := []struct{ name, token, value, message string }{
		{"absolute", "SOURCE", "/a", "sources.files[0] must be a relative slash-separated path"},
		{"backslash", "SOURCE", `a\\b`, "sources.files[0] must be a relative slash-separated path"},
		{"empty segment", "ROOT", "a//b", "targets.one.output_root must not contain empty path segments"},
		{"dot", "ROOT", "a/./b", "targets.one.output_root must not contain . or .. path segments"},
		{"parent", "PATH", "../{1}", "targets.one.rules[0].path must not contain . or .. path segments"},
		{"nul", "SOURCE", "a\\u0000b", "sources.files[0] must not contain NUL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := strings.Replace(base, tt.token, tt.value, 1)
			_, diagnostics := ParseProject("paths.toml", []byte(input))
			assertDiagnostic(t, diagnostics, tt.message)
		})
	}
}

func TestOutputRootAllowsRepositoryRootOnlyInSpecVersionTwo(t *testing.T) {
	project := func(version int, root string) string {
		versionValue := "version = \"v\""
		if version == 2 {
			versionValue = "version = \"v\""
		}
		return fmt.Sprintf(`spec_version = %d
[project]
id = "p"
%s
[sources]
files = ["src/a.md"]
[targets.one]
output_root = %q
[[targets.one.rules]]
match = "src/*.md"
path = "AGENTS.md"
profile = "markdown-v1"
`, version, versionValue, root)
	}

	if _, diagnostics := ParseProject("v2.toml", []byte(project(2, "."))); len(diagnostics) != 0 {
		t.Fatalf("v2 root target diagnostics = %#v", diagnostics)
	}
	for _, test := range []struct {
		name, input, want string
	}{
		{name: "v1 root", input: project(1, "."), want: "targets.one.output_root must not contain . or .. path segments"},
		{name: "v2 empty", input: project(2, ""), want: "targets.one.output_root must be a relative slash-separated path"},
		{name: "source path", input: strings.Replace(project(2, "."), `files = ["src/a.md"]`, `files = ["."]`, 1), want: "sources.files[0] must not contain . or .. path segments"},
		{name: "managed root", input: strings.Replace(project(2, "."), `[targets.one]`, "[targets.one]\nmanaged_roots = [\".\"]", 1), want: "targets.one.managed_roots[0] must not contain . or .. path segments"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, diagnostics := ParseProject("case.toml", []byte(test.input))
			assertDiagnostic(t, diagnostics, test.want)
		})
	}
}

func TestIdentifierGrammarAndOrderedSources(t *testing.T) {
	targetCases := []struct {
		id    string
		valid bool
	}{
		{"a", true},
		{"a__-", true},
		{"a" + strings.Repeat("0", 31), true},
		{"1target", false},
		{"Target", false},
		{"a" + strings.Repeat("0", 32), false},
	}
	for _, tt := range targetCases {
		t.Run("target_"+tt.id, func(t *testing.T) {
			input := fmt.Sprintf(`spec_version = 1
[project]
id = "p"
version = "v"
[sources]
files = ["first", "second", "third"]
[targets.%q]
output_root = "out"
`, tt.id)
			project, diagnostics := ParseProject("identifiers.toml", []byte(input))
			if tt.valid {
				if len(diagnostics) != 0 {
					t.Fatalf("valid target %q diagnostics = %v", tt.id, diagnostics)
				}
				want := []string{"first", "second", "third"}
				if !reflect.DeepEqual(project.Sources.Files, want) {
					t.Fatalf("source order = %v, want %v", project.Sources.Files, want)
				}
				return
			}
			assertDiagnostic(t, diagnostics, "invalid target ID")
		})
	}

	termCases := []struct {
		name  string
		value string
		valid bool
	}{
		{"a", "value", true},
		{"a_b1", "value", true},
		{"1term", "value", false},
		{"a__b", "value", false},
		{"a_", "value", false},
		{"a-b", "value", false},
		{"term", "line1\nline2", false},
	}
	for _, tt := range termCases {
		t.Run("term_"+tt.name, func(t *testing.T) {
			input := fmt.Sprintf(`spec_version = 1
[project]
id = "p"
version = "v"
[sources]
files = ["source"]
[terms.%q]
one = %s
[targets.one]
output_root = "out"
`, tt.name, quoteTOMLValue(tt.value))
			_, diagnostics := ParseProject("terms.toml", []byte(input))
			if tt.valid {
				if len(diagnostics) != 0 {
					t.Fatalf("valid term %q diagnostics = %v", tt.name, diagnostics)
				}
				return
			}
			if strings.Contains(tt.value, "\n") {
				assertDiagnostic(t, diagnostics, "must be a single-line string")
			} else {
				assertDiagnostic(t, diagnostics, "invalid term name")
			}
		})
	}
}

func quoteTOMLValue(value string) string {
	if strings.Contains(value, "\n") {
		return `"""` + value + `"""`
	}
	return fmt.Sprintf("%q", value)
}

func TestParseProjectValidatesRuleProfiles(t *testing.T) {
	base := `spec_version = 1
[project]
id = "p"
version = "v"
[sources]
files = ["src/a"]
[targets.one]
output_root = "out"
[[targets.one.rules]]
match = "src/*"
path = "{1}"
PROFILE_FIELDS
`
	tests := []struct{ name, fields, message string }{
		{"markdown accepts header", `profile = "markdown-v1"` + "\n" + `header = "generated"`, ""},
		{"yaml accepts empty metadata preserve branch", `profile = "markdown+yaml-frontmatter-v1"`, ""},
		{"toml accepts body and metadata", `profile = "toml-v1"` + "\n" + `body_field = "body"` + "\n" + `metadata = [{ field = "name", from = "core:name", type = "string" }]`, ""},
		{"json accepts metadata", `profile = "json-v1"` + "\n" + `metadata = [{ field = "name", from = "core:name", type = "string" }]`, ""},
		{"plain text requires value", `profile = "plain-text-v1"`, "value_from is required"},
		{"multiline text forbids metadata", `profile = "multiline-text-v1"` + "\nmetadata = []", "metadata is not allowed"},
		{"markdown forbids metadata", `profile = "markdown-v1"` + "\n" + `metadata = []`, "metadata is not allowed"},
		{"yaml forbids body field", `profile = "markdown+yaml-frontmatter-v1"` + "\n" + `body_field = "body"`, "body_field is not allowed"},
		{"toml requires producer", `profile = "toml-v1"`, "toml-v1 rule needs a content producer"},
		{"json forbids header", `profile = "json-v1"` + "\n" + `header = "x"`, "header is not allowed"},
		{"json forbids body field", `profile = "json-v1"` + "\n" + `body_field = "body"`, "body_field is not allowed"},
		{"plain text forbids metadata", `profile = "plain-text-v1"` + "\n" + `value_from = "project:version"` + "\nmetadata = []", "metadata is not allowed"},
		{"unknown profile lists multiline text", `profile = "html-v1"`, "multiline-text-v1"},
		{"header nonempty", `profile = "markdown-v1"` + "\n" + `header = ""`, "header must be non-empty"},
		{"header one line", `profile = "markdown-v1"` + "\n" + `header = """a\nb"""`, "header must be a single-line string"},
		{"rule unknown", `profile = "markdown-v1"` + "\nunknown = true", "unknown key targets.one.rules[0].unknown"},
		{"metadata unknown", `profile = "json-v1"` + "\n" + `metadata = [{ field = "name", from = "core:name", type = "string", unknown = true }]`, "unknown key targets.one.rules[0].metadata[0].unknown"},
		{"metadata type", `profile = "json-v1"` + "\n" + `metadata = [{ field = "name", from = "core:name", type = "token" }]`, "metadata type must be one of"},
		{"metadata field required", `profile = "json-v1"` + "\n" + `metadata = [{ from = "core:name", type = "string" }]`, "field is required"},
		{"metadata from required", `profile = "json-v1"` + "\n" + `metadata = [{ field = "name", type = "string" }]`, "from is required"},
		{"metadata type required", `profile = "json-v1"` + "\n" + `metadata = [{ field = "name", from = "core:name" }]`, "type is required"},
		{"metadata required boolean", `profile = "json-v1"` + "\n" + `metadata = [{ field = "name", from = "core:name", type = "string", required = "yes" }]`, "required must be a boolean"},
		{"metadata field grammar", `profile = "json-v1"` + "\n" + `metadata = [{ field = "bad.name", from = "core:name", type = "string" }]`, "invalid metadata field"},
		{"metadata duplicate", `profile = "json-v1"` + "\n" + `metadata = [{ field = "name", from = "core:name", type = "string" }, { field = "name", from = "core:name", type = "string" }]`, "duplicate metadata field name"},
		{"body collision", `profile = "toml-v1"` + "\n" + `body_field = "body"` + "\n" + `metadata = [{ field = "body", from = "core:name", type = "string" }]`, "body_field conflicts with metadata field body"},
		{"yaml reserved field", `profile = "markdown+yaml-frontmatter-v1"` + "\n" + `metadata = [{ field = "true", from = "core:name", type = "string" }]`, "reserved YAML metadata field true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, diagnostics := ParseProject("profiles.toml", []byte(strings.Replace(base, "PROFILE_FIELDS", tt.fields, 1)))
			if tt.message == "" {
				if len(diagnostics) != 0 {
					t.Fatalf("unexpected diagnostics: %v", diagnostics)
				}
				return
			}
			assertDiagnostic(t, diagnostics, tt.message)
		})
	}
}

func TestProfileOptionalFieldMatrix(t *testing.T) {
	type fieldExpectation struct {
		field   string
		allowed bool
	}
	matrix := map[Profile][]fieldExpectation{
		ProfileMarkdown:      {{"header", true}, {"metadata", false}, {"body_field", false}, {"value_from", false}},
		ProfileYAML:          {{"header", true}, {"metadata", true}, {"body_field", false}, {"value_from", false}},
		ProfileTOML:          {{"header", true}, {"metadata", true}, {"body_field", true}, {"value_from", false}},
		ProfileJSON:          {{"header", false}, {"metadata", true}, {"body_field", false}, {"value_from", false}},
		ProfilePlainText:     {{"header", false}, {"metadata", false}, {"body_field", false}, {"value_from", true}},
		ProfileMultilineText: {{"header", false}, {"metadata", false}, {"body_field", false}, {"value_from", false}},
	}
	profiles := []Profile{ProfileMarkdown, ProfileYAML, ProfileTOML, ProfileJSON, ProfilePlainText, ProfileMultilineText}
	for _, profile := range profiles {
		for _, expectation := range matrix[profile] {
			t.Run(string(profile)+"_"+expectation.field, func(t *testing.T) {
				input := projectWithOptionalRuleField(profile, expectation.field)
				_, diagnostics := ParseProject("profile-matrix.toml", []byte(input))
				if expectation.allowed {
					if len(diagnostics) != 0 {
						t.Fatalf("allowed %s.%s diagnostics = %v", profile, expectation.field, diagnostics)
					}
					return
				}
				assertDiagnostic(t, diagnostics, expectation.field+" is not allowed")
			})
		}
	}
}

func TestMultilineTextProfileIsAvailableInBothSpecVersions(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(fmt.Sprintf("v%d", version), func(t *testing.T) {
			projectVersion := `version = "v"`
			if version == 2 {
				projectVersion = `version_from = "VERSION"`
			}
			input := fmt.Sprintf(`spec_version = %d
[project]
id = "p"
%s
[sources]
files = ["src/a.html"]
[targets.one]
output_root = "out"
[[targets.one.rules]]
match = "src/*"
path = "{1}"
profile = "multiline-text-v1"
`, version, projectVersion)
			project, diagnostics := ParseProject("gunte.toml", []byte(input))
			if len(diagnostics) != 0 || project.Targets[0].Rules[0].Profile != ProfileMultilineText {
				t.Fatalf("ParseProject() = %#v, %#v", project, diagnostics)
			}
		})
	}
}

func projectWithOptionalRuleField(profile Profile, field string) string {
	fieldDeclaration := map[string]string{
		"header":     `header = "generated"`,
		"metadata":   `metadata = [{ field = "name", from = "core:name", type = "string" }]`,
		"body_field": `body_field = "body"`,
		"value_from": `value_from = "project:version"`,
	}[field]
	var support string
	if profile == ProfileTOML && field == "value_from" {
		support = "\nheader = \"generated\""
	}
	if profile == ProfilePlainText && field != "value_from" {
		support = "\nvalue_from = \"project:version\""
	}
	return fmt.Sprintf(`spec_version = 1
[project]
id = "p"
version = "v"
[sources]
files = ["src/a"]
[targets.one]
output_root = "out"
[[targets.one.rules]]
match = "src/*"
path = "{1}"
profile = %q
%s%s
`, profile, fieldDeclaration, support)
}

func TestMetadataRequiredValues(t *testing.T) {
	input := []byte(`spec_version = 1
[project]
id = "p"
version = "v"
[sources]
files = ["src/a"]
[targets.one]
output_root = "out"
[[targets.one.rules]]
match = "src/*"
path = "{1}"
profile = "json-v1"
metadata = [
  { field = "defaulted", from = "core:name", type = "string" },
  { field = "optional", from = "core:name", type = "string", required = false },
]
`)
	project, diagnostics := ParseProject("metadata-required.toml", input)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	metadata := project.Targets[0].Rules[0].Metadata
	if len(metadata) != 2 || !metadata[0].Required || metadata[1].Required {
		t.Fatalf("required values = %#v", metadata)
	}
}

func TestRuleRequiredFields(t *testing.T) {
	base := `spec_version = 1
[project]
id = "p"
version = "v"
[sources]
files = ["src/a"]
[targets.one]
output_root = "out"
[[targets.one.rules]]
match = "src/*"
path = "{1}"
profile = "markdown-v1"
`
	for _, tt := range []struct{ name, line, message string }{
		{"match", `match = "src/*"` + "\n", "match is required"},
		{"path", `path = "{1}"` + "\n", "path is required"},
		{"profile", `profile = "markdown-v1"` + "\n", "profile is required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, diagnostics := ParseProject("required.toml", []byte(strings.Replace(base, tt.line, "", 1)))
			assertDiagnostic(t, diagnostics, tt.message)
		})
	}
}

func TestProjectSchemaTypesAndDefaults(t *testing.T) {
	base := `spec_version = 1
[project]
id = "p"
version = "v"
[sources]
files = ["src/a"]
[terms.word]
one = "value"
[targets.one]
output_root = "out"
`
	withoutTerms := strings.Replace(base, "[terms.word]\none = \"value\"\n", "", 1)
	termsWrongType := strings.Replace(withoutTerms, "[project]", "terms = \"bad\"\n[project]", 1)
	withoutTarget := strings.Replace(withoutTerms, "[targets.one]\noutput_root = \"out\"\n", "", 1)
	targetWrongType := strings.Replace(withoutTarget, "[project]", "targets = { one = \"bad\" }\n[project]", 1)
	tests := []struct{ name, input, message string }{
		{"project table", strings.Replace(base, "[project]\nid = \"p\"\nversion = \"v\"", `project = "bad"`, 1), "project table is required"},
		{"project id string", strings.Replace(base, `id = "p"`, `id = 1`, 1), "project.id must be a string"},
		{"project version string", strings.Replace(base, `version = "v"`, `version = 1`, 1), "project.version must be a string"},
		{"sources table", strings.Replace(base, "[sources]\nfiles = [\"src/a\"]", `sources = "bad"`, 1), "sources table is required"},
		{"sources files array", strings.Replace(base, `files = ["src/a"]`, `files = "src/a"`, 1), "sources.files must be an array of strings"},
		{"terms table", termsWrongType, "terms must be a table"},
		{"term value string", strings.Replace(base, `one = "value"`, `one = 1`, 1), "terms.word.one must be a string"},
		{"target table", targetWrongType, "target one must be a table"},
		{"output root string", strings.Replace(base, `output_root = "out"`, `output_root = 1`, 1), "targets.one.output_root must be a string"},
		{"rules array", base + "rules = \"bad\"\n", "targets.one.rules must be an array"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, diagnostics := ParseProject("types.toml", []byte(tt.input))
			assertDiagnostic(t, diagnostics, tt.message)
		})
	}
	project, diagnostics := ParseProject("defaults.toml", []byte(base))
	if len(diagnostics) != 0 || len(project.Targets) != 1 || project.Targets[0].Rules == nil || len(project.Targets[0].Rules) != 0 {
		t.Fatalf("rules default = (%#v, %v)", project.Targets, diagnostics)
	}
}

func TestRuleSchemaTypesAndMetadataDefault(t *testing.T) {
	base := `spec_version = 1
[project]
id = "p"
version = "v"
[sources]
files = ["src/a"]
[targets.one]
output_root = "out"
[[targets.one.rules]]
match = "src/*"
path = "{1}"
profile = "markdown-v1"
`
	tests := []struct{ name, fields, message string }{
		{"match string", "match = 1\npath = \"{1}\"\nprofile = \"markdown-v1\"", "match must be a string"},
		{"path string", "match = \"src/*\"\npath = 1\nprofile = \"markdown-v1\"", "path must be a string"},
		{"profile string", "match = \"src/*\"\npath = \"{1}\"\nprofile = 1", "profile must be a string"},
		{"header string", "match = \"src/*\"\npath = \"{1}\"\nprofile = \"markdown-v1\"\nheader = 1", "header must be a string"},
		{"metadata array", "match = \"src/*\"\npath = \"{1}\"\nprofile = \"json-v1\"\nmetadata = \"bad\"", "metadata must be an array"},
		{"body field string", "match = \"src/*\"\npath = \"{1}\"\nprofile = \"toml-v1\"\nheader = \"generated\"\nbody_field = true", "body_field must be a string"},
		{"value from string", "match = \"src/*\"\npath = \"{1}\"\nprofile = \"plain-text-v1\"\nvalue_from = true", "value_from must be a string"},
		{"metadata field string", "match = \"src/*\"\npath = \"{1}\"\nprofile = \"json-v1\"\nmetadata = [{ field = 1, from = \"core:name\", type = \"string\" }]", "field must be a string"},
		{"metadata from string", "match = \"src/*\"\npath = \"{1}\"\nprofile = \"json-v1\"\nmetadata = [{ field = \"name\", from = 1, type = \"string\" }]", "from must be a string"},
		{"metadata type string", "match = \"src/*\"\npath = \"{1}\"\nprofile = \"json-v1\"\nmetadata = [{ field = \"name\", from = \"core:name\", type = 1 }]", "type must be a string"},
		{"metadata required bool", "match = \"src/*\"\npath = \"{1}\"\nprofile = \"json-v1\"\nmetadata = [{ field = \"name\", from = \"core:name\", type = \"string\", required = 1 }]", "required must be a boolean"},
	}
	prefix := "match = \"src/*\"\npath = \"{1}\"\nprofile = \"markdown-v1\""
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := strings.Replace(base, prefix, tt.fields, 1)
			_, diagnostics := ParseProject("rule-types.toml", []byte(input))
			assertDiagnostic(t, diagnostics, tt.message)
		})
	}
	project, diagnostics := ParseProject("metadata-default.toml", []byte(base))
	if len(diagnostics) != 0 || project.Targets[0].Rules[0].Metadata == nil || len(project.Targets[0].Rules[0].Metadata) != 0 {
		t.Fatalf("metadata default = (%#v, %v)", project.Targets[0].Rules[0].Metadata, diagnostics)
	}
}

func TestContractSchemaTypes(t *testing.T) {
	base := `[contracts.item]
kind = "order"
before = "first"
after = "second"
applies_to = ["one"]
`
	tests := []struct{ name, old, replacement, message string }{
		{"kind string", `kind = "order"`, `kind = 1`, "kind must be a string"},
		{"slice string", `kind = "order"`, "kind = \"forbids\"\npattern = \"x\"\nslice = 1", "slice must be a string"},
		{"pattern string", `kind = "order"`, "kind = \"forbids\"\npattern = 1", "pattern must be a string"},
		{"before string", `before = "first"`, `before = 1`, "before must be a string"},
		{"after string", `after = "second"`, `after = 1`, "after must be a string"},
		{"applies array", `applies_to = ["one"]`, `applies_to = "one"`, "applies_to must be an array of strings"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, diagnostics := ParseContracts("contract-types.toml", []byte(strings.Replace(base, tt.old, tt.replacement, 1)), []string{"one"})
			assertDiagnostic(t, diagnostics, tt.message)
		})
	}
}

func TestHeaderDiagnosticPointsToInvalidRule(t *testing.T) {
	input := []byte(`spec_version = 1
[project]
id = "p"
version = "v"
[sources]
files = ["src/a"]
[targets.one]
output_root = "out"
[[targets.one.rules]]
match = "src/a"
path = "a"
profile = "markdown-v1"
header = "valid"
[[targets.one.rules]]
match = "src/b"
path = "b"
profile = "markdown-v1"
header = """invalid
header"""
`)
	_, diagnostics := ParseProject("headers.toml", input)
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Message, "header must be a single-line string") {
			if diagnostic.Line != 18 {
				t.Fatalf("header diagnostic line = %d, want 18", diagnostic.Line)
			}
			return
		}
	}
	t.Fatalf("header diagnostic missing: %v", diagnostics)
}

func TestParseContractsValidatesPredicateSchemas(t *testing.T) {
	valid := `[contracts.need]
kind = "requires"
slice = "main"
pattern = "reviewer"
applies_to = ["one"]
[contracts.ban]
kind = "forbids"
pattern = "bad"
applies_to = ["one"]
[contracts.sequence]
kind = "order"
before = "first"
after = "second"
applies_to = ["one"]
`
	registry, diagnostics := ParseContracts("contracts.toml", []byte(valid), []string{"one"})
	if len(diagnostics) != 0 || len(registry.Contracts) != 3 || registry.Contracts[0].ID != "need" {
		t.Fatalf("ParseContracts(valid) = (%#v, %v)", registry, diagnostics)
	}

	tests := []struct{ name, input, message string }{
		{"top unknown", "unknown = true\n[contracts]", "unknown key unknown"},
		{"predicate id", strings.Replace(valid, "[contracts.need]", "[contracts.bad.name]", 1), "unknown key contracts.bad.name"},
		{"unknown field", strings.Replace(valid, `slice = "main"`, `slice = "main"`+"\nunknown = true", 1), "unknown key contracts.need.unknown"},
		{"unknown kind lists known", strings.Replace(valid, `kind = "requires"`, `kind = "sometimes"`, 1), "kind must be one of requires, forbids, order"},
		{"requires exact", strings.Replace(valid, `slice = "main"`, `slice = "main"`+"\nbefore = \"x\"", 1), "before is not allowed for requires"},
		{"requires slice required", strings.Replace(valid, `slice = "main"`+"\n", "", 1), "slice is required for requires"},
		{"forbids optional slice", strings.Replace(valid, `pattern = "bad"`, `pattern = "bad"`+"\nslice = \"part\"", 1), ""},
		{"order exact", strings.Replace(valid, `before = "first"`, `before = "first"`+"\npattern = \"x\"", 1), "pattern is not allowed for order"},
		{"order before required", strings.Replace(valid, `before = "first"`+"\n", "", 1), "before is required for order"},
		{"pattern nonempty", strings.Replace(valid, `pattern = "reviewer"`, `pattern = ""`, 1), "pattern must be non-empty"},
		{"pattern edge whitespace", strings.Replace(valid, `pattern = "reviewer"`, `pattern = " reviewer"`, 1), "pattern must not have leading or trailing whitespace"},
		{"applies nonempty", strings.Replace(valid, `["one"]`, `[]`, 1), "applies_to must contain at least one target"},
		{"applies required", strings.Replace(valid, `applies_to = ["one"]`+"\n", "", 1), "applies_to is required"},
		{"applies unknown", strings.Replace(valid, `["one"]`, `["two"]`, 1), "unknown target two"},
		{"applies duplicate", strings.Replace(valid, `["one"]`, `["one", "one"]`, 1), "duplicate target one"},
		{"slice id", strings.Replace(valid, `slice = "main"`, `slice = "bad.name"`, 1), "invalid slice ID"},
		{"slice nonempty", strings.Replace(valid, `slice = "main"`, `slice = ""`, 1), "invalid slice ID"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, diagnostics := ParseContracts("contracts.toml", []byte(tt.input), []string{"one"})
			if tt.message == "" {
				if len(diagnostics) != 0 {
					t.Fatalf("unexpected diagnostics: %v", diagnostics)
				}
				return
			}
			assertDiagnostic(t, diagnostics, tt.message)
		})
	}
}

func TestPredicateIDGrammarWithQuotedKeys(t *testing.T) {
	tests := []struct {
		id    string
		valid bool
	}{
		{"A", true},
		{"A_-0", true},
		{"A" + strings.Repeat("0", 63), true},
		{"1predicate", false},
		{"_predicate", false},
		{"predicate.name", false},
		{"A" + strings.Repeat("0", 64), false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			input := fmt.Sprintf(`[contracts.%q]
kind = "forbids"
pattern = "x"
applies_to = ["one"]
`, tt.id)
			_, diagnostics := ParseContracts("predicate-ids.toml", []byte(input), []string{"one"})
			if tt.valid {
				if len(diagnostics) != 0 {
					t.Fatalf("valid predicate %q diagnostics = %v", tt.id, diagnostics)
				}
				return
			}
			assertDiagnostic(t, diagnostics, "invalid predicate ID")
		})
	}
}

func TestPredicateKindFieldMatrix(t *testing.T) {
	type expectation string
	const (
		required  expectation = "required"
		optional  expectation = "optional"
		forbidden expectation = "forbidden"
	)
	matrix := map[PredicateKind]map[string]expectation{
		PredicateRequires: {"slice": required, "pattern": required, "before": forbidden, "after": forbidden},
		PredicateForbids:  {"slice": optional, "pattern": required, "before": forbidden, "after": forbidden},
		PredicateOrder:    {"slice": forbidden, "pattern": forbidden, "before": required, "after": required},
	}
	kinds := []PredicateKind{PredicateRequires, PredicateForbids, PredicateOrder}
	fields := []string{"slice", "pattern", "before", "after"}
	for _, kind := range kinds {
		for _, field := range fields {
			state := matrix[kind][field]
			t.Run(string(kind)+"_"+field, func(t *testing.T) {
				input := contractForKindField(kind, field, string(state))
				_, diagnostics := ParseContracts("kind-matrix.toml", []byte(input), []string{"one"})
				switch state {
				case optional:
					if len(diagnostics) != 0 {
						t.Fatalf("optional %s.%s diagnostics = %v", kind, field, diagnostics)
					}
				case required:
					assertDiagnostic(t, diagnostics, field+" is required for "+string(kind))
				case forbidden:
					assertDiagnostic(t, diagnostics, field+" is not allowed for "+string(kind))
				}
			})
		}
	}
}

func contractForKindField(kind PredicateKind, field, state string) string {
	values := map[string]string{
		"slice":   `"part"`,
		"pattern": `"token"`,
		"before":  `"first"`,
		"after":   `"second"`,
	}
	requiredByKind := map[PredicateKind][]string{
		PredicateRequires: {"slice", "pattern"},
		PredicateForbids:  {"pattern"},
		PredicateOrder:    {"before", "after"},
	}
	var declarations []string
	for _, candidate := range requiredByKind[kind] {
		if candidate != field {
			declarations = append(declarations, candidate+" = "+values[candidate])
		}
	}
	if state != "required" {
		declarations = append(declarations, field+" = "+values[field])
	}
	return fmt.Sprintf(`[contracts.item]
kind = %q
%s
applies_to = ["one"]
`, kind, strings.Join(declarations, "\n"))
}

func TestContractArrayElementAndReferenceIDValidation(t *testing.T) {
	base := `[contracts.item]
kind = "order"
before = "first"
after = "second"
applies_to = ["one"]
`
	tests := []struct{ name, input, message string }{
		{"applies element string", strings.Replace(base, `["one"]`, `["one", 2]`, 1), "applies_to entries must be strings"},
		{"before ID", strings.Replace(base, `before = "first"`, `before = "bad.name"`, 1), "invalid before ID"},
		{"after ID", strings.Replace(base, `after = "second"`, `after = "bad.name"`, 1), "invalid after ID"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, diagnostics := ParseContracts("references.toml", []byte(tt.input), []string{"one"})
			assertDiagnostic(t, diagnostics, tt.message)
		})
	}
}

func TestDuplicateDynamicIDsAreRejected(t *testing.T) {
	project := `spec_version = 1
[project]
id = "p"
version = "v"
[sources]
files = ["a"]
[targets.one]
output_root = "one"
[targets.one]
output_root = "two"
`
	if _, diagnostics := ParseProject("gunte.toml", []byte(project)); len(diagnostics) != 1 {
		t.Fatalf("duplicate target diagnostics = %v", diagnostics)
	}
	contracts := `[contracts.same]
kind = "forbids"
pattern = "x"
applies_to = ["one"]
[contracts.same]
kind = "forbids"
pattern = "y"
applies_to = ["one"]
`
	if _, diagnostics := ParseContracts("contracts.toml", []byte(contracts), []string{"one"}); len(diagnostics) != 1 {
		t.Fatalf("duplicate predicate diagnostics = %v", diagnostics)
	}
}

func TestIdentifierLengthBoundaries(t *testing.T) {
	validTarget := "a" + strings.Repeat("0", 31)
	invalidTarget := validTarget + "0"
	base := `spec_version = 1
[project]
id = "p"
version = "v"
[sources]
files = ["a"]
[targets.TARGET]
output_root = "out"
`
	if _, diagnostics := ParseProject("gunte.toml", []byte(strings.Replace(base, "TARGET", validTarget, 1))); len(diagnostics) != 0 {
		t.Fatalf("32-byte target ID diagnostics = %v", diagnostics)
	}
	_, diagnostics := ParseProject("gunte.toml", []byte(strings.Replace(base, "TARGET", invalidTarget, 1)))
	assertDiagnostic(t, diagnostics, "invalid target ID")

	validPredicate := "A" + strings.Repeat("0", 63)
	invalidPredicate := validPredicate + "0"
	contract := `[contracts.PREDICATE]
kind = "forbids"
pattern = "x"
applies_to = ["one"]
`
	if _, diagnostics := ParseContracts("contracts.toml", []byte(strings.Replace(contract, "PREDICATE", validPredicate, 1)), []string{"one"}); len(diagnostics) != 0 {
		t.Fatalf("64-byte predicate ID diagnostics = %v", diagnostics)
	}
	_, diagnostics = ParseContracts("contracts.toml", []byte(strings.Replace(contract, "PREDICATE", invalidPredicate, 1)), []string{"one"})
	assertDiagnostic(t, diagnostics, "invalid predicate ID")
}

func TestValidationDiagnosticsAreAggregatedAndLocated(t *testing.T) {
	input := []byte(`spec_version = 3
[project]
id = ""
version = "ok"
[sources]
files = []
[targets.one]
output_root = "out"
`)
	_, diagnostics := ParseProject("broken.toml", input)
	want := map[string]struct{ line, column int }{
		"spec_version must be 1 or 2":                  {1, 1},
		"project.id must be non-empty":                 {3, 1},
		"sources.files must contain at least one path": {6, 1},
	}
	for message, position := range want {
		diagnostic := findDiagnostic(t, diagnostics, message)
		if diagnostic.Path != "broken.toml" || diagnostic.Line != position.line || diagnostic.Column != position.column {
			t.Errorf("diagnostic %q position = %s:%d:%d, want broken.toml:%d:%d", message, diagnostic.Path, diagnostic.Line, diagnostic.Column, position.line, position.column)
		}
	}
}

func TestContractValidationDiagnosticLocation(t *testing.T) {
	input := []byte(`[contracts.item]
kind = "forbids"
pattern = " invalid "
applies_to = ["one"]
`)
	_, diagnostics := ParseContracts("contracts.toml", input, []string{"one"})
	diagnostic := findDiagnostic(t, diagnostics, "pattern must not have leading or trailing whitespace")
	if diagnostic.Path != "contracts.toml" || diagnostic.Line != 3 || diagnostic.Column != 1 {
		t.Fatalf("contract diagnostic = %#v, want contracts.toml:3:1", diagnostic)
	}
}

func TestParseContractsPreservesPredicateTablePosition(t *testing.T) {
	input := []byte("# header\n\n[contracts.first]\nkind = \"forbids\"\npattern = \"x\"\napplies_to = [\"one\"]\n\n   [contracts.'second'] # trailing comment\nkind = \"forbids\"\npattern = \"y\"\napplies_to = [\"one\"]\n")
	registry, diagnostics := ParseContracts("contracts.toml", input, []string{"one"})
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if len(registry.Contracts) != 2 {
		t.Fatalf("contracts = %#v", registry.Contracts)
	}
	want := []ContractPosition{{Path: "contracts.toml", Line: 3, Column: 1}, {Path: "contracts.toml", Line: 8, Column: 4}}
	for index, contract := range registry.Contracts {
		if contract.Position != want[index] {
			t.Errorf("contract %d position = %#v, want %#v", index, contract.Position, want[index])
		}
	}
}

func TestSyntaxErrorIsLocated(t *testing.T) {
	_, diagnostics := ParseProject("syntax.toml", []byte("spec_version = [\n"))
	if len(diagnostics) != 1 || diagnostics[0].Path != "syntax.toml" || diagnostics[0].Line != 1 || diagnostics[0].Column < 1 {
		t.Fatalf("syntax diagnostics = %#v", diagnostics)
	}
}

func TestSpecVersionTwoAcceptsExactlyOneVersionSource(t *testing.T) {
	base := `spec_version = 2
[project]
id = "p"
VERSION
[sources]
files = ["src/a.md"]
[targets.one]
output_root = "out"
`
	for _, version := range []string{`version = "literal"`, `version_from = "VERSION"`} {
		project, diagnostics := ParseProject("gunte.toml", []byte(strings.Replace(base, "VERSION", version, 1)))
		if len(diagnostics) != 0 {
			t.Fatalf("%s diagnostics = %v", version, diagnostics)
		}
		if project.SpecVersion != 2 {
			t.Fatalf("spec version = %d", project.SpecVersion)
		}
	}
	for _, version := range []string{"", "version = \"literal\"\nversion_from = \"VERSION\""} {
		_, diagnostics := ParseProject("gunte.toml", []byte(strings.Replace(base, "VERSION", version, 1)))
		assertDiagnostic(t, diagnostics, "exactly one of project.version or project.version_from")
	}
}

func TestSpecVersionOneRejectsVersionFromAsUnknownAndStillRequiresLiteralVersion(t *testing.T) {
	input := []byte(`spec_version = 1
[project]
id = "p"
version_from = "VERSION"
[sources]
files = ["src/a.md"]
[targets.one]
output_root = "out"
`)
	_, diagnostics := ParseProject("gunte.toml", input)
	assertDiagnostic(t, diagnostics, "unknown key project.version_from")
	assertDiagnostic(t, diagnostics, "project.version is required")
}

func TestSpecVersionTwoParsesManagedScopesAndAllowsTargetRelativePaths(t *testing.T) {
	input := []byte(`spec_version = 2
[project]
id = "p"
version = "v"
[sources]
files = ["src/a.md"]
managed_roots = ["src"]
allow_files = ["src/keep"]
allow_dirs = ["src/generated"]
[targets.one]
output_root = "out"
managed_roots = ["artifacts"]
allow_files = ["artifacts/README"]
allow_dirs = ["artifacts/generated"]
`)
	project, diagnostics := ParseProject("gunte.toml", input)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %v", diagnostics)
	}
	if got := project.Sources.ManagedRoots; !reflect.DeepEqual(got, []string{"src"}) {
		t.Fatalf("source managed roots = %#v", got)
	}
	if got := project.Targets[0].AllowDirs; !reflect.DeepEqual(got, []string{"artifacts/generated"}) {
		t.Fatalf("target allow dirs = %#v", got)
	}
}

func TestSpecVersionOneRejectsManagedScopeKeys(t *testing.T) {
	input := []byte(`spec_version = 1
[project]
id = "p"
version = "v"
[sources]
files = ["src/a.md"]
managed_roots = ["src"]
[targets.one]
output_root = "out"
allow_dirs = ["generated"]
`)
	_, diagnostics := ParseProject("gunte.toml", input)
	assertDiagnostic(t, diagnostics, "unknown key sources.managed_roots")
	assertDiagnostic(t, diagnostics, "unknown key targets.one.allow_dirs")
}

func TestSpecVersionTwoRejectsOverlappingRootsAndRedundantAllows(t *testing.T) {
	input := []byte(`spec_version = 2
[project]
id = "p"
version = "v"
[sources]
files = ["src/a.md"]
managed_roots = ["src"]
allow_files = ["src/keep", "src/keep"]
allow_dirs = ["src/generated", "src/generated/nested"]
[targets.one]
output_root = "out"
managed_roots = ["artifacts"]
[targets.two]
output_root = "src"
managed_roots = ["x"]
`)
	_, diagnostics := ParseProject("gunte.toml", input)
	assertDiagnostic(t, diagnostics, "allow entries overlap")
	assertDiagnostic(t, diagnostics, "redundantly contained")
	assertDiagnostic(t, diagnostics, "managed root")
}

func TestSpecVersionTwoRejectsAllowOutsideExactlyOneManagedRoot(t *testing.T) {
	input := []byte(`spec_version = 2
[project]
id = "p"
version = "v"
[sources]
files = ["src/a.md"]
managed_roots = ["src/a", "src/b"]
allow_files = ["outside"]
[targets.one]
output_root = "out"
`)
	_, diagnostics := ParseProject("gunte.toml", input)
	assertDiagnostic(t, diagnostics, "allow file must be under exactly one managed root")
}

func TestSpecVersionTwoRejectsAllowEntryEqualToManagedRoot(t *testing.T) {
	input := []byte(`spec_version = 2
[project]
id = "p"
version = "v"
[sources]
files = ["src/a.md"]
managed_roots = ["src"]
allow_files = ["src"]
allow_dirs = ["src"]
[targets.one]
output_root = "out"
`)
	_, diagnostics := ParseProject("gunte.toml", input)
	assertDiagnostic(t, diagnostics, "allow file must be under exactly one managed root")
	assertDiagnostic(t, diagnostics, "allow directory must be under exactly one managed root")
}

func TestVersionFileNormalizationPreservesOpaqueSingleLineValue(t *testing.T) {
	for _, test := range []struct {
		name  string
		input []byte
		want  string
	}{
		{name: "no newline", input: []byte(" v2 "), want: " v2 "},
		{name: "one CRLF", input: []byte("\xef\xbb\xbf2.0\r\n"), want: "2.0"},
		{name: "one bare CR", input: []byte("2.0\r"), want: "2.0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, diagnostic := NormalizeVersionFile("VERSION", test.input)
			if diagnostic != nil || got != test.want {
				t.Fatalf("NormalizeVersionFile() = %q, %#v; want %q", got, diagnostic, test.want)
			}
		})
	}
}

func TestVersionFileNormalizationRejectsInvalidOrNonSingleLineValues(t *testing.T) {
	for _, input := range [][]byte{{0xff}, {}, []byte("\xef\xbb\xbf"), []byte("a\nb"), []byte("a\n\n"), []byte("\xef\xbb\xbf\xef\xbb\xbfv")} {
		if _, diagnostic := NormalizeVersionFile("VERSION", input); diagnostic == nil || diagnostic.Path != "VERSION" {
			t.Errorf("input %q diagnostic = %#v", input, diagnostic)
		}
	}
}

func TestSpecVersionTwoValidatesSlicedPredicateIDAtItsDeclaration(t *testing.T) {
	input := []byte(`[contracts.need-000000000000]
kind = "requires"
slice = "span"
pattern = "wanted"
applies_to = ["one"]
`)
	_, diagnostics := ParseContractsForSpec("contracts.toml", input, []string{"one"}, 2)
	diagnostic := findDiagnostic(t, diagnostics, "predicate ID must be need-1bb12accd185")
	if diagnostic.Line != 1 || diagnostic.Column != 1 {
		t.Fatalf("predicate diagnostic = %#v", diagnostic)
	}
}

func TestSpecVersionTwoAcceptsCanonicalSlicedPredicateID(t *testing.T) {
	input := []byte(`[contracts.need-1bb12accd185]
kind = "requires"
slice = "span"
pattern = "wanted"
applies_to = ["one"]
`)
	if _, diagnostics := ParseContractsForSpec("contracts.toml", input, []string{"one"}, 2); len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %v", diagnostics)
	}
}

func TestSpecVersionTwoSelectsContractFiles(t *testing.T) {
	base := `spec_version = 2
[project]
id = "p"
version = "v"
[sources]
files = ["src/a.md"]
[targets.one]
output_root = "out"
`
	project, diagnostics := ParseProject("gunte.toml", []byte(base))
	if len(diagnostics) != 0 || !reflect.DeepEqual(project.ContractFiles, []string{"contracts.toml"}) {
		t.Fatalf("default contracts = %#v, %v", project.ContractFiles, diagnostics)
	}
	explicit := strings.Replace(base, "[sources]", "[contracts]\nfiles = [\"rules/a.toml\", \"rules/b.toml\"]\n[sources]", 1)
	project, diagnostics = ParseProject("gunte.toml", []byte(explicit))
	if len(diagnostics) != 0 || !reflect.DeepEqual(project.ContractFiles, []string{"rules/a.toml", "rules/b.toml"}) {
		t.Fatalf("explicit contracts = %#v, %v", project.ContractFiles, diagnostics)
	}
}

func TestContractFileSelectionRejectsInvalidValuesAndRemainsV1Closed(t *testing.T) {
	base := `spec_version = 2
[project]
id = "p"
version = "v"
[contracts]
files = VALUE
[sources]
files = ["src/a.md"]
[targets.one]
output_root = "out"
`
	tests := []struct{ value, message string }{
		{`[]`, "contracts.files must contain at least one path"},
		{`[1]`, "contracts.files[0] must be a string"},
		{`["../a.toml"]`, "contracts.files[0] must not contain . or .. path segments"},
		{`["a.toml", "a.toml"]`, "duplicate contract path a.toml"},
	}
	for _, test := range tests {
		_, diagnostics := ParseProject("gunte.toml", []byte(strings.Replace(base, "VALUE", test.value, 1)))
		assertDiagnostic(t, diagnostics, test.message)
	}
	v1 := strings.Replace(strings.Replace(base, "spec_version = 2", "spec_version = 1", 1), "version = \"v\"", "version = \"v\"", 1)
	_, diagnostics := ParseProject("gunte.toml", []byte(strings.Replace(v1, "VALUE", `["a.toml"]`, 1)))
	assertDiagnostic(t, diagnostics, "unknown key contracts")
}

func TestParseContractDocumentsMergesInFileAndDeclarationOrder(t *testing.T) {
	documents := []ContractDocument{
		{Path: "a.toml", Bytes: []byte("[contracts.first]\nkind = \"forbids\"\npattern = \"a\"\napplies_to = [\"one\"]\n")},
		{Path: "b.toml", Bytes: []byte("[contracts.second]\nkind = \"forbids\"\npattern = \"b\"\napplies_to = [\"one\"]\n")},
	}
	registry, diagnostics := ParseContractDocuments(documents, []string{"one"}, 2)
	if len(diagnostics) != 0 || len(registry.Contracts) != 2 || registry.Contracts[0].ID != "first" || registry.Contracts[1].ID != "second" {
		t.Fatalf("registry = %#v, diagnostics = %#v", registry, diagnostics)
	}
}

func TestParseContractDocumentsReportsGlobalDuplicateAtLaterDeclaration(t *testing.T) {
	documents := []ContractDocument{
		{Path: "a.toml", Bytes: []byte("[contracts.same]\nkind = \"forbids\"\npattern = \"a\"\napplies_to = [\"one\"]\n")},
		{Path: "b.toml", Bytes: []byte("\n[contracts.same]\nkind = \"forbids\"\npattern = \"b\"\napplies_to = [\"one\"]\n")},
	}
	_, diagnostics := ParseContractDocuments(documents, []string{"one"}, 2)
	diagnostic := findDiagnostic(t, diagnostics, "duplicate predicate same")
	if diagnostic.Path != "b.toml" || diagnostic.Line != 2 || len(diagnostic.Related) != 1 || diagnostic.Related[0].Path != "a.toml" {
		t.Fatalf("duplicate diagnostic = %#v", diagnostic)
	}
}

func TestContractDeclarationsPreserveSupportedFormsAndPositions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		line  int
	}{
		{"header", `[contracts.header]
kind = "forbids"
pattern = "x"
applies_to = ["one"]
`, 1},
		{"quoted header", `[contracts."quoted"]
kind = "forbids"
pattern = "x"
applies_to = ["one"]
`, 1},
		{"inline", `contracts = { inline = { kind = "forbids", pattern = "x", applies_to = ["one"] } }
`, 1},
		{"dotted", `contracts.dotted.kind = "forbids"
contracts.dotted.pattern = "x"
contracts.dotted.applies_to = ["one"]
`, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry, diagnostics := ParseContractDocuments([]ContractDocument{{Path: test.name + ".toml", Bytes: []byte(test.input)}}, []string{"one"}, 2)
			if len(diagnostics) != 0 || len(registry.Contracts) != 1 || registry.Contracts[0].Position.Line != test.line {
				t.Fatalf("registry = %#v, diagnostics = %#v", registry, diagnostics)
			}
		})
	}
}

func TestTopLevelInlinePredicateAndFieldPositionsAreExact(t *testing.T) {
	input := []byte(`contracts = { first = { kind = "forbids", pattern = "x", applies_to = ["one"] }, "second" = { kind = "forbids", pattern = " invalid, # literal ", applies_to = ["one"] } }
`)
	registry, diagnostics := ParseContractDocuments([]ContractDocument{{Path: "inline.toml", Bytes: input}}, []string{"one"}, 2)
	if len(registry.Contracts) != 2 {
		t.Fatalf("registry = %#v, diagnostics = %#v", registry, diagnostics)
	}
	if registry.Contracts[0].Position.Column != strings.Index(string(input), "first")+1 || registry.Contracts[1].Position.Column != strings.Index(string(input), `"second"`)+1 {
		t.Fatalf("positions = %#v, %#v", registry.Contracts[0].Position, registry.Contracts[1].Position)
	}
	diagnostic := findDiagnostic(t, diagnostics, "pattern must not have leading or trailing whitespace")
	if diagnostic.Path != "inline.toml" || diagnostic.Line != 1 || diagnostic.Column != strings.LastIndex(string(input), "pattern")+1 {
		t.Fatalf("field diagnostic = %#v", diagnostic)
	}
}

func TestTopLevelInlineDuplicateUsesLaterPrimaryAndGlobalFirstRelated(t *testing.T) {
	input := []byte(`contracts = { same = { kind = "forbids", pattern = "a", applies_to = ["one"] }, same = { kind = "forbids", pattern = "b", applies_to = ["one"] } }
`)
	_, diagnostics := ParseContractDocuments([]ContractDocument{{Path: "inline.toml", Bytes: input}}, []string{"one"}, 2)
	diagnostic := findDiagnostic(t, diagnostics, "duplicate predicate same")
	if diagnostic.Path != "inline.toml" || diagnostic.Column != strings.LastIndex(string(input), "same")+1 || len(diagnostic.Related) != 1 || diagnostic.Related[0].Column != strings.Index(string(input), "same")+1 {
		t.Fatalf("duplicate diagnostic = %#v", diagnostic)
	}
}

func TestSecondStructureAssertionFieldDiagnosticUsesItsOwnPosition(t *testing.T) {
	input := []byte(`[contracts.first]
kind = "structure"
subject = "source_frontmatter"
paths = ["src/a.md"]
[[contracts.first.assertions]]
path = "name"
op = "exists"

[contracts.second]
kind = "structure"
subject = "source_frontmatter"
paths = ["src/b.md"]
[[contracts.second.assertions]]
path = "name"
op = "invalid"
`)
	_, diagnostics := ParseContractDocuments([]ContractDocument{{Path: "structure.toml", Bytes: input}}, []string{"one"}, 2)
	diagnostic := findDiagnostic(t, diagnostics, "unknown assertion op")
	if diagnostic.Line != 15 || diagnostic.Column != 1 {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestInlineDottedPredicateRemainsInRegistry(t *testing.T) {
	input := []byte(`contracts = { inline.kind = "forbids", inline.pattern = "x", inline.applies_to = ["one"] }
`)
	registry, diagnostics := ParseContractDocuments([]ContractDocument{{Path: "inline-dotted.toml", Bytes: input}}, []string{"one"}, 2)
	if len(diagnostics) != 0 || len(registry.Contracts) != 1 || registry.Contracts[0].ID != "inline" {
		t.Fatalf("registry = %#v, diagnostics = %#v", registry, diagnostics)
	}
	if registry.Contracts[0].Position.Line != 1 || registry.Contracts[0].Position.Column != strings.Index(string(input), "inline.kind")+1 {
		t.Fatalf("position = %#v", registry.Contracts[0].Position)
	}
}

func TestInlineDottedFieldDiagnosticUsesItsPredicateKeyPosition(t *testing.T) {
	input := []byte(`contracts = { first.kind = "forbids", first.pattern = "x", first.applies_to = ["one"], second.kind = "forbids", second.pattern = " invalid ", second.applies_to = ["one"] }
`)
	registry, diagnostics := ParseContractDocuments([]ContractDocument{{Path: "inline-dotted.toml", Bytes: input}}, []string{"one"}, 2)
	if len(registry.Contracts) != 2 {
		t.Fatalf("registry = %#v, diagnostics = %#v", registry, diagnostics)
	}
	diagnostic := findDiagnostic(t, diagnostics, "pattern must not have leading or trailing whitespace")
	if diagnostic.Path != "inline-dotted.toml" || diagnostic.Line != 1 || diagnostic.Column != strings.Index(string(input), "second.pattern")+len("second.")+1 {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestLaterInlineArrayAssertionFieldUsesItsOwnExactPosition(t *testing.T) {
	input := []byte(`[contracts.only]
kind = "structure"
subject = "source_frontmatter"
paths = ["src/a.md"]
assertions = [
  { path = "name", op = "exists" },
  { path = "name", op = "invalid" },
]
`)
	_, diagnostics := ParseContractDocuments([]ContractDocument{{Path: "assertions.toml", Bytes: input}}, []string{"one"}, 2)
	diagnostic := findDiagnostic(t, diagnostics, "unknown assertion op")
	if diagnostic.Path != "assertions.toml" || diagnostic.Line != 7 || diagnostic.Column != 20 {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestTopLevelInlinePredicateLaterAssertionFieldUsesItsOwnExactPosition(t *testing.T) {
	input := []byte(`contracts = { only = { kind = "structure", subject = "source_frontmatter", paths = ["src/a.md"], assertions = [{ path = "name", op = "exists" }, { path = "name", op = "invalid" }] } }
`)
	_, diagnostics := ParseContractDocuments([]ContractDocument{{Path: "inline-assertions.toml", Bytes: input}}, []string{"one"}, 2)
	diagnostic := findDiagnostic(t, diagnostics, "unknown assertion op")
	wantColumn := strings.LastIndex(string(input), "op = \"invalid\"") + 1
	if diagnostic.Path != "inline-assertions.toml" || diagnostic.Line != 1 || diagnostic.Column != wantColumn {
		t.Fatalf("diagnostic = %#v, want column %d", diagnostic, wantColumn)
	}
}

func TestDiagnosticsPointToTheirOwnFields(t *testing.T) {
	tests := []struct {
		name, second, field, message string
	}{
		{"path", `{ path = ".", op = "exists" }`, "path", "assertion path must"},
		{"value", `{ path = "name", op = "exists", value = "x" }`, "value", "value is not allowed for exists"},
		{"count", `{ path = "name", op = "cardinality", count = -1 }`, "count", "count must be a non-negative integer"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := []byte(`contracts = { only = { kind = "structure", subject = "source_frontmatter", paths = ["src/a.md"], assertions = [{ path = "name", op = "exists" }, ` + test.second + `] } }
`)
			_, diagnostics := ParseContractDocuments([]ContractDocument{{Path: "inline-assertions.toml", Bytes: input}}, []string{"one"}, 2)
			diagnostic := findDiagnostic(t, diagnostics, test.message)
			wantColumn := strings.LastIndex(string(input), test.field+" =") + 1
			if diagnostic.Line != 1 || diagnostic.Column != wantColumn {
				t.Fatalf("diagnostic = %#v, want column %d", diagnostic, wantColumn)
			}
		})
	}
}

func TestCrossFileDuplicatesAlwaysRelateToGlobalFirstInlinePredicate(t *testing.T) {
	documents := []ContractDocument{
		{Path: "a.toml", Bytes: []byte(`contracts = { same = { kind = "forbids", pattern = "a", applies_to = ["one"] } }`)},
		{Path: "b.toml", Bytes: []byte(`contracts = { same = { kind = "forbids", pattern = "b", applies_to = ["one"] }, same = { kind = "forbids", pattern = "c", applies_to = ["one"] } }`)},
	}
	_, diagnostics := ParseContractDocuments(documents, []string{"one"}, 2)
	duplicates := make([]Diagnostic, 0)
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Message, "duplicate predicate same") {
			duplicates = append(duplicates, diagnostic)
		}
	}
	if len(duplicates) != 2 {
		t.Fatalf("duplicates = %#v; all diagnostics = %#v", duplicates, diagnostics)
	}
	for _, diagnostic := range duplicates {
		if diagnostic.Path != "b.toml" || len(diagnostic.Related) != 1 || diagnostic.Related[0].Path != "a.toml" || diagnostic.Related[0].Column != 15 {
			t.Fatalf("duplicate = %#v", diagnostic)
		}
	}
}

func TestDottedImplicitThenFirstExplicitIsOnePredicateButSecondExplicitIsDuplicate(t *testing.T) {
	input := []byte(`contracts.same.kind = "forbids"
contracts.same.pattern = "x"
contracts.same.applies_to = ["one"]
[contracts.same]
[contracts.same]
`)
	_, diagnostics := ParseContractDocuments([]ContractDocument{{Path: "same.toml", Bytes: input}}, []string{"one"}, 2)
	duplicate := findDiagnostic(t, diagnostics, "duplicate predicate same")
	if duplicate.Line != 5 || duplicate.Related[0].Line != 1 {
		t.Fatalf("duplicate = %#v", duplicate)
	}
}

func TestCommentsAndMultilineStringsDoNotCreatePredicates(t *testing.T) {
	input := []byte(`[contracts.real]
kind = "forbids"
pattern = """\
[contracts.fake]\
"""
# [contracts.also_fake]
applies_to = ["one"]
`)
	registry, diagnostics := ParseContractDocuments([]ContractDocument{{Path: "decoy.toml", Bytes: input}}, []string{"one"}, 2)
	if len(diagnostics) != 0 || len(registry.Contracts) != 1 || registry.Contracts[0].ID != "real" || registry.Contracts[0].Position.Line != 1 {
		t.Fatalf("registry = %#v, diagnostics = %#v", registry, diagnostics)
	}
}

func TestContractFieldDiagnosticIsScopedToItsPredicate(t *testing.T) {
	input := []byte(`[contracts.first]
kind = "forbids"
pattern = "valid"
applies_to = ["one"]

[contracts.second]
kind = "forbids"
pattern = " invalid "
applies_to = ["one"]
`)
	_, diagnostics := ParseContractDocuments([]ContractDocument{{Path: "fields.toml", Bytes: input}}, []string{"one"}, 2)
	diagnostic := findDiagnostic(t, diagnostics, "pattern must not have leading or trailing whitespace")
	if diagnostic.Line != 8 || diagnostic.Column != 1 {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func assertDiagnostic(t *testing.T, diagnostics []Diagnostic, message string) {
	t.Helper()
	findDiagnostic(t, diagnostics, message)
}

func findDiagnostic(t *testing.T, diagnostics []Diagnostic, message string) Diagnostic {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Message, message) {
			return diagnostic
		}
	}
	t.Fatalf("diagnostics %v do not contain %q", diagnostics, message)
	return Diagnostic{}
}
