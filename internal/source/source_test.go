package source

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"github.com/akitanabe/gunte/internal/config"
)

func TestNormalizeCanonicalizesBOMLineEndingsAndTrailingLF(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{name: "empty", input: nil, want: []byte("\n")},
		{name: "bom", input: append([]byte{0xef, 0xbb, 0xbf}, []byte("alpha")...), want: []byte("alpha\n")},
		{name: "mixed line endings", input: []byte("a\r\nb\rc\n"), want: []byte("a\nb\nc\n")},
		{name: "without trailing newline", input: []byte("a\n\n"), want: []byte("a\n")},
		{name: "does not mutate input", input: []byte("a\r\n"), want: []byte("a\n")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := append([]byte(nil), tt.input...)
			got, diagnostics := Normalize(input)
			if len(diagnostics) != 0 {
				t.Fatalf("Normalize() diagnostics = %#v", diagnostics)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("Normalize() = %q, want %q", got, tt.want)
			}
			if !bytes.Equal(input, tt.input) {
				t.Fatalf("Normalize() mutated input: got %q, want %q", input, tt.input)
			}
		})
	}
}

func TestNormalizeRejectsInvalidUTF8AtNormalizedPosition(t *testing.T) {
	input := []byte("ok\r\nline\r\n\xff")
	got, diagnostics := Normalize(input)
	if len(diagnostics) != 1 {
		t.Fatalf("Normalize() diagnostics = %#v, want one diagnostic", diagnostics)
	}
	if diagnostics[0].Line != 3 || diagnostics[0].Column != 1 || diagnostics[0].Offset != 8 {
		t.Fatalf("invalid UTF-8 position = %#v, want offset 8 line 3 column 1", diagnostics[0])
	}
	if utf8.Valid(got) {
		t.Fatalf("Normalize() returned valid UTF-8 buffer for invalid input: %x", got)
	}
}

func TestNormalizeRemovesOnlyOneLeadingBOM(t *testing.T) {
	input := append([]byte{0xef, 0xbb, 0xbf, 0xef, 0xbb, 0xbf}, []byte("body")...)
	got, diagnostics := Normalize(input)
	want := append([]byte{0xef, 0xbb, 0xbf}, []byte("body\n")...)
	if len(diagnostics) != 0 {
		t.Fatalf("Normalize() diagnostics = %#v", diagnostics)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Normalize() = %x, want %x", got, want)
	}
}

func TestNormalizeReportsInvalidUTF8ColumnAsByteOffset(t *testing.T) {
	input := append([]byte("é"), 0xff)
	_, diagnostics := Normalize(input)
	if len(diagnostics) != 1 {
		t.Fatalf("Normalize() diagnostics = %#v, want one diagnostic", diagnostics)
	}
	if diagnostics[0].Offset != 2 || diagnostics[0].Line != 1 || diagnostics[0].Column != 3 {
		t.Fatalf("invalid UTF-8 position = %#v, want offset 2 line 1 column 3", diagnostics[0])
	}
}

func TestSplitRequiresExactFrontmatterDelimitersAndPreservesHalfOpenRanges(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		frontmatter *Range
		body        Range
		wantError   bool
	}{
		{
			name:        "frontmatter includes one following blank line",
			input:       []byte("+++\na = 1\n+++\n\nbody\n"),
			frontmatter: &Range{Start: 0, End: 15},
			body:        Range{Start: 15, End: 20},
		},
		{
			name:  "no frontmatter",
			input: []byte(" +++\nbody\n"),
			body:  Range{Start: 0, End: 10},
		},
		{
			name:        "closing delimiter must be exact",
			input:       []byte("+++\na = 1\n +++\nbody\n"),
			frontmatter: nil,
			body:        Range{},
			wantError:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts, diagnostics := Split(tt.input)
			if tt.wantError {
				if len(diagnostics) != 1 {
					t.Fatalf("Split() diagnostics = %#v, want unclosed-frontmatter diagnostic", diagnostics)
				}
				return
			}
			if len(diagnostics) != 0 {
				t.Fatalf("Split() diagnostics = %#v", diagnostics)
			}
			if !sameRange(parts.FrontmatterRange, tt.frontmatter) {
				t.Fatalf("frontmatter range = %#v, want %#v", parts.FrontmatterRange, tt.frontmatter)
			}
			if parts.BodyRange != tt.body {
				t.Fatalf("body range = %#v, want %#v", parts.BodyRange, tt.body)
			}
			if got := tt.input[parts.BodyRange.Start:parts.BodyRange.End]; got == nil {
				t.Fatalf("body range is not a valid half-open range")
			}
		})
	}
}

func TestSplitTreatsOpeningDelimiterWithTrailingWhitespaceAsBody(t *testing.T) {
	input := []byte("+++ \nbody\n")
	parts, diagnostics := Split(input)
	if len(diagnostics) != 0 {
		t.Fatalf("Split() diagnostics = %#v", diagnostics)
	}
	if parts.FrontmatterRange != nil {
		t.Fatalf("frontmatter range = %#v, want nil", parts.FrontmatterRange)
	}
	if parts.BodyRange != (Range{Start: 0, End: len(input)}) {
		t.Fatalf("body range = %#v, want [0,%d)", parts.BodyRange, len(input))
	}
	if got := input[parts.BodyRange.Start:parts.BodyRange.End]; string(got) != "+++ \nbody\n" {
		t.Fatalf("body = %q, want original bytes", got)
	}
}

func TestSplitAbsorbsAtMostOneBlankLineAfterClosingDelimiter(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		frontmatter Range
		body        Range
		bodyBytes   string
	}{
		{
			name:        "no blank line",
			input:       "+++\na = 1\n+++\nbody\n",
			frontmatter: Range{Start: 0, End: 14},
			body:        Range{Start: 14, End: 19},
			bodyBytes:   "body\n",
		},
		{
			name:        "one blank line",
			input:       "+++\na = 1\n+++\n\nbody\n",
			frontmatter: Range{Start: 0, End: 15},
			body:        Range{Start: 15, End: 20},
			bodyBytes:   "body\n",
		},
		{
			name:        "two blank lines",
			input:       "+++\na = 1\n+++\n\n\nbody\n",
			frontmatter: Range{Start: 0, End: 15},
			body:        Range{Start: 15, End: 21},
			bodyBytes:   "\nbody\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts, diagnostics := Split([]byte(tt.input))
			if len(diagnostics) != 0 {
				t.Fatalf("Split() diagnostics = %#v", diagnostics)
			}
			if !sameRange(parts.FrontmatterRange, &tt.frontmatter) {
				t.Fatalf("frontmatter range = %#v, want %#v", parts.FrontmatterRange, tt.frontmatter)
			}
			if parts.BodyRange != tt.body {
				t.Fatalf("body range = %#v, want %#v", parts.BodyRange, tt.body)
			}
			if got := string([]byte(tt.input)[parts.BodyRange.Start:parts.BodyRange.End]); got != tt.bodyBytes {
				t.Fatalf("body = %q, want %q", got, tt.bodyBytes)
			}
		})
	}
}

func TestSplitReportsUnclosedFrontmatterAtOpeningPosition(t *testing.T) {
	parts, diagnostics := Split([]byte("+++\na = 1\nbody\n"))
	if parts.BodyRange != (Range{}) {
		t.Fatalf("Split() body = %#v on error, want zero range", parts.BodyRange)
	}
	if len(diagnostics) != 1 {
		t.Fatalf("Split() diagnostics = %#v, want one diagnostic", diagnostics)
	}
	if diagnostics[0].Line != 1 || diagnostics[0].Column != 1 || diagnostics[0].Offset != 0 {
		t.Fatalf("unclosed frontmatter position = %#v, want opening position", diagnostics[0])
	}
}

func TestSplitRejectsNonExactClosingDelimitersAsUnclosedFrontmatter(t *testing.T) {
	for _, closing := range []string{"+++ ", "++++"} {
		t.Run(closing, func(t *testing.T) {
			parts, diagnostics := Split([]byte("+++\na = 1\n" + closing + "\nbody\n"))
			if parts != (Parts{}) {
				t.Fatalf("Split() parts = %#v on unclosed frontmatter", parts)
			}
			if len(diagnostics) != 1 {
				t.Fatalf("Split() diagnostics = %#v, want one diagnostic", diagnostics)
			}
			if diagnostics[0].Offset != 0 || diagnostics[0].Line != 1 || diagnostics[0].Column != 1 {
				t.Fatalf("unclosed frontmatter position = %#v, want opening position", diagnostics[0])
			}
		})
	}
}

func TestParseFrontmatterRetainsNestedValuesAndLiteralTerms(t *testing.T) {
	input := []byte("+++\n[claude]\ndescription = \"keep {{term}} literal\"\n[claude.options]\ncount = 2\n+++\nbody\n")
	parts, diagnostics := Split(input)
	if len(diagnostics) != 0 {
		t.Fatalf("Split() diagnostics = %#v", diagnostics)
	}
	frontmatter, diagnostics := ParseFrontmatter("source.md", input, *parts.FrontmatterRange)
	if len(diagnostics) != 0 {
		t.Fatalf("ParseFrontmatter() diagnostics = %#v", diagnostics)
	}
	claude, ok := frontmatter["claude"].(map[string]any)
	if !ok || claude["description"] != "keep {{term}} literal" {
		t.Fatalf("frontmatter claude = %#v", frontmatter["claude"])
	}
	options, ok := claude["options"].(map[string]any)
	if !ok || options["count"] != int64(2) {
		t.Fatalf("frontmatter options = %#v", claude["options"])
	}
}

func TestParseFrontmatterReportsTOMLSyntaxPositionOnNormalizedBuffer(t *testing.T) {
	input := []byte("+++\n[claude\ndescription = \"broken\"\n+++\nbody\n")
	parts, diagnostics := Split(input)
	if len(diagnostics) != 0 {
		t.Fatalf("Split() diagnostics = %#v", diagnostics)
	}
	_, diagnostics = ParseFrontmatter("source.md", input, *parts.FrontmatterRange)
	if len(diagnostics) != 1 {
		t.Fatalf("ParseFrontmatter() diagnostics = %#v, want one diagnostic", diagnostics)
	}
	want := Diagnostic{
		Path:    "source.md",
		Offset:  11,
		Line:    2,
		Column:  8,
		Message: diagnostics[0].Message,
	}
	if diagnostics[0] != want {
		t.Fatalf("TOML syntax diagnostic = %#v, want path/offset/line/column %#v", diagnostics[0], want)
	}

	document, parseDiagnostics := Parse("source.md", input)
	if document.FrontmatterData != nil {
		t.Fatalf("Parse() frontmatter data = %#v on syntax error, want nil", document.FrontmatterData)
	}
	if len(parseDiagnostics) != 1 || parseDiagnostics[0] != diagnostics[0] {
		t.Fatalf("Parse() diagnostics = %#v, want %#v", parseDiagnostics, diagnostics)
	}
}

func TestParseNormalizesBeforeSplittingAndParsingFrontmatter(t *testing.T) {
	input := append([]byte{0xef, 0xbb, 0xbf}, []byte("+++\r\n[claude]\r\ndescription = \"ok\"\r\n[claude.options]\r\ncount = 2\r\n+++\r\n\r\nbody\r\n")...)
	document, diagnostics := Parse("source.md", input)
	if len(diagnostics) != 0 {
		t.Fatalf("Parse() diagnostics = %#v", diagnostics)
	}
	wantBuffer := []byte("+++\n[claude]\ndescription = \"ok\"\n[claude.options]\ncount = 2\n+++\n\nbody\n")
	if !bytes.Equal(document.Buffer, wantBuffer) {
		t.Fatalf("normalized buffer = %q, want %q", document.Buffer, wantBuffer)
	}
	wantFrontmatter := &Range{Start: 0, End: 64}
	if !sameRange(document.FrontmatterRange, wantFrontmatter) {
		t.Fatalf("frontmatter range = %#v, want %#v", document.FrontmatterRange, wantFrontmatter)
	}
	if document.BodyRange != (Range{Start: 64, End: 69}) {
		t.Fatalf("body range = %#v, want [64,69)", document.BodyRange)
	}
	if got := string(document.Buffer[document.BodyRange.Start:document.BodyRange.End]); got != "body\n" {
		t.Fatalf("body = %q, want %q", got, "body\n")
	}
	claude, ok := document.FrontmatterData["claude"].(map[string]any)
	if !ok || claude["description"] != "ok" {
		t.Fatalf("claude frontmatter = %#v", document.FrontmatterData["claude"])
	}
	options, ok := claude["options"].(map[string]any)
	if !ok || options["count"] != int64(2) {
		t.Fatalf("nested frontmatter = %#v", claude["options"])
	}
}

func TestParseNormalizesBeforeReportingFrontmatterTOMLSyntaxPosition(t *testing.T) {
	input := append([]byte{0xef, 0xbb, 0xbf}, []byte("+++\r\n[claude\r\ndescription = \"broken\"\r\n+++\r\nbody\r\n")...)
	document, diagnostics := Parse("source.md", input)
	if document.FrontmatterData != nil {
		t.Fatalf("Parse() frontmatter data = %#v on syntax error, want nil", document.FrontmatterData)
	}
	if len(diagnostics) != 1 {
		t.Fatalf("Parse() diagnostics = %#v, want one diagnostic", diagnostics)
	}
	if diagnostics[0].Path != "source.md" || diagnostics[0].Offset != 11 || diagnostics[0].Line != 2 || diagnostics[0].Column != 8 {
		t.Fatalf("syntax diagnostic = %#v, want path source.md offset 11 line 2 column 8", diagnostics[0])
	}
}

func TestParseOracleSourcesNormalizesSplitsAndParsesAllDeclaredFiles(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "oracle", "input")
	projectInput, err := os.ReadFile(filepath.Join(root, "gunte.toml"))
	if err != nil {
		t.Fatal(err)
	}
	project, configDiagnostics := config.ParseProject("gunte.toml", projectInput)
	if len(configDiagnostics) != 0 {
		t.Fatalf("ParseProject() diagnostics = %#v", configDiagnostics)
	}
	if len(project.Sources.Files) != 40 {
		t.Fatalf("oracle source count = %d, want 40", len(project.Sources.Files))
	}
	for _, relativePath := range project.Sources.Files {
		t.Run(relativePath, func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relativePath)))
			if err != nil {
				t.Fatal(err)
			}
			document, diagnostics := Parse(relativePath, input)
			if len(diagnostics) != 0 {
				t.Fatalf("Parse() diagnostics = %#v", diagnostics)
			}
			if len(document.Buffer) == 0 || document.Buffer[len(document.Buffer)-1] != '\n' {
				t.Fatalf("normalized buffer does not end in exactly one LF")
			}
			if document.BodyRange.End != len(document.Buffer) || document.BodyRange.Start > document.BodyRange.End {
				t.Fatalf("body range = %#v for buffer length %d", document.BodyRange, len(document.Buffer))
			}
		})
	}
}

func sameRange(got, want *Range) bool {
	if got == nil || want == nil {
		return got == want
	}
	return *got == *want
}
