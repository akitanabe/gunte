package adapter

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/akitanabe/gunte/internal/compile"
	"github.com/akitanabe/gunte/internal/config"
	"github.com/akitanabe/gunte/internal/source"
)

func TestAdaptResolvesCapturesMetadataAndPreservesProjection(t *testing.T) {
	project := config.ProjectConfig{
		Project: config.Project{Version: "1.2.3"},
		Sources: config.Sources{Files: []string{"shared/skill/demo/references/guide.md"}},
		Targets: []config.Target{{
			ID:         "claude",
			OutputRoot: "plugins/claude",
			Rules: []config.Rule{{
				Match:     "shared/skill/*/references/*.md",
				Path:      "skills/{1}/references/{2}.md",
				Profile:   config.ProfileYAML,
				Header:    "Generated",
				BodyField: "body",
				Metadata: []config.MetadataEntry{{
					Field: "name", From: "core:name", Type: config.MetadataString,
				}, {
					Field: "tools", From: "frontmatter:claude.tools", Type: config.MetadataCommaList,
				}, {
					Field: "version", From: "project:version", Type: config.MetadataString,
				}},
			}},
		}},
	}
	body := []byte("projected\n")
	sources := []Source{{
		Projection: compile.SourceProjection{
			Path:      "shared/skill/demo/references/guide.md",
			Bytes:     body,
			Contracts: []compile.ProjectedDeclaration{{ID: "contract", Source: compile.SourcePosition{Path: "shared/skill/demo/references/guide.md", Offset: 1}, Emitted: true, ProjectedRange: source.Range{Start: 1, End: 4}}},
			Anchors:   []compile.ProjectedDeclaration{{ID: "anchor", Source: compile.SourcePosition{Path: "shared/skill/demo/references/guide.md", Offset: 2}, Emitted: true, ProjectedRange: source.Range{Start: 2, End: 2}}},
		},
		Frontmatter: map[string]any{"claude": map[string]any{"tools": "Read, Write"}},
	}}
	originalBody := append([]byte(nil), body...)
	originalFrontmatter := cloneMap(sources[0].Frontmatter)
	originalProjection := cloneProjection(sources[0].Projection)

	result, diagnostics := Adapt(project, sources)
	if len(diagnostics) != 0 {
		t.Fatalf("Adapt() diagnostics = %#v", diagnostics)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("Adapt() artifacts = %#v, want one artifact", result.Artifacts)
	}
	artifact := result.Artifacts[0]
	if artifact.Path != "plugins/claude/skills/demo/references/guide.md" {
		t.Fatalf("artifact path = %q", artifact.Path)
	}
	if artifact.TargetID != "claude" || artifact.SourcePath != sources[0].Projection.Path || artifact.Profile != config.ProfileYAML {
		t.Fatalf("artifact identity = %#v", artifact)
	}
	if artifact.BodyField != "body" || !reflect.DeepEqual(artifact.Contracts, sources[0].Projection.Contracts) || !reflect.DeepEqual(artifact.Anchors, sources[0].Projection.Anchors) {
		t.Fatalf("artifact provenance = %#v", artifact)
	}
	if !bytes.Equal(artifact.Body, body) {
		t.Fatalf("artifact body = %q, want %q", artifact.Body, body)
	}
	if got := artifact.Metadata[0].Value.String; got != "guide" {
		t.Fatalf("core:name = %q, want guide", got)
	}
	if got := artifact.Metadata[1].Value.Strings; !reflect.DeepEqual(got, []string{"Read", "Write"}) {
		t.Fatalf("comma_list = %#v", got)
	}
	if got := artifact.Metadata[2].Value.String; got != "1.2.3" {
		t.Fatalf("project:version = %q", got)
	}
	artifact.Body[0] = 'X'
	artifact.Contracts[0].ID = "mutated"
	artifact.Anchors[0].Source.Offset = 99
	if !bytes.Equal(sources[0].Projection.Bytes, originalBody) || !reflect.DeepEqual(sources[0].Frontmatter, originalFrontmatter) || !reflect.DeepEqual(sources[0].Projection, originalProjection) {
		t.Fatal("Adapt() mutated source input")
	}
}

func TestAdaptMatchesLiteralAndWildcardRulesAcrossTargets(t *testing.T) {
	project := config.ProjectConfig{
		Sources: config.Sources{Files: []string{"a+b/file.md", "a/b/file.md"}},
		Targets: []config.Target{{ID: "one", OutputRoot: "out/one", Rules: []config.Rule{{Match: "a+b/*.md", Path: "literal/{1}.md", Profile: config.ProfileMarkdown}}}, {
			ID: "two", OutputRoot: "out/two", Rules: []config.Rule{{Match: "a/*/*.md", Path: "nested/{1}-{2}.md", Profile: config.ProfileMarkdown}},
		}},
	}
	sources := []Source{
		{Projection: compile.SourceProjection{Path: "a+b/file.md", Bytes: []byte("x")}},
		{Projection: compile.SourceProjection{Path: "a/b/file.md", Bytes: []byte("y")}},
	}
	result, diagnostics := Adapt(project, sources)
	if len(diagnostics) != 0 {
		t.Fatalf("Adapt() diagnostics = %#v", diagnostics)
	}
	if got := []string{result.Artifacts[0].Path, result.Artifacts[1].Path}; !reflect.DeepEqual(got, []string{"out/one/literal/file.md", "out/two/nested/b-file.md"}) {
		t.Fatalf("artifact paths = %#v", got)
	}
}

func TestAdaptRejectsAmbiguousMatchAndInvalidTemplates(t *testing.T) {
	project := config.ProjectConfig{Targets: []config.Target{{ID: "target", OutputRoot: "out", Rules: []config.Rule{{Match: "*.md", Path: "a/{1}.md", Profile: config.ProfileMarkdown}, {Match: "name.md", Path: "b.md", Profile: config.ProfileMarkdown}}}}}
	_, diagnostics := Adapt(project, []Source{{Projection: compile.SourceProjection{Path: "name.md", Bytes: []byte("x")}}})
	if !hasCode(diagnostics, "multiple_rule_match") {
		t.Fatalf("ambiguous match diagnostics = %#v", diagnostics)
	}

	for _, path := range []string{"a/{2}.md", "a/{0}.md", "a/{x}.md", "a/{.md"} {
		project.Targets[0].Rules = []config.Rule{{Match: "*.md", Path: path, Profile: config.ProfileMarkdown}}
		_, diagnostics = Adapt(project, []Source{{Projection: compile.SourceProjection{Path: "name.md", Bytes: []byte("x")}}})
		if !hasCode(diagnostics, "invalid_path_template") {
			t.Fatalf("path %q diagnostics = %#v", path, diagnostics)
		}
	}
}

func TestAdaptExpandsNinthCaptureRepeatedlyAndRejectsTenth(t *testing.T) {
	project := config.ProjectConfig{Targets: []config.Target{{ID: "target", OutputRoot: "out", Rules: []config.Rule{{Match: "a/*/*/*/*/*/*/*/*/*", Path: "captures/{9}-{9}.md", Profile: config.ProfileMarkdown}}}}}
	result, diagnostics := Adapt(project, []Source{{Projection: compile.SourceProjection{Path: "a/one/two/three/four/five/six/seven/eight/nine", Bytes: []byte("body")}}})
	if len(diagnostics) != 0 || len(result.Artifacts) != 1 || result.Artifacts[0].Path != "out/captures/nine-nine.md" {
		t.Fatalf("ninth capture result=%#v diagnostics=%#v", result, diagnostics)
	}

	project.Targets[0].Rules[0].Path = "captures/{10}.md"
	result, diagnostics = Adapt(project, []Source{{Projection: compile.SourceProjection{Path: "a/one/two/three/four/five/six/seven/eight/nine", Bytes: []byte("body")}}})
	if len(result.Artifacts) != 0 || !hasCode(diagnostics, "invalid_path_template") {
		t.Fatalf("tenth capture result=%#v diagnostics=%#v", result, diagnostics)
	}
}

func TestAdaptTreatsDoubleStarAsTwoNonRecursiveWildcards(t *testing.T) {
	project := config.ProjectConfig{Targets: []config.Target{{ID: "target", OutputRoot: "out", Rules: []config.Rule{{Match: "a/**/file.md", Path: "files/{1}-{2}.md", Profile: config.ProfileMarkdown}}}}}
	result, diagnostics := Adapt(project, []Source{{Projection: compile.SourceProjection{Path: "a/one/file.md", Bytes: []byte("one")}}})
	if len(diagnostics) != 0 || len(result.Artifacts) != 1 || result.Artifacts[0].Path != "out/files/one-.md" {
		t.Fatalf("double-star single-level result=%#v diagnostics=%#v", result, diagnostics)
	}
	result, diagnostics = Adapt(project, []Source{{Projection: compile.SourceProjection{Path: "a/one/two/file.md", Bytes: []byte("two")}}})
	if len(result.Artifacts) != 0 || !hasCode(diagnostics, "unmatched_source") {
		t.Fatalf("double-star recursive result=%#v diagnostics=%#v", result, diagnostics)
	}
}

func TestAdaptReportsUnmatchedWarningAndCollisionsDeterministically(t *testing.T) {
	project := config.ProjectConfig{Sources: config.Sources{Files: []string{"unmatched.md", "one.md", "two.md"}}, Targets: []config.Target{{ID: "target", OutputRoot: "out", Rules: []config.Rule{{Match: "one.md", Path: "same.md", Profile: config.ProfileMarkdown}, {Match: "two.md", Path: "same.md", Profile: config.ProfileMarkdown}}}}}
	result, diagnostics := Adapt(project, []Source{{Projection: compile.SourceProjection{Path: "unmatched.md"}}, {Projection: compile.SourceProjection{Path: "one.md"}}, {Projection: compile.SourceProjection{Path: "two.md"}}})
	if len(result.Artifacts) != 1 {
		t.Fatalf("artifacts = %#v", result.Artifacts)
	}
	if !hasCode(diagnostics, "unmatched_source") || !hasCode(diagnostics, "path_collision") {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if diagnostics[0].Code != "unmatched_source" || diagnostics[1].Code != "path_collision" {
		t.Fatalf("diagnostic order = %#v", diagnostics)
	}
}

func TestAdaptDoesNotReservePathWhenMetadataMappingFails(t *testing.T) {
	project := config.ProjectConfig{Targets: []config.Target{{ID: "target", OutputRoot: "out", Rules: []config.Rule{{Match: "first.md", Path: "same.md", Profile: config.ProfileYAML, Metadata: []config.MetadataEntry{{Field: "required", From: "frontmatter:missing", Type: config.MetadataString, Required: true}}}, {Match: "second.md", Path: "same.md", Profile: config.ProfileYAML}}}}}
	result, diagnostics := Adapt(project, []Source{{Projection: compile.SourceProjection{Path: "first.md", Bytes: []byte("first")}}, {Projection: compile.SourceProjection{Path: "second.md", Bytes: []byte("second")}}})
	if len(diagnostics) != 1 || diagnostics[0].Code != "metadata_missing" {
		t.Fatalf("diagnostics = %#v, want only first metadata error", diagnostics)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].SourcePath != "second.md" || !bytes.Equal(result.Artifacts[0].Body, []byte("second")) {
		t.Fatalf("result = %#v, want valid second artifact", result)
	}
}

func TestAdaptResolvesMetadataTypesAndRequiredRules(t *testing.T) {
	base := config.Rule{Match: "source.md", Path: "source.md", Profile: config.ProfileYAML, Metadata: []config.MetadataEntry{{Field: "string", From: "frontmatter:string", Type: config.MetadataString, Required: true}, {Field: "list", From: "frontmatter:list", Type: config.MetadataStringList, Required: true}, {Field: "comma", From: "frontmatter:comma", Type: config.MetadataCommaList, Required: true}, {Field: "token", From: "literal:token", Type: config.MetadataPlainToken, Required: true}, {Field: "optional", From: "frontmatter:missing", Type: config.MetadataString, Required: false}}}
	project := config.ProjectConfig{Targets: []config.Target{{ID: "target", OutputRoot: "out", Rules: []config.Rule{base}}}}
	result, diagnostics := Adapt(project, []Source{{Projection: compile.SourceProjection{Path: "source.md", Bytes: []byte("body")}, Frontmatter: map[string]any{"string": "value", "list": []any{"a", "b"}, "comma": "A, B"}}})
	if len(diagnostics) != 0 || len(result.Artifacts) != 1 {
		t.Fatalf("result=%#v diagnostics=%#v", result, diagnostics)
	}
	if got := len(result.Artifacts[0].Metadata); got != 4 {
		t.Fatalf("metadata count = %d, want optional omission", got)
	}

	for _, entry := range []config.MetadataEntry{{Field: "value", From: "frontmatter:missing", Type: config.MetadataString, Required: true}, {Field: "value", From: "frontmatter:missing", Type: config.MetadataStringList, Required: true}, {Field: "value", From: "frontmatter:missing", Type: config.MetadataCommaList, Required: true}, {Field: "value", From: "frontmatter:missing", Type: config.MetadataPlainToken, Required: true}} {
		project.Targets[0].Rules[0].Metadata = []config.MetadataEntry{entry}
		_, diagnostics = Adapt(project, []Source{{Projection: compile.SourceProjection{Path: "source.md", Bytes: []byte("body")}}})
		if !hasCode(diagnostics, "metadata_missing") {
			t.Fatalf("entry %#v diagnostics = %#v", entry, diagnostics)
		}
	}
}

func TestAdaptReturnsExplicitTypedMetadataValuesIncludingEmptyStringList(t *testing.T) {
	project := config.ProjectConfig{Targets: []config.Target{{ID: "target", OutputRoot: "out", Rules: []config.Rule{{Match: "source.md", Path: "source.md", Profile: config.ProfileYAML, Metadata: []config.MetadataEntry{{Field: "string", From: "frontmatter:string", Type: config.MetadataString, Required: true}, {Field: "string_list", From: "frontmatter:string_list", Type: config.MetadataStringList, Required: true}, {Field: "comma_list", From: "frontmatter:comma_list", Type: config.MetadataCommaList, Required: true}, {Field: "plain_token", From: "frontmatter:plain_token", Type: config.MetadataPlainToken, Required: true}}}}}}}
	sourceInput := Source{Projection: compile.SourceProjection{Path: "source.md", Bytes: []byte("body")}, Frontmatter: map[string]any{"string": "value", "string_list": []any{}, "comma_list": "Read, Write", "plain_token": "tool"}}
	result, diagnostics := Adapt(project, []Source{sourceInput})
	if len(diagnostics) != 0 || len(result.Artifacts) != 1 {
		t.Fatalf("result=%#v diagnostics=%#v", result, diagnostics)
	}
	metadata := result.Artifacts[0].Metadata
	if len(metadata) != 4 {
		t.Fatalf("metadata = %#v", metadata)
	}
	if metadata[0].Value.Type != config.MetadataString || metadata[0].Value.String != "value" {
		t.Fatalf("string metadata = %#v", metadata[0])
	}
	if metadata[1].Value.Type != config.MetadataStringList || !reflect.DeepEqual(metadata[1].Value.Strings, []string{}) {
		t.Fatalf("empty string_list metadata = %#v", metadata[1])
	}
	if metadata[2].Value.Type != config.MetadataCommaList || !reflect.DeepEqual(metadata[2].Value.Strings, []string{"Read", "Write"}) {
		t.Fatalf("comma_list metadata = %#v", metadata[2])
	}
	if metadata[3].Value.Type != config.MetadataPlainToken || metadata[3].Value.String != "tool" {
		t.Fatalf("plain_token metadata = %#v", metadata[3])
	}
}

func TestAdaptRejectsPresentOptionalEmptyOrWrongMetadataAndOmitsArtifact(t *testing.T) {
	tests := []struct {
		name  string
		entry config.MetadataEntry
		raw   any
	}{
		{name: "optional string empty", entry: config.MetadataEntry{Field: "field", From: "frontmatter:value", Type: config.MetadataString, Required: false}, raw: ""},
		{name: "optional string wrong type", entry: config.MetadataEntry{Field: "field", From: "frontmatter:value", Type: config.MetadataString, Required: false}, raw: 1},
		{name: "optional string list wrong type", entry: config.MetadataEntry{Field: "field", From: "frontmatter:value", Type: config.MetadataStringList, Required: false}, raw: "one"},
		{name: "optional comma list wrong type", entry: config.MetadataEntry{Field: "field", From: "frontmatter:value", Type: config.MetadataCommaList, Required: false}, raw: 1},
		{name: "optional plain token wrong type", entry: config.MetadataEntry{Field: "field", From: "frontmatter:value", Type: config.MetadataPlainToken, Required: false}, raw: []any{"token"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project := config.ProjectConfig{Targets: []config.Target{{ID: "target", OutputRoot: "out", Rules: []config.Rule{{Match: "source.md", Path: "source.md", Profile: config.ProfileYAML, Metadata: []config.MetadataEntry{test.entry}}}}}}
			result, diagnostics := Adapt(project, []Source{{Projection: compile.SourceProjection{Path: "source.md", Bytes: []byte("body")}, Frontmatter: map[string]any{"value": test.raw}}})
			if len(result.Artifacts) != 0 || !hasCode(diagnostics, "metadata_invalid") {
				t.Fatalf("result=%#v diagnostics=%#v", result, diagnostics)
			}
		})
	}
}

func TestAdaptResolvesPlainTextValueFromAndRejectsTypeOrEmptyValues(t *testing.T) {
	project := config.ProjectConfig{Project: config.Project{Version: "2.0.0"}, Targets: []config.Target{{ID: "target", OutputRoot: "out", Rules: []config.Rule{{Match: "VERSION", Path: "VERSION", Profile: config.ProfilePlainText, ValueFrom: "project:version"}}}}}
	result, diagnostics := Adapt(project, []Source{{Projection: compile.SourceProjection{Path: "VERSION", Bytes: []byte("ignored")}}})
	if len(diagnostics) != 0 || result.Artifacts[0].Value == nil || result.Artifacts[0].Value.String != "2.0.0" {
		t.Fatalf("plain text result=%#v diagnostics=%#v", result, diagnostics)
	}

	for _, raw := range []any{"", 42} {
		project.Targets[0].Rules[0] = config.Rule{Match: "VERSION", Path: "VERSION", Profile: config.ProfilePlainText, ValueFrom: "frontmatter:value"}
		_, diagnostics = Adapt(project, []Source{{Projection: compile.SourceProjection{Path: "VERSION", Bytes: []byte("ignored")}, Frontmatter: map[string]any{"value": raw}}})
		if !hasCode(diagnostics, "value_from_invalid") {
			t.Fatalf("value %#v diagnostics = %#v", raw, diagnostics)
		}
	}
}

func TestAdaptUsesAnchoredCaseSensitiveWildcardWithEmptyCapture(t *testing.T) {
	project := config.ProjectConfig{Targets: []config.Target{{ID: "target", OutputRoot: "out", Rules: []config.Rule{{Match: "prefix/*/suffix", Path: "{1}.md", Profile: config.ProfileMarkdown}}}}}
	for _, test := range []struct {
		path string
		want string
	}{
		{path: "prefix//suffix", want: "out/.md"},
		{path: "prefix/value/suffix/extra", want: ""},
		{path: "Prefix/value/suffix", want: ""},
		{path: "prefix/a/b/suffix", want: ""},
	} {
		result, diagnostics := Adapt(project, []Source{{Projection: compile.SourceProjection{Path: test.path, Bytes: []byte("body")}}})
		if test.want == "" {
			if len(result.Artifacts) != 0 {
				t.Fatalf("path %q artifacts = %#v, want no match", test.path, result.Artifacts)
			}
			continue
		}
		if len(diagnostics) != 0 || len(result.Artifacts) != 1 || result.Artifacts[0].Path != test.want {
			t.Fatalf("path %q result=%#v diagnostics=%#v", test.path, result, diagnostics)
		}
	}
}

func TestAdaptTreatsLeadingDotAsNameAndReportsNestedFrontmatterMissing(t *testing.T) {
	project := config.ProjectConfig{Targets: []config.Target{{ID: "target", OutputRoot: "out", Rules: []config.Rule{{Match: "*.md", Path: "{1}.out", Profile: config.ProfileYAML, Metadata: []config.MetadataEntry{{Field: "name", From: "core:name", Type: config.MetadataString, Required: true}, {Field: "nested", From: "frontmatter:outer.inner.value", Type: config.MetadataString, Required: true}}}}}}}
	result, diagnostics := Adapt(project, []Source{{Projection: compile.SourceProjection{Path: ".profile.md", Bytes: []byte("body")}, Frontmatter: map[string]any{"outer": "not a table"}}})
	if len(result.Artifacts) != 0 || !hasCode(diagnostics, "metadata_missing") {
		t.Fatalf("nested missing result=%#v diagnostics=%#v", result, diagnostics)
	}

	project.Targets[0].Rules[0].Metadata[1].Required = false
	result, diagnostics = Adapt(project, []Source{{Projection: compile.SourceProjection{Path: ".profile.md", Bytes: []byte("body")}, Frontmatter: map[string]any{"outer": "not a table"}}})
	if len(diagnostics) != 0 || len(result.Artifacts) != 1 || result.Artifacts[0].Metadata[0].Value.String != ".profile" {
		t.Fatalf("leading dot result=%#v diagnostics=%#v", result, diagnostics)
	}
}

func TestAdaptRejectsMetadataTypeAndEmptyValues(t *testing.T) {
	tests := []struct {
		name         string
		raw          any
		metadataType config.MetadataType
		code         string
	}{
		{name: "string type mismatch", raw: 1, metadataType: config.MetadataString, code: "metadata_invalid"},
		{name: "string empty", raw: "", metadataType: config.MetadataString, code: "metadata_invalid"},
		{name: "string list type mismatch", raw: "one", metadataType: config.MetadataStringList, code: "metadata_invalid"},
		{name: "comma list empty", raw: "  ", metadataType: config.MetadataCommaList, code: "metadata_invalid"},
		{name: "plain token empty", raw: "", metadataType: config.MetadataPlainToken, code: "metadata_invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project := config.ProjectConfig{Targets: []config.Target{{ID: "target", OutputRoot: "out", Rules: []config.Rule{{Match: "source.md", Path: "source.md", Profile: config.ProfileYAML, Metadata: []config.MetadataEntry{{Field: "field", From: "frontmatter:value", Type: test.metadataType, Required: true}}}}}}}
			_, diagnostics := Adapt(project, []Source{{Projection: compile.SourceProjection{Path: "source.md", Bytes: []byte("body")}, Frontmatter: map[string]any{"value": test.raw}}})
			if !hasCode(diagnostics, test.code) {
				t.Fatalf("raw=%#v diagnostics=%#v", test.raw, diagnostics)
			}
		})
	}
}

func TestAdaptDetectsSemanticInputCollision(t *testing.T) {
	project := config.ProjectConfig{Sources: config.Sources{Files: []string{"out/gunte.toml"}}, Targets: []config.Target{{ID: "target", OutputRoot: "out", Rules: []config.Rule{{Match: "source.md", Path: "gunte.toml", Profile: config.ProfileMarkdown}}}}}
	_, diagnostics := Adapt(project, []Source{{Projection: compile.SourceProjection{Path: "source.md", Bytes: []byte("body")}}})
	if !hasCode(diagnostics, "path_collision") {
		t.Fatalf("semantic collision diagnostics = %#v", diagnostics)
	}
}

func TestAdaptDoesNotMutateCompleteProjectOrSourceInput(t *testing.T) {
	project := config.ProjectConfig{
		SpecVersion: 1,
		Project:     config.Project{ID: "project", Version: "1.0.0"},
		Sources:     config.Sources{Files: []string{"source.md"}},
		Terms:       []config.Term{{Name: "term", Values: []config.TargetValue{{TargetID: "target", Value: "value"}}}},
		Targets:     []config.Target{{ID: "target", OutputRoot: "out", Rules: []config.Rule{{Match: "source.md", Path: "source.md", Profile: config.ProfileYAML, Metadata: []config.MetadataEntry{{Field: "field", From: "literal:value", Type: config.MetadataString, Required: true}}}}}},
	}
	sources := []Source{{Projection: compile.SourceProjection{Path: "source.md", Bytes: []byte("body")}, Frontmatter: map[string]any{"nested": map[string]any{"value": "original"}}}}
	originalProject := cloneProject(project)
	originalSources := cloneSources(sources)
	Adapt(project, sources)
	if !reflect.DeepEqual(project, originalProject) {
		t.Fatalf("project mutated: got %#v want %#v", project, originalProject)
	}
	if !reflect.DeepEqual(sources, originalSources) {
		t.Fatalf("sources mutated: got %#v want %#v", sources, originalSources)
	}
}

func TestAdaptResolvesEveryOracleRuleToExpectedArtifactPath(t *testing.T) {
	projectBytes, err := os.ReadFile(filepath.Join("..", "..", "testdata", "oracle", "input", "gunte.toml"))
	if err != nil {
		t.Fatalf("oracle fixture unavailable: %v", err)
	}
	project, diagnostics := config.ParseProject("gunte.toml", projectBytes)
	if len(diagnostics) != 0 {
		t.Fatalf("ParseProject() diagnostics = %#v", diagnostics)
	}
	root := filepath.Join("..", "..", "testdata", "oracle", "input")
	sources := make([]Source, 0, len(project.Sources.Files))
	for _, relative := range project.Sources.Files {
		input, readErr := os.ReadFile(filepath.Join(root, relative))
		if readErr != nil {
			t.Fatalf("read %s: %v", relative, readErr)
		}
		document, sourceDiagnostics := source.Parse(relative, input)
		if len(sourceDiagnostics) != 0 {
			t.Fatalf("Parse(%s) diagnostics = %#v", relative, sourceDiagnostics)
		}
		sources = append(sources, Source{Projection: compile.SourceProjection{Path: relative, Bytes: append([]byte(nil), document.Buffer[document.BodyRange.Start:document.BodyRange.End]...)}, Frontmatter: document.FrontmatterData})
	}
	result, adapterDiagnostics := Adapt(project, sources)
	if len(adapterDiagnostics) != 0 {
		t.Fatalf("Adapt() diagnostics = %#v", adapterDiagnostics)
	}
	wantPaths := make([]string, 0)
	err = filepath.Walk(filepath.Join("..", "..", "testdata", "oracle", "golden"), func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info == nil || info.IsDir() {
			return nil
		}
		relative, relativeErr := filepath.Rel(filepath.Join("..", "..", "testdata", "oracle", "golden"), path)
		if relativeErr != nil {
			return relativeErr
		}
		wantPaths = append(wantPaths, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatalf("walk golden: %v", err)
	}
	sort.Strings(wantPaths)
	if len(wantPaths) != 77 {
		t.Fatalf("oracle golden artifact count = %d, want 77", len(wantPaths))
	}
	gotPaths := make([]string, len(result.Artifacts))
	for index, artifact := range result.Artifacts {
		gotPaths[index] = artifact.Path
	}
	sort.Strings(gotPaths)
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("oracle artifact paths = %#v, want %#v", gotPaths, wantPaths)
	}
}

func cloneProject(project config.ProjectConfig) config.ProjectConfig {
	clone := project
	clone.Sources.Files = append([]string(nil), project.Sources.Files...)
	clone.Terms = append([]config.Term(nil), project.Terms...)
	for index, term := range clone.Terms {
		clone.Terms[index].Values = append([]config.TargetValue(nil), term.Values...)
	}
	clone.Targets = append([]config.Target(nil), project.Targets...)
	for index, target := range clone.Targets {
		clone.Targets[index].Rules = append([]config.Rule(nil), target.Rules...)
		for ruleIndex, rule := range clone.Targets[index].Rules {
			clone.Targets[index].Rules[ruleIndex].Metadata = append([]config.MetadataEntry(nil), rule.Metadata...)
		}
	}
	return clone
}

func cloneSources(sources []Source) []Source {
	clone := append([]Source(nil), sources...)
	for index, source := range clone {
		clone[index].Projection.Bytes = append([]byte(nil), source.Projection.Bytes...)
		clone[index].Frontmatter = cloneMap(source.Frontmatter)
	}
	return clone
}

func cloneProjection(projection compile.SourceProjection) compile.SourceProjection {
	clone := projection
	clone.Bytes = append([]byte(nil), projection.Bytes...)
	clone.Contracts = append([]compile.ProjectedDeclaration(nil), projection.Contracts...)
	clone.Anchors = append([]compile.ProjectedDeclaration(nil), projection.Anchors...)
	return clone
}

func hasCode(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		if nested, ok := value.(map[string]any); ok {
			output[key] = cloneMap(nested)
			continue
		}
		output[key] = value
	}
	return output
}
