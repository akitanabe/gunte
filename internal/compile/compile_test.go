package compile

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/akitanabe/gunte/internal/config"
	"github.com/akitanabe/gunte/internal/lexer"
	"github.com/akitanabe/gunte/internal/source"
)

func TestValidateAndProjectTracksDeclarationsAcrossBodySplices(t *testing.T) {
	input := []byte("+++\ntitle = \"ignored {{word}}\"\n+++\n\n" +
		"before {{word}}\n" +
		"<!-- @contract review -->\n" +
		"α{{word}}{{word}}\n" +
		"<!-- @only claude -->\n" +
		"<!-- @anchor kept -->\n" +
		"claude\n" +
		"<!-- @/only -->\n" +
		"<!-- @only codex -->\n" +
		"<!-- @anchor hidden -->\n" +
		"codex {{word}}\n" +
		"<!-- @/only -->\n" +
		"<!-- @/contract -->\n" +
		"after contract\n")
	unit := parsedUnit(t, "docs/a.md", input)
	project := config.ProjectConfig{
		Sources: config.Sources{Files: []string{"docs/a.md"}},
		Terms: []config.Term{{Name: "word", Values: []config.TargetValue{
			{TargetID: "claude", Value: "長い値"},
			{TargetID: "codex", Value: "x"},
		}}},
		Targets: []config.Target{{ID: "claude"}, {ID: "codex"}},
	}

	result, diagnostics := ValidateAndProject(project, []SourceUnit{unit})
	if len(diagnostics) != 0 {
		t.Fatalf("ValidateAndProject() diagnostics = %#v", diagnostics)
	}
	if len(result.Targets) != 2 || result.Targets[0].TargetID != "claude" || result.Targets[1].TargetID != "codex" {
		t.Fatalf("target order = %#v, want claude then codex", result.Targets)
	}

	claude := result.Targets[0].Sources[0]
	wantClaude := []byte("before 長い値\nα長い値長い値\nclaude\nafter contract\n")
	if !bytes.Equal(claude.Bytes, wantClaude) {
		t.Fatalf("claude projection = %q, want %q", claude.Bytes, wantClaude)
	}
	claudeContractEnd := strings.Index(string(wantClaude), "after contract")
	assertDeclaration(t, claude.Contracts, "review", true, source.Range{Start: len("before 長い値\n"), End: claudeContractEnd})
	if claude.Contracts[0].ProjectedRange.End >= len(wantClaude) {
		t.Fatalf("claude contract range includes trailing body: %#v", claude.Contracts[0].ProjectedRange)
	}
	keptOffset := strings.Index(string(wantClaude), "claude")
	assertDeclaration(t, claude.Anchors, "kept", true, source.Range{Start: keptOffset, End: keptOffset})
	assertDeclaration(t, claude.Anchors, "hidden", false, source.Range{})

	codex := result.Targets[1].Sources[0]
	wantCodex := []byte("before x\nαxx\ncodex x\nafter contract\n")
	if !bytes.Equal(codex.Bytes, wantCodex) {
		t.Fatalf("codex projection = %q, want %q", codex.Bytes, wantCodex)
	}
	codexContractEnd := strings.Index(string(wantCodex), "after contract")
	assertDeclaration(t, codex.Contracts, "review", true, source.Range{Start: len("before x\n"), End: codexContractEnd})
	if codex.Contracts[0].ProjectedRange.End >= len(wantCodex) {
		t.Fatalf("codex contract range includes trailing body: %#v", codex.Contracts[0].ProjectedRange)
	}
	assertDeclaration(t, codex.Anchors, "kept", false, source.Range{})
	hiddenOffset := strings.Index(string(wantCodex), "codex")
	assertDeclaration(t, codex.Anchors, "hidden", true, source.Range{Start: hiddenOffset, End: hiddenOffset})

	if bytes.Contains(claude.Bytes, []byte("title")) || bytes.Contains(codex.Bytes, []byte("title")) {
		t.Fatalf("projection included source frontmatter: claude=%q codex=%q", claude.Bytes, codex.Bytes)
	}
	if got := string(result.Targets[0].Sources[0].Bytes); strings.Contains(got, "{{word}}") {
		t.Fatalf("term replacement was recursive or incomplete: %q", got)
	}
}

func TestValidateAndProjectKeepsReplacementValuesLiteral(t *testing.T) {
	unit := parsedUnit(t, "terms.md", []byte("{{first}}{{second}}\n"))
	project := config.ProjectConfig{
		Sources: config.Sources{Files: []string{"terms.md"}},
		Terms: []config.Term{
			{Name: "first", Values: []config.TargetValue{{TargetID: "codex", Value: "{{second}}"}}},
			{Name: "second", Values: []config.TargetValue{{TargetID: "codex", Value: "界"}}},
		},
		Targets: []config.Target{{ID: "codex"}},
	}

	result, diagnostics := ValidateAndProject(project, []SourceUnit{unit})
	if len(diagnostics) != 0 {
		t.Fatalf("ValidateAndProject() diagnostics = %#v", diagnostics)
	}
	if got, want := string(result.Targets[0].Sources[0].Bytes), "{{second}}界\n"; got != want {
		t.Fatalf("projection = %q, want non-recursive %q", got, want)
	}
}

func TestValidateAndProjectReplacesBodyValuesWithoutRecursiveExpansion(t *testing.T) {
	input := []byte("before {{term}} {{release}}\n" +
		"<!-- @contract review -->\n" +
		"<!-- @only codex -->\n" +
		"kept {{release}}\n" +
		"<!-- @anchor tail -->\n" +
		"<!-- @/only -->\n" +
		"<!-- @only other -->\n" +
		"hidden {{release}}\n" +
		"<!-- @/only -->\n" +
		"<!-- @/contract -->\n" +
		"```md\n{{release}}\n```\n")
	unit := parsedUnit(t, "body-values.md", input)
	project := config.ProjectConfig{
		Project:    config.Project{Version: "1.{{term}}"},
		Sources:    config.Sources{Files: []string{"body-values.md"}},
		Terms:      []config.Term{{Name: "term", Values: []config.TargetValue{{TargetID: "codex", Value: "value"}}}},
		BodyValues: []config.BodyValue{{Name: "release", From: "project:version"}},
		Targets:    []config.Target{{ID: "codex"}, {ID: "other"}},
	}

	result, diagnostics := ValidateAndProject(project, []SourceUnit{unit})
	if len(diagnostics) != 0 {
		t.Fatalf("ValidateAndProject() diagnostics = %#v", diagnostics)
	}
	got := result.Targets[0].Sources[0]
	want := []byte("before value 1.{{term}}\nkept 1.{{term}}\n```md\n{{release}}\n```\n")
	if !bytes.Equal(got.Bytes, want) {
		t.Fatalf("projection = %q, want %q", got.Bytes, want)
	}
	contractStart := strings.Index(string(want), "kept")
	contractEnd := strings.Index(string(want), "```md")
	assertDeclaration(t, got.Contracts, "review", true, source.Range{Start: contractStart, End: contractEnd})
	assertDeclaration(t, got.Anchors, "tail", true, source.Range{Start: contractEnd, End: contractEnd})
	if strings.Contains(string(got.Bytes), "hidden") {
		t.Fatalf("excluded body value block was retained: %q", got.Bytes)
	}
}

func TestValidateAndProjectReportsUndefinedBodyValueTokens(t *testing.T) {
	unit := parsedUnit(t, "undefined-body-value.md", []byte("{{missing}}\n"))
	project := config.ProjectConfig{
		Sources:    config.Sources{Files: []string{"undefined-body-value.md"}},
		BodyValues: []config.BodyValue{{Name: "release", From: "project:version"}},
		Targets:    []config.Target{{ID: "codex"}},
	}
	_, diagnostics := ValidateAndProject(project, []SourceUnit{unit})
	want := []source.Diagnostic{{Path: "undefined-body-value.md", Offset: 0, Line: 1, Column: 1, Message: "undefined term missing"}}
	if !reflect.DeepEqual(diagnostics, want) {
		t.Fatalf("diagnostics = %#v, want %#v", diagnostics, want)
	}
}

func TestValidateAndProjectPreservesLiteralDirectiveShapesAndFences(t *testing.T) {
	input := []byte(" <!-- @anchor indented -->\n<!--@contract tight-->\n```md\n<!-- @only other -->\n{{unknown}}\n<!-- @/only -->\n```\n")
	unit := parsedUnit(t, "literal.md", input)
	project := config.ProjectConfig{Sources: config.Sources{Files: []string{"literal.md"}}, Targets: []config.Target{{ID: "codex"}}}

	result, diagnostics := ValidateAndProject(project, []SourceUnit{unit})
	if len(diagnostics) != 0 {
		t.Fatalf("ValidateAndProject() diagnostics = %#v", diagnostics)
	}
	if got := result.Targets[0].Sources[0].Bytes; !bytes.Equal(got, input) {
		t.Fatalf("literal bytes changed: got %q, want %q", got, input)
	}
}

func TestValidateAndProjectReportsDeclarationAndOnlyErrorsAtTheirSourcePositions(t *testing.T) {
	first := parsedUnit(t, "one.md", []byte("<!-- @contract shared -->\n<!-- @/contract -->\n<!-- @anchor _bad -->\n"))
	secondInput := []byte("é\n<!-- @anchor shared -->\n<!-- @only Missing -->\ntext\n<!-- @/only -->\n<!-- @only ghost -->\ntext\n<!-- @/only -->\n")
	second := parsedUnit(t, "two.md", secondInput)
	project := config.ProjectConfig{
		Sources: config.Sources{Files: []string{"one.md", "two.md"}},
		Targets: []config.Target{{ID: "codex"}},
	}

	_, diagnostics := ValidateAndProject(project, []SourceUnit{first, second})
	want := []source.Diagnostic{
		{Path: "one.md", Offset: 46, Line: 3, Column: 1, Message: "invalid span or anchor ID _bad"},
		{Path: "two.md", Offset: 3, Line: 2, Column: 1, Message: "duplicate span or anchor ID shared"},
		{Path: "two.md", Offset: 27, Line: 3, Column: 1, Message: "invalid only target Missing"},
		{Path: "two.md", Offset: 71, Line: 6, Column: 1, Message: "unknown only target ghost"},
	}
	if !reflect.DeepEqual(diagnostics, want) {
		t.Fatalf("diagnostics = %#v, want %#v", diagnostics, want)
	}
}

func TestValidateAndProjectAcceptsGeneralIDLengthBoundaryAndRejectsOverflow(t *testing.T) {
	valid := "A" + strings.Repeat("x", 63)
	invalid := valid + "y"
	input := []byte("<!-- @anchor " + valid + " -->\n<!-- @anchor " + invalid + " -->\n")
	unit := parsedUnit(t, "ids.md", input)
	project := config.ProjectConfig{Sources: config.Sources{Files: []string{"ids.md"}}, Targets: []config.Target{{ID: "codex"}}}

	_, diagnostics := ValidateAndProject(project, []SourceUnit{unit})
	if len(diagnostics) != 1 || diagnostics[0].Message != "invalid span or anchor ID "+invalid {
		t.Fatalf("diagnostics = %#v, want only overlength ID error", diagnostics)
	}
}

func TestValidateAndProjectAcceptsTargetIDLengthBoundaryAndRejectsOverflow(t *testing.T) {
	valid := "a" + strings.Repeat("x", 31)
	invalid := valid + "y"
	t.Run("32 byte target ID", func(t *testing.T) {
		unit := parsedUnit(t, "valid-target.md", []byte("<!-- @only "+valid+" -->\nbody\n<!-- @/only -->\n"))
		project := config.ProjectConfig{Sources: config.Sources{Files: []string{"valid-target.md"}}, Targets: []config.Target{{ID: valid}}}

		result, diagnostics := ValidateAndProject(project, []SourceUnit{unit})
		if len(diagnostics) != 0 {
			t.Fatalf("ValidateAndProject() diagnostics = %#v", diagnostics)
		}
		if got := string(result.Targets[0].Sources[0].Bytes); got != "body\n" {
			t.Fatalf("projection = %q, want retained body", got)
		}
	})
	t.Run("33 byte target ID", func(t *testing.T) {
		unit := parsedUnit(t, "invalid-target.md", []byte("<!-- @only "+invalid+" -->\nbody\n<!-- @/only -->\n"))
		project := config.ProjectConfig{Sources: config.Sources{Files: []string{"invalid-target.md"}}, Targets: []config.Target{{ID: "codex"}}}

		_, diagnostics := ValidateAndProject(project, []SourceUnit{unit})
		if len(diagnostics) != 1 || diagnostics[0].Message != "invalid only target "+invalid {
			t.Fatalf("diagnostics = %#v, want overlength target error", diagnostics)
		}
	})
}

func TestValidateAndProjectReportsUndefinedTermsBeforeExcludedProjection(t *testing.T) {
	input := []byte("<!-- @only claude -->\n{{missing}}\n<!-- @/only -->\n{{also_missing}}\n")
	unit := parsedUnit(t, "undefined.md", input)
	project := config.ProjectConfig{Sources: config.Sources{Files: []string{"undefined.md"}}, Targets: []config.Target{{ID: "claude"}, {ID: "codex"}}}

	_, diagnostics := ValidateAndProject(project, []SourceUnit{unit})
	want := []source.Diagnostic{
		{Path: "undefined.md", Offset: 22, Line: 2, Column: 1, Message: "undefined term missing"},
		{Path: "undefined.md", Offset: 50, Line: 4, Column: 1, Message: "undefined term also_missing"},
	}
	if !reflect.DeepEqual(diagnostics, want) {
		t.Fatalf("diagnostics = %#v, want %#v", diagnostics, want)
	}
}

func TestValidateAndProjectRetainsEmptyContractRange(t *testing.T) {
	unit := parsedUnit(t, "empty.md", []byte("<!-- @contract empty -->\n<!-- @/contract -->\n"))
	project := config.ProjectConfig{Sources: config.Sources{Files: []string{"empty.md"}}, Targets: []config.Target{{ID: "codex"}}}

	result, diagnostics := ValidateAndProject(project, []SourceUnit{unit})
	if len(diagnostics) != 0 {
		t.Fatalf("ValidateAndProject() diagnostics = %#v", diagnostics)
	}
	projection := result.Targets[0].Sources[0]
	if len(projection.Bytes) != 0 {
		t.Fatalf("projection = %q, want empty", projection.Bytes)
	}
	assertDeclaration(t, projection.Contracts, "empty", true, source.Range{})
}

func TestValidateAndProjectUsesConfiguredSourceOrderAndDoesNotMutateInputs(t *testing.T) {
	unitA := parsedUnit(t, "a.md", []byte("a {{term}}\n"))
	unitB := parsedUnit(t, "b.md", []byte("b {{term}}\n"))
	originalA := append([]byte(nil), unitA.Document.Buffer...)
	originalB := append([]byte(nil), unitB.Document.Buffer...)
	project := config.ProjectConfig{
		Sources: config.Sources{Files: []string{"b.md", "a.md"}},
		Terms:   []config.Term{{Name: "term", Values: []config.TargetValue{{TargetID: "codex", Value: "value"}}}},
		Targets: []config.Target{{ID: "codex"}},
	}

	result, diagnostics := ValidateAndProject(project, []SourceUnit{unitB, unitA})
	if len(diagnostics) != 0 {
		t.Fatalf("ValidateAndProject() diagnostics = %#v", diagnostics)
	}
	if got := []string{result.Targets[0].Sources[0].Path, result.Targets[0].Sources[1].Path}; !reflect.DeepEqual(got, []string{"b.md", "a.md"}) {
		t.Fatalf("source order = %#v", got)
	}
	if !bytes.Equal(unitA.Document.Buffer, originalA) || !bytes.Equal(unitB.Document.Buffer, originalB) {
		t.Fatalf("ValidateAndProject() mutated input buffers")
	}

	_, diagnostics = ValidateAndProject(project, []SourceUnit{unitA, unitB})
	if len(diagnostics) != 2 || diagnostics[0].Message != "source unit path a.md does not match configured path b.md" || diagnostics[1].Message != "source unit path b.md does not match configured path a.md" {
		t.Fatalf("source order diagnostics = %#v", diagnostics)
	}
}

func TestValidateAndProjectDoesNotMutateProjectOrCompleteSourceUnits(t *testing.T) {
	project, units := completeProjectionFixture(t)
	wantProject, wantUnits := completeProjectionFixture(t)

	_, diagnostics := ValidateAndProject(project, units)
	if len(diagnostics) != 0 {
		t.Fatalf("ValidateAndProject() diagnostics = %#v", diagnostics)
	}
	if !reflect.DeepEqual(project, wantProject) {
		t.Fatalf("ValidateAndProject() mutated ProjectConfig:\n got  %#v\n want %#v", project, wantProject)
	}
	if !reflect.DeepEqual(units, wantUnits) {
		t.Fatalf("ValidateAndProject() mutated SourceUnits:\n got  %#v\n want %#v", units, wantUnits)
	}
}

func TestOracleSourcesValidateAndProjectForEveryTarget(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "oracle", "input")
	projectBytes, err := os.ReadFile(filepath.Join(root, "gunte.toml"))
	if err != nil {
		t.Fatal(err)
	}
	project, configDiagnostics := config.ParseProject("gunte.toml", projectBytes)
	if len(configDiagnostics) != 0 {
		t.Fatalf("ParseProject() diagnostics = %#v", configDiagnostics)
	}
	units := make([]SourceUnit, 0, len(project.Sources.Files))
	for _, path := range project.Sources.Files {
		input, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		document, parseDiagnostics := source.Parse(path, input)
		if len(parseDiagnostics) != 0 {
			t.Fatalf("source.Parse(%q) diagnostics = %#v", path, parseDiagnostics)
		}
		ir, lexDiagnostics := lexer.Lex(path, document.Buffer, document.BodyRange)
		if len(lexDiagnostics) != 0 {
			t.Fatalf("lexer.Lex(%q) diagnostics = %#v", path, lexDiagnostics)
		}
		units = append(units, SourceUnit{Path: path, Document: document, IR: ir})
	}

	result, diagnostics := ValidateAndProject(project, units)
	if len(diagnostics) != 0 {
		t.Fatalf("ValidateAndProject() diagnostics = %#v", diagnostics)
	}
	if len(units) != 40 || len(result.Targets) != 2 {
		t.Fatalf("oracle cardinality: units=%d targets=%d", len(units), len(result.Targets))
	}
	for _, target := range result.Targets {
		if len(target.Sources) != 40 {
			t.Fatalf("target %s source projections = %d, want 40", target.TargetID, len(target.Sources))
		}
		for _, projected := range target.Sources {
			for _, term := range project.Terms {
				if bytes.Contains(projected.Bytes, []byte("{{"+term.Name+"}}")) {
					t.Fatalf("target %s source %s retained term %s", target.TargetID, projected.Path, term.Name)
				}
			}
			for _, marker := range []string{"<!-- @only ", "<!-- @/only -->", "<!-- @contract ", "<!-- @/contract -->", "<!-- @anchor "} {
				if bytes.Contains(projected.Bytes, []byte(marker)) {
					t.Fatalf("target %s source %s retained recognized marker %q", target.TargetID, projected.Path, marker)
				}
			}
		}
	}
	claude := projectionFor(t, result, "claude", "shared/skill/impl-lead/references/implementation-branches.md")
	if !bytes.Contains(claude.Bytes, []byte("新規の `Agent` 呼び出し")) || bytes.Contains(claude.Bytes, []byte("`spawn_agent` で新規 Implementer")) {
		t.Fatalf("claude projection did not select claude-only sentinel")
	}
	codex := projectionFor(t, result, "codex", "shared/skill/impl-lead/references/implementation-branches.md")
	if !bytes.Contains(codex.Bytes, []byte("`spawn_agent` で新規 Implementer")) || bytes.Contains(codex.Bytes, []byte("新規の `Agent` 呼び出し")) {
		t.Fatalf("codex projection did not select codex-only sentinel")
	}
	implementer := projectionFor(t, result, "codex", "shared/agents/implementer.md")
	if !bytes.Contains(implementer.Bytes, []byte("親 Codex エージェント")) {
		t.Fatalf("codex projection did not resolve target-specific term")
	}
}

func parsedUnit(t *testing.T, path string, input []byte) SourceUnit {
	t.Helper()
	document, diagnostics := source.Parse(path, input)
	if len(diagnostics) != 0 {
		t.Fatalf("source.Parse() diagnostics = %#v", diagnostics)
	}
	ir, diagnostics := lexer.Lex(path, document.Buffer, document.BodyRange)
	if len(diagnostics) != 0 {
		t.Fatalf("lexer.Lex() diagnostics = %#v", diagnostics)
	}
	return SourceUnit{Path: path, Document: document, IR: ir}
}

func completeProjectionFixture(t *testing.T) (config.ProjectConfig, []SourceUnit) {
	t.Helper()
	input := []byte("+++\n[owner]\nname = \"fixture\"\n+++\n\n" +
		"before {{term}}\n" +
		"<!-- @contract contract -->\n" +
		"<!-- @only claude -->\n" +
		"<!-- @anchor anchor -->\n" +
		"body {{term}}\n" +
		"<!-- @/only -->\n" +
		"<!-- @/contract -->\n")
	project := config.ProjectConfig{
		SpecVersion: 1,
		Project:     config.Project{ID: "fixture", Version: "1.0.0"},
		Sources:     config.Sources{Files: []string{"fixture.md"}},
		Terms: []config.Term{{Name: "term", Values: []config.TargetValue{
			{TargetID: "claude", Value: "Claude"},
			{TargetID: "codex", Value: "Codex"},
		}}},
		Targets: []config.Target{
			{ID: "claude", OutputRoot: "out/claude", Rules: []config.Rule{{
				Match: "*.md", Path: "{1}.md", Profile: config.ProfileYAML,
				Metadata: []config.MetadataEntry{{Field: "name", From: "frontmatter:owner.name", Type: config.MetadataString, Required: true}},
			}}},
			{ID: "codex", OutputRoot: "out/codex", Rules: []config.Rule{{
				Match: "*.md", Path: "{1}.toml", Profile: config.ProfileTOML, BodyField: "instructions",
				Metadata: []config.MetadataEntry{{Field: "name", From: "frontmatter:owner.name", Type: config.MetadataString, Required: true}},
			}}},
		},
	}
	return project, []SourceUnit{parsedUnit(t, "fixture.md", input)}
}

func assertDeclaration(t *testing.T, declarations []ProjectedDeclaration, id string, emitted bool, projectedRange source.Range) {
	t.Helper()
	for _, declaration := range declarations {
		if declaration.ID == id {
			if declaration.Emitted != emitted || declaration.ProjectedRange != projectedRange {
				t.Fatalf("declaration %s = %#v, want emitted=%t range=%#v", id, declaration, emitted, projectedRange)
			}
			if declaration.Source.Path == "" || declaration.Source.Line == 0 || declaration.Source.Column == 0 {
				t.Fatalf("declaration %s source position = %#v", id, declaration.Source)
			}
			return
		}
	}
	t.Fatalf("declaration %s not found in %#v", id, declarations)
}

func projectionFor(t *testing.T, result Result, targetID, path string) SourceProjection {
	t.Helper()
	for _, target := range result.Targets {
		if target.TargetID != targetID {
			continue
		}
		for _, projected := range target.Sources {
			if projected.Path == path {
				return projected
			}
		}
	}
	t.Fatalf("projection target=%s path=%s not found", targetID, path)
	return SourceProjection{}
}
