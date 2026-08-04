package serialize

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/akitanabe/gunte/internal/adapter"
	"github.com/akitanabe/gunte/internal/compile"
	"github.com/akitanabe/gunte/internal/config"
	"github.com/akitanabe/gunte/internal/source"
)

func TestSerializeMarkdownInsertsHeaderAndMapsPositions(t *testing.T) {
	input := adapter.Artifact{
		TargetID: "codex", SourcePath: "shared/a.md", Path: "out/a.md",
		Profile: config.ProfileMarkdown, Header: "Generated",
		Body:      []byte("before\ncontent\nafter\n"),
		Contracts: []compile.ProjectedDeclaration{{ID: "review", Emitted: true, ProjectedRange: source.Range{Start: 7, End: 15}}},
		Anchors:   []compile.ProjectedDeclaration{{ID: "after", Emitted: true, ProjectedRange: source.Range{Start: 16, End: 16}}},
	}
	got, diagnostics := Serialize(input)
	if len(diagnostics) != 0 {
		t.Fatalf("Serialize() diagnostics = %#v", diagnostics)
	}
	want := []byte("Generated\n\nbefore\ncontent\nafter\n")
	if !bytes.Equal(got.Bytes, want) {
		t.Fatalf("bytes = %q, want %q", got.Bytes, want)
	}
	if got.Path != input.Path || got.TargetID != input.TargetID || got.SourcePath != input.SourcePath {
		t.Fatalf("artifact identity = %#v", got)
	}
	if got.Contracts[0].ArtifactRange != (source.Range{Start: 18, End: 26}) || got.Anchors[0].ArtifactRange != (source.Range{Start: 27, End: 27}) {
		t.Fatalf("mapped declarations = %#v %#v", got.Contracts, got.Anchors)
	}
}

func TestSerializeAcceptsOptionalHeadersWithoutExtraBytes(t *testing.T) {
	tests := []struct {
		name  string
		input adapter.Artifact
		want  string
	}{
		{"markdown", adapter.Artifact{Profile: config.ProfileMarkdown, Body: []byte("body\n")}, "body\n"},
		{"yaml generated", adapter.Artifact{Profile: config.ProfileYAML, Body: []byte("body\n"), Metadata: []adapter.MetadataField{{Field: "name", Value: adapter.MetadataValue{Type: config.MetadataString, String: "agent"}}}}, "---\nname: \"agent\"\n---\nbody\n"},
		{"yaml preserve", adapter.Artifact{Profile: config.ProfileYAML, Body: []byte("---\nname: source\n---\nbody\n")}, "---\nname: source\n---\nbody\n"},
		{"toml", adapter.Artifact{Profile: config.ProfileTOML, BodyField: "body", Body: []byte("body\n")}, "body = \"\"\"\nbody\n\"\"\"\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, diagnostics := Serialize(test.input)
			if len(diagnostics) != 0 || string(got.Bytes) != test.want {
				t.Fatalf("result = %q diagnostics = %#v, want %q", got.Bytes, diagnostics, test.want)
			}
		})
	}
}

func TestSerializeRejectsNonEmptyInvalidHeaders(t *testing.T) {
	for _, header := range []string{"line\nbreak", string([]byte{0xff})} {
		_, diagnostics := Serialize(adapter.Artifact{Path: "out/a", Profile: config.ProfileMarkdown, Header: header, Body: []byte("body\n")})
		if !hasCode(diagnostics, "invalid_header") || diagnostics[0].Path != "out/a" {
			t.Fatalf("header %q diagnostics = %#v", header, diagnostics)
		}
	}
}

func TestSerializeRemovesBOMNormalizesLineEndingsAndKeepsOneFinalLF(t *testing.T) {
	input := adapter.Artifact{Profile: config.ProfileMarkdown, Body: []byte{0xef, 0xbb, 0xbf, 'a', '\r', '\n', 'b', '\r', '\n', '\n'}}
	got, diagnostics := Serialize(input)
	if len(diagnostics) != 0 || string(got.Bytes) != "a\nb\n" {
		t.Fatalf("bytes = %q diagnostics = %#v", got.Bytes, diagnostics)
	}
}

func TestSerializeYAMLGenerationAndLogicalTypes(t *testing.T) {
	input := adapter.Artifact{
		Profile: config.ProfileYAML, Header: "Generated",
		Body: []byte("body\n"),
		Metadata: []adapter.MetadataField{
			{Field: "name", Value: adapter.MetadataValue{Type: config.MetadataString, String: "a\"b"}},
			{Field: "tools", Value: adapter.MetadataValue{Type: config.MetadataCommaList, Strings: []string{"Read", "Write"}}},
			{Field: "modes", Value: adapter.MetadataValue{Type: config.MetadataStringList, Strings: []string{"a", "b"}}},
			{Field: "kind", Value: adapter.MetadataValue{Type: config.MetadataPlainToken, String: "Agent"}},
		},
	}
	got, diagnostics := Serialize(input)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	want := "---\nname: \"a\\\"b\"\ntools: Read, Write\nmodes: [\"a\", \"b\"]\nkind: Agent\n---\nGenerated\n\nbody\n"
	if string(got.Bytes) != want {
		t.Fatalf("bytes = %q, want %q", got.Bytes, want)
	}
}

func TestSerializeYAMLPreserveKeepsFrontmatterBytesAndMapsBody(t *testing.T) {
	body := []byte("---\nname: original\ncustom: [x, y]\n---\nbody\n")
	input := adapter.Artifact{
		Profile: config.ProfileYAML, Header: "Generated", Body: body,
		Contracts: []compile.ProjectedDeclaration{{ID: "body", Emitted: true, ProjectedRange: source.Range{Start: len("---\nname: original\ncustom: [x, y]\n---\n"), End: len(body) - 1}}},
	}
	got, diagnostics := Serialize(input)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	want := "---\nname: original\ncustom: [x, y]\n---\nGenerated\n\nbody\n"
	if string(got.Bytes) != want {
		t.Fatalf("bytes = %q, want %q", got.Bytes, want)
	}
	start := strings.Index(want, "body")
	if got.Contracts[0].ArtifactRange != (source.Range{Start: start, End: start + 4}) {
		t.Fatalf("mapped range = %#v, want [%d,%d)", got.Contracts[0].ArtifactRange, start, start+4)
	}
}

func TestSerializeYAMLPreserveRemovesProjectionBlankLineBeforeHeader(t *testing.T) {
	body := []byte("---\nname: source\n---\n\nbody\n")
	bodyStart := bytes.Index(body, []byte("body"))
	if bodyStart < 0 {
		t.Fatal("body fixture is missing its declaration text")
	}
	input := adapter.Artifact{
		Profile: config.ProfileYAML, Header: "Generated", Body: body,
		Contracts: []compile.ProjectedDeclaration{{ID: "body", Emitted: true, ProjectedRange: source.Range{Start: bodyStart, End: bodyStart + len("body")}}},
	}
	got, diagnostics := Serialize(input)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	want := "---\nname: source\n---\nGenerated\n\nbody\n"
	if string(got.Bytes) != want {
		t.Fatalf("bytes = %q, want %q", got.Bytes, want)
	}
	wantStart := strings.Index(want, "body")
	if got.Contracts[0].ArtifactRange != (source.Range{Start: wantStart, End: wantStart + len("body")}) {
		t.Fatalf("mapped range = %#v, want [%d,%d)", got.Contracts[0].ArtifactRange, wantStart, wantStart+len("body"))
	}
}

func TestSerializeYAMLPreserveSeparatorDeletionMapsToInsertionBoundary(t *testing.T) {
	body := []byte("---\nname: source\n---\n\nbody\n")
	frontEnd, ok := yamlFrontmatterEnd(body)
	if !ok {
		t.Fatal("frontmatter fixture is invalid")
	}
	input := adapter.Artifact{
		Profile: config.ProfileYAML, Header: "Generated", Body: body,
		Contracts: []compile.ProjectedDeclaration{
			{ID: "anchor", Emitted: true, ProjectedRange: source.Range{Start: frontEnd, End: frontEnd}},
			{ID: "separator", Emitted: true, ProjectedRange: source.Range{Start: frontEnd, End: frontEnd + 1}},
		},
	}
	got, diagnostics := Serialize(input)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	want := "---\nname: source\n---\nGenerated\n\nbody\n"
	wantBoundary := strings.Index(want, "Generated") + len("Generated\n\n")
	wantRange := source.Range{Start: wantBoundary, End: wantBoundary}
	if got.Contracts[0].ArtifactRange != wantRange || got.Contracts[1].ArtifactRange != wantRange {
		t.Fatalf("separator mappings = %#v, want both %#v", got.Contracts, wantRange)
	}
}

func TestSerializeYAMLPreserveCanonicalSeparatorsAndMappings(t *testing.T) {
	tests := []struct {
		name          string
		body          []byte
		header        string
		want          string
		bodyStartFrom string
	}{
		{
			name:   "header with empty suffix",
			body:   []byte("---\nname: source\n---\n"),
			header: "Generated",
			want:   "---\nname: source\n---\nGenerated\n\n",
		},
		{
			name:          "header with multiple blank lines",
			body:          []byte("---\nname: source\n---\n\n\nbody\n"),
			header:        "Generated",
			want:          "---\nname: source\n---\nGenerated\n\n\nbody\n",
			bodyStartFrom: "body",
		},
		{
			name:          "header absent preserves blank lines",
			body:          []byte("---\nname: source\n---\n\n\nbody\n"),
			want:          "---\nname: source\n---\n\n\nbody\n",
			bodyStartFrom: "body",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			frontEnd, ok := yamlFrontmatterEnd(test.body)
			if !ok {
				t.Fatal("frontmatter fixture is invalid")
			}
			anchor := compile.ProjectedDeclaration{ID: "anchor", Emitted: true, ProjectedRange: source.Range{Start: frontEnd, End: frontEnd}}
			declarations := []compile.ProjectedDeclaration{anchor}
			if test.bodyStartFrom != "" {
				bodyStart := bytes.Index(test.body, []byte(test.bodyStartFrom))
				if bodyStart < 0 {
					t.Fatalf("body fixture is missing %q", test.bodyStartFrom)
				}
				declarations = append(declarations,
					compile.ProjectedDeclaration{ID: "body", Emitted: true, ProjectedRange: source.Range{Start: bodyStart, End: bodyStart + len(test.bodyStartFrom)}},
				)
			}
			declarations = append(declarations,
				compile.ProjectedDeclaration{ID: "eof", Emitted: true, ProjectedRange: source.Range{Start: len(test.body), End: len(test.body)}},
			)
			got, diagnostics := Serialize(adapter.Artifact{
				Profile: config.ProfileYAML, Header: test.header, Body: test.body, Contracts: declarations,
			})
			if len(diagnostics) != 0 {
				t.Fatalf("diagnostics = %#v", diagnostics)
			}
			if string(got.Bytes) != test.want {
				t.Fatalf("bytes = %q, want %q", got.Bytes, test.want)
			}
			if got.Contracts[len(got.Contracts)-1].ArtifactRange != (source.Range{Start: len(test.want), End: len(test.want)}) {
				t.Fatalf("EOF mapping = %#v, want %d", got.Contracts[len(got.Contracts)-1], len(test.want))
			}
			if test.header == "" {
				if got.Contracts[0].ArtifactRange != (source.Range{Start: frontEnd, End: frontEnd}) {
					t.Fatalf("headerless anchor mapping = %#v, want %d", got.Contracts[0], frontEnd)
				}
				return
			}
			headerEnd := frontEnd + len(test.header+"\n\n")
			if got.Contracts[0].ArtifactRange != (source.Range{Start: headerEnd, End: headerEnd}) {
				t.Fatalf("header anchor mapping = %#v, want %d", got.Contracts[0], headerEnd)
			}
			if len(got.Contracts) > 2 {
				bodyStart := bytes.Index(test.body, []byte(test.bodyStartFrom))
				trimmed := 1
				wantBodyStart := frontEnd + len(test.header+"\n\n") + (bodyStart - frontEnd - trimmed)
				wantBodyEnd := wantBodyStart + len(test.bodyStartFrom)
				if got.Contracts[1].ArtifactRange != (source.Range{Start: wantBodyStart, End: wantBodyEnd}) {
					t.Fatalf("body mapping = %#v, want [%d,%d)", got.Contracts[1], wantBodyStart, wantBodyEnd)
				}
			}
		})
	}
}

func TestSerializeTOMLHeaderMetadataAndEscapedBodyMapPositions(t *testing.T) {
	body := []byte("a\\\"\nβ\n")
	input := adapter.Artifact{
		Profile: config.ProfileTOML, Header: "Generated", BodyField: "prompt", Body: body,
		Metadata:  []adapter.MetadataField{{Field: "name", Value: adapter.MetadataValue{Type: config.MetadataString, String: "agent"}}},
		Contracts: []compile.ProjectedDeclaration{{ID: "all", Emitted: true, ProjectedRange: source.Range{Start: 0, End: len(body) - 1}}},
	}
	got, diagnostics := Serialize(input)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	want := "# Generated\nname = \"agent\"\nprompt = \"\"\"\na\\\\\\\"\nβ\n\"\"\"\n"
	if string(got.Bytes) != want {
		t.Fatalf("bytes = %q, want %q", got.Bytes, want)
	}
	start := strings.Index(want, "a\\\\")
	if got.Contracts[0].ArtifactRange != (source.Range{Start: start, End: start + len([]byte("a\\\\\\\"\nβ"))}) {
		t.Fatalf("mapped range = %#v", got.Contracts[0].ArtifactRange)
	}
}

func TestSerializeTOMLOmitsOptionalBodyFieldWithoutAddingAnEmptyLine(t *testing.T) {
	input := adapter.Artifact{Profile: config.ProfileTOML, Header: "Generated", Metadata: []adapter.MetadataField{{Field: "name", Value: adapter.MetadataValue{Type: config.MetadataString, String: "agent"}}}}
	got, diagnostics := Serialize(input)
	if len(diagnostics) != 0 || string(got.Bytes) != "# Generated\nname = \"agent\"\n" {
		t.Fatalf("result = %#v diagnostics = %#v", got, diagnostics)
	}
}

func TestSerializeHidesDeclarationsWhenProfileDoesNotEmitBody(t *testing.T) {
	input := adapter.Artifact{
		Profile:   config.ProfileTOML,
		Body:      []byte("body\n"),
		Contracts: []compile.ProjectedDeclaration{{ID: "contract", Emitted: true, ProjectedRange: source.Range{Start: 0, End: 4}}},
		Anchors:   []compile.ProjectedDeclaration{{ID: "anchor", Emitted: true, ProjectedRange: source.Range{Start: 4, End: 4}}},
	}
	got, diagnostics := Serialize(input)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if got.Contracts[0].Emitted || got.Contracts[0].ArtifactRange != (source.Range{}) || got.Anchors[0].Emitted || got.Anchors[0].ArtifactRange != (source.Range{}) {
		t.Fatalf("declarations = %#v %#v", got.Contracts, got.Anchors)
	}
	input.Profile = config.ProfilePlainText
	input.Value = &adapter.MetadataValue{Type: config.MetadataString, String: "1"}
	got, diagnostics = Serialize(input)
	if len(diagnostics) != 0 || got.Contracts[0].Emitted || got.Anchors[0].Emitted {
		t.Fatalf("plain declarations = %#v %#v", got.Contracts, got.Anchors)
	}
}

func TestSerializeMapsEOFBoundariesAcrossProfiles(t *testing.T) {
	body := []byte("body\n")
	inputs := []adapter.Artifact{
		{Profile: config.ProfileMarkdown, Header: "H", Body: body, Contracts: []compile.ProjectedDeclaration{{ID: "eof", Emitted: true, ProjectedRange: source.Range{Start: len(body), End: len(body)}}}},
		{Profile: config.ProfileYAML, Header: "H", Body: body, Metadata: []adapter.MetadataField{{Field: "name", Value: adapter.MetadataValue{Type: config.MetadataString, String: "a"}}}, Contracts: []compile.ProjectedDeclaration{{ID: "eof", Emitted: true, ProjectedRange: source.Range{Start: len(body), End: len(body)}}}},
		{Profile: config.ProfileTOML, Header: "H", BodyField: "body", Body: []byte("a\\\"\n"), Contracts: []compile.ProjectedDeclaration{{ID: "eof", Emitted: true, ProjectedRange: source.Range{Start: len([]byte("a\\\"\n")), End: len([]byte("a\\\"\n"))}}}},
		{Profile: config.ProfileJSON, Body: []byte("{\n  \"a\": \"x\"\n}\n"), Contracts: []compile.ProjectedDeclaration{{ID: "eof", Emitted: true, ProjectedRange: source.Range{Start: len([]byte("{\n  \"a\": \"x\"\n}\n")), End: len([]byte("{\n  \"a\": \"x\"\n}\n"))}}}},
	}
	for _, input := range inputs {
		got, diagnostics := Serialize(input)
		if len(diagnostics) != 0 {
			t.Fatalf("profile %s diagnostics = %#v", input.Profile, diagnostics)
		}
		declarations := got.Contracts
		wantEnd := len(got.Bytes)
		if input.Profile == config.ProfileTOML {
			wantEnd = bytes.LastIndex(got.Bytes, []byte("\"\"\"\n"))
		}
		if len(declarations) != 1 || declarations[0].ArtifactRange != (source.Range{Start: wantEnd, End: wantEnd}) {
			t.Fatalf("profile %s EOF declaration = %#v, artifact length=%d wantEnd=%d", input.Profile, declarations, len(got.Bytes), wantEnd)
		}
	}
}

func TestSerializeJSONPreservesSourceOrderOverlaysMetadataAndMapsNestedBytes(t *testing.T) {
	body := []byte("{\n  \"first\": \"a\\nβ\",\n  \"nested\": [\"x\", {\"value\": \"y\"}]\n}\n")
	rangeStart := strings.Index(string(body), "  \"nested\"")
	rangeEnd := strings.Index(string(body), "\n}\n") + 1
	input := adapter.Artifact{
		Profile: config.ProfileJSON, Body: body,
		Metadata:  []adapter.MetadataField{{Field: "first", Value: adapter.MetadataValue{Type: config.MetadataString, String: "replaced"}}, {Field: "new", Value: adapter.MetadataValue{Type: config.MetadataString, String: "z"}}},
		Contracts: []compile.ProjectedDeclaration{{ID: "value", Emitted: true, ProjectedRange: source.Range{Start: rangeStart, End: rangeEnd}}},
	}
	got, diagnostics := Serialize(input)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	want := "{\n  \"first\": \"replaced\",\n  \"nested\": [\n    \"x\",\n    {\n      \"value\": \"y\"\n    }\n  ],\n  \"new\": \"z\"\n}\n"
	if string(got.Bytes) != want {
		t.Fatalf("bytes = %q, want %q", got.Bytes, want)
	}
	wantOffset := strings.Index(want, "  \"nested\"")
	wantEnd := strings.Index(want, "\n}\n") + 1
	if got.Contracts[0].ArtifactRange != (source.Range{Start: wantOffset, End: wantEnd}) {
		t.Fatalf("mapped range = %#v, want [%d,%d)", got.Contracts[0].ArtifactRange, wantOffset, wantEnd)
	}
}

func TestSerializeJSONMapsEveryNonOverlayStringBoundaryAfterEscaping(t *testing.T) {
	body := []byte("{\"value\":\"a\\nβ\"}\n")
	contentStart := strings.Index(string(body), `"a\n`) + 1
	input := adapter.Artifact{
		Profile: config.ProfileJSON,
		Body:    body,
		Contracts: []compile.ProjectedDeclaration{
			{ID: "a", Emitted: true, ProjectedRange: source.Range{Start: contentStart, End: contentStart + 1}},
			{ID: "escaped", Emitted: true, ProjectedRange: source.Range{Start: contentStart, End: contentStart + 3}},
			{ID: "unicode", Emitted: true, ProjectedRange: source.Range{Start: contentStart + 3, End: contentStart + 5}},
		},
	}
	got, diagnostics := Serialize(input)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	want := "{\n  \"value\": \"a\\nβ\"\n}\n"
	if string(got.Bytes) != want {
		t.Fatalf("bytes = %q, want %q", got.Bytes, want)
	}
	wantStart := strings.Index(want, `"a\n`) + 1
	if got.Contracts[0].ArtifactRange != (source.Range{Start: wantStart, End: wantStart + 1}) {
		t.Fatalf("plain boundary = %#v", got.Contracts[0].ArtifactRange)
	}
	if got.Contracts[1].ArtifactRange != (source.Range{Start: wantStart, End: wantStart + 3}) {
		t.Fatalf("escape boundary = %#v", got.Contracts[1].ArtifactRange)
	}
	wantUnicodeStart := strings.Index(want, "β")
	if got.Contracts[2].ArtifactRange != (source.Range{Start: wantUnicodeStart, End: wantUnicodeStart + len([]byte("β"))}) {
		t.Fatalf("unicode boundary = %#v", got.Contracts[2].ArtifactRange)
	}
}

func TestSerializeJSONGeneratedMetadataNeverOverwritesSourceBoundaries(t *testing.T) {
	for _, test := range []struct {
		name     string
		metadata []adapter.MetadataField
	}{
		{name: "append", metadata: []adapter.MetadataField{{Field: "new", Value: adapter.MetadataValue{Type: config.MetadataString, String: "value"}}}},
		{name: "overlay", metadata: []adapter.MetadataField{{Field: "source", Value: adapter.MetadataValue{Type: config.MetadataString, String: "replaced"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := adapter.Artifact{
				Profile:   config.ProfileJSON,
				Body:      []byte("{\"source\":\"x\"}\n"),
				Metadata:  test.metadata,
				Contracts: []compile.ProjectedDeclaration{{ID: "root", Emitted: true, ProjectedRange: source.Range{Start: 0, End: 1}}},
				Anchors:   []compile.ProjectedDeclaration{{ID: "root-anchor", Emitted: true, ProjectedRange: source.Range{Start: 0, End: 0}}},
			}
			got, diagnostics := Serialize(input)
			if len(diagnostics) != 0 {
				t.Fatalf("diagnostics = %#v", diagnostics)
			}
			if got.Contracts[0].ArtifactRange != (source.Range{Start: 0, End: 4}) || got.Anchors[0].ArtifactRange != (source.Range{Start: 0, End: 0}) {
				t.Fatalf("source boundary mapping = %#v %#v, bytes=%q", got.Contracts, got.Anchors, got.Bytes)
			}
		})
	}
}

func TestSerializePlainTextUsesValueOnly(t *testing.T) {
	input := adapter.Artifact{Path: "VERSION", Profile: config.ProfilePlainText, Value: &adapter.MetadataValue{Type: config.MetadataString, String: "1.2.3"}, Body: []byte("ignored")}
	got, diagnostics := Serialize(input)
	if len(diagnostics) != 0 || string(got.Bytes) != "1.2.3\n" {
		t.Fatalf("result = %#v diagnostics = %#v", got, diagnostics)
	}
}

func TestSerializeRejectsInvalidUTF8EscapeAndMatrixValues(t *testing.T) {
	tests := []struct {
		name string
		in   adapter.Artifact
		code string
	}{
		{"invalid utf8", adapter.Artifact{Profile: config.ProfileMarkdown, Body: []byte{0xff}}, "invalid_utf8"},
		{"comma empty", adapter.Artifact{Profile: config.ProfileYAML, Header: "Generated", Metadata: []adapter.MetadataField{{Field: "tools", Value: adapter.MetadataValue{Type: config.MetadataCommaList, Strings: []string{"Read", ""}}}}}, "invalid_metadata"},
		{"reserved field", adapter.Artifact{Profile: config.ProfileYAML, Header: "Generated", Metadata: []adapter.MetadataField{{Field: "TRUE", Value: adapter.MetadataValue{Type: config.MetadataString, String: "x"}}}}, "invalid_metadata"},
		{"json matrix", adapter.Artifact{Profile: config.ProfileJSON, Metadata: []adapter.MetadataField{{Field: "a", Value: adapter.MetadataValue{Type: config.MetadataCommaList, Strings: []string{"x"}}}}}, "invalid_metadata"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, diagnostics := Serialize(test.in)
			if !hasCode(diagnostics, test.code) {
				t.Fatalf("diagnostics = %#v, want %s", diagnostics, test.code)
			}
		})
	}
}

func TestSerializeEscapesEveryASCIIControlWithoutEscapingNonASCII(t *testing.T) {
	var input strings.Builder
	for value := 0; value <= 0x1f; value++ {
		input.WriteByte(byte(value))
	}
	input.WriteByte(0x7f)
	input.WriteString("é")
	artifact := adapter.Artifact{Profile: config.ProfileYAML, Header: "Generated", Body: []byte("body\n"), Metadata: []adapter.MetadataField{{Field: "value", Value: adapter.MetadataValue{Type: config.MetadataString, String: input.String()}}}}
	got, diagnostics := Serialize(artifact)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	wantEscaped := "\\u0000\\u0001\\u0002\\u0003\\u0004\\u0005\\u0006\\u0007\\b\\t\\n\\u000b\\f\\r\\u000e\\u000f\\u0010\\u0011\\u0012\\u0013\\u0014\\u0015\\u0016\\u0017\\u0018\\u0019\\u001a\\u001b\\u001c\\u001d\\u001e\\u001f\\u007fé"
	wantLine := []byte("value: \"" + wantEscaped + "\"\n")
	if !bytes.Contains(got.Bytes, wantLine) {
		t.Fatalf("control/non-ASCII escaping = %q, want line %q", got.Bytes, wantLine)
	}
}

func TestSerializeJSONEmptyObjectDuplicateAndUnsupportedScalarCases(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		code string
		want string
	}{
		{name: "empty", body: nil, want: "{}\n"},
		{name: "duplicate", body: []byte(`{"a":"x","a":"y"}`), code: "invalid_json"},
		{name: "number", body: []byte(`{"a":1}`), code: "invalid_json"},
		{name: "boolean", body: []byte(`{"a":true}`), code: "invalid_json"},
		{name: "null", body: []byte(`{"a":null}`), code: "invalid_json"},
		{name: "array root", body: []byte(`[]`), code: "invalid_json"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, diagnostics := Serialize(adapter.Artifact{Profile: config.ProfileJSON, Body: test.body})
			if test.code != "" {
				if !hasCode(diagnostics, test.code) {
					t.Fatalf("diagnostics = %#v, want %s", diagnostics, test.code)
				}
				return
			}
			if len(diagnostics) != 0 || string(got.Bytes) != test.want {
				t.Fatalf("result = %#v diagnostics = %#v", got, diagnostics)
			}
		})
	}
	empty, diagnostics := Serialize(adapter.Artifact{Profile: config.ProfileJSON, Body: nil, Contracts: []compile.ProjectedDeclaration{{ID: "eof", Emitted: true, ProjectedRange: source.Range{}}}})
	if len(diagnostics) != 0 || empty.Contracts[0].ArtifactRange != (source.Range{Start: len(empty.Bytes), End: len(empty.Bytes)}) {
		t.Fatalf("empty JSON EOF = %#v diagnostics = %#v", empty.Contracts, diagnostics)
	}
}

func TestSerializeJSONCombinesSurrogatePairsAndRejectsLoneSurrogates(t *testing.T) {
	got, diagnostics := Serialize(adapter.Artifact{Profile: config.ProfileJSON, Body: []byte(`{"emoji":"\ud83d\ude00"}`)})
	if len(diagnostics) != 0 || string(got.Bytes) != "{\n  \"emoji\": \"😀\"\n}\n" {
		t.Fatalf("surrogate result = %q diagnostics = %#v", got.Bytes, diagnostics)
	}
	for _, body := range []string{`{"x":"\ud83d"}`, `{"x":"\ude00"}`} {
		_, diagnostics := Serialize(adapter.Artifact{Profile: config.ProfileJSON, Body: []byte(body)})
		if !hasCode(diagnostics, "invalid_json") {
			t.Fatalf("body %q diagnostics = %#v", body, diagnostics)
		}
	}
}

func TestSerializeMetadataProfileMatrix(t *testing.T) {
	valid := []struct {
		name    string
		profile config.Profile
		typeID  config.MetadataType
		want    string
	}{
		{"yaml string", config.ProfileYAML, config.MetadataString, "name: \"value\""},
		{"yaml list", config.ProfileYAML, config.MetadataStringList, "name: [\"a\", \"b\"]"},
		{"yaml comma", config.ProfileYAML, config.MetadataCommaList, "name: a, b"},
		{"yaml token", config.ProfileYAML, config.MetadataPlainToken, "name: Agent"},
		{"toml string", config.ProfileTOML, config.MetadataString, "name = \"value\""},
		{"toml list", config.ProfileTOML, config.MetadataStringList, "name = [\"a\", \"b\"]"},
		{"json string", config.ProfileJSON, config.MetadataString, "\"name\": \"value\""},
		{"json list", config.ProfileJSON, config.MetadataStringList, "\"name\": [\n    \"a\",\n    \"b\"\n  ]"},
	}
	for _, test := range valid {
		t.Run(test.name, func(t *testing.T) {
			stringValue := "value"
			if test.typeID == config.MetadataPlainToken {
				stringValue = "Agent"
			}
			input := adapter.Artifact{Profile: test.profile, Header: "H", Body: []byte("{}\n"), Metadata: []adapter.MetadataField{{Field: "name", Value: adapter.MetadataValue{Type: test.typeID, String: stringValue, Strings: []string{"a", "b"}}}}}
			if test.profile == config.ProfileYAML {
				input.Body = []byte("body\n")
			}
			got, diagnostics := Serialize(input)
			if len(diagnostics) != 0 || !bytes.Contains(got.Bytes, []byte(test.want)) {
				t.Fatalf("result = %q diagnostics = %#v, want %q", got.Bytes, diagnostics, test.want)
			}
		})
	}
	for _, test := range []struct {
		profile config.Profile
		typeID  config.MetadataType
	}{
		{config.ProfileTOML, config.MetadataCommaList},
		{config.ProfileTOML, config.MetadataPlainToken},
		{config.ProfileJSON, config.MetadataCommaList},
		{config.ProfileJSON, config.MetadataPlainToken},
	} {
		input := adapter.Artifact{Profile: test.profile, Header: "H", Body: []byte("{}\n"), Metadata: []adapter.MetadataField{{Field: "name", Value: adapter.MetadataValue{Type: test.typeID, Strings: []string{"a"}, String: "a"}}}}
		got, diagnostics := Serialize(input)
		if len(diagnostics) == 0 || !hasCode(diagnostics, "invalid_metadata") || got.Bytes != nil {
			t.Fatalf("profile=%s type=%s result=%q diagnostics=%#v", test.profile, test.typeID, got.Bytes, diagnostics)
		}
	}
	for _, value := range []string{"TRUE", "No", "off"} {
		_, diagnostics := Serialize(adapter.Artifact{Profile: config.ProfileYAML, Header: "H", Body: []byte("body\n"), Metadata: []adapter.MetadataField{{Field: "name", Value: adapter.MetadataValue{Type: config.MetadataPlainToken, String: value}}}})
		if !hasCode(diagnostics, "invalid_metadata") {
			t.Fatalf("reserved YAML token %q diagnostics = %#v", value, diagnostics)
		}
	}
	_, diagnostics := Serialize(adapter.Artifact{Profile: config.ProfileYAML, Header: "H", Body: []byte("body\n"), Metadata: []adapter.MetadataField{{Field: "tools", Value: adapter.MetadataValue{Type: config.MetadataCommaList, Strings: []string{"Read", "YES"}}}}})
	if !hasCode(diagnostics, "invalid_metadata") {
		t.Fatalf("reserved YAML comma element diagnostics = %#v", diagnostics)
	}
}

func TestSerializeYAMLPreserveRequiresStrictDelimiters(t *testing.T) {
	for _, body := range [][]byte{
		[]byte(" -- -\nvalue\n---\nbody\n"),
		[]byte("---\nvalue\n ---\nbody\n"),
		[]byte("---\nvalue\n"),
	} {
		_, diagnostics := Serialize(adapter.Artifact{Profile: config.ProfileYAML, Header: "Generated", Body: body})
		if !hasCode(diagnostics, "invalid_yaml_frontmatter") {
			t.Fatalf("body %q diagnostics = %#v", body, diagnostics)
		}
	}
}

func TestSerializeKeepsEmittedFalseAndDoesNotMutateInput(t *testing.T) {
	input := adapter.Artifact{
		Profile: config.ProfileMarkdown, Header: "Generated", Body: []byte("body\n"),
		Contracts: []compile.ProjectedDeclaration{{ID: "hidden", Emitted: false}},
	}
	original := input
	original.Body = append([]byte(nil), input.Body...)
	got, diagnostics := Serialize(input)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if got.Contracts[0].Emitted {
		t.Fatalf("hidden contract = %#v", got.Contracts)
	}
	if !reflect.DeepEqual(input, original) {
		t.Fatalf("Serialize() mutated input")
	}
}

func TestSerializeDoesNotMutateNestedMetadataOrDeclarationsAndReportsPath(t *testing.T) {
	value := &adapter.MetadataValue{Type: config.MetadataStringList, Strings: []string{"a", "b"}}
	input := adapter.Artifact{Path: "out/a", Profile: config.ProfileJSON, Body: []byte(`{"a":"x"}`), Metadata: []adapter.MetadataField{{Field: "list", Value: *value}}, Value: value, Contracts: []compile.ProjectedDeclaration{{ID: "hidden", Emitted: false}}}
	original := input
	original.Body = append([]byte(nil), input.Body...)
	original.Metadata = append([]adapter.MetadataField(nil), input.Metadata...)
	original.Metadata[0].Value.Strings = append([]string(nil), input.Metadata[0].Value.Strings...)
	original.Value = &adapter.MetadataValue{Type: value.Type, Strings: append([]string(nil), value.Strings...)}
	_, diagnostics := Serialize(input)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if !reflect.DeepEqual(input, original) {
		t.Fatalf("Serialize() mutated nested input")
	}
	_, diagnostics = Serialize(adapter.Artifact{Path: "out/b", Profile: config.ProfileJSON, Body: []byte(`{"x":1}`)})
	if len(diagnostics) == 0 || diagnostics[0].Path != "out/b" {
		t.Fatalf("diagnostic path = %#v", diagnostics)
	}
}

func TestSerializeDoesNotMutateContractsOrAnchors(t *testing.T) {
	input := adapter.Artifact{
		Profile: config.ProfileMarkdown,
		Header:  "H",
		Body:    []byte("body\n"),
		Contracts: []compile.ProjectedDeclaration{{
			ID: "contract", Source: compile.SourcePosition{Path: "source.md", Offset: 3}, Emitted: true, ProjectedRange: source.Range{Start: 0, End: 4},
		}},
		Anchors: []compile.ProjectedDeclaration{{
			ID: "anchor", Source: compile.SourcePosition{Path: "source.md", Offset: 4}, Emitted: true, ProjectedRange: source.Range{Start: 4, End: 4},
		}},
	}
	wantContracts := append([]compile.ProjectedDeclaration(nil), input.Contracts...)
	wantAnchors := append([]compile.ProjectedDeclaration(nil), input.Anchors...)
	_, diagnostics := Serialize(input)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if !reflect.DeepEqual(input.Contracts, wantContracts) || !reflect.DeepEqual(input.Anchors, wantAnchors) {
		t.Fatalf("Serialize() mutated declarations: contracts=%#v anchors=%#v", input.Contracts, input.Anchors)
	}
}

func hasCode(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
