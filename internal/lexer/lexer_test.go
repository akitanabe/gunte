package lexer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akitanabe/gunte/internal/config"
	"github.com/akitanabe/gunte/internal/source"
)

func TestLexDescribesMarkersBlocksAnchorsAndTermsInSourceOrder(t *testing.T) {
	buffer := []byte("frontmatter\n<!-- @contract review -->\nfirst {{parent_agent}}\n<!-- @only codex -->\n<!-- @anchor gate -->\nsecond\n<!-- @/only -->\n<!-- @/contract -->\n")
	body := source.Range{Start: len("frontmatter\n"), End: len(buffer)}

	ir, diagnostics := Lex("source.md", buffer, body)
	if len(diagnostics) != 0 {
		t.Fatalf("Lex() diagnostics = %#v", diagnostics)
	}
	wantKinds := []MarkerKind{ContractOpen, OnlyOpen, AnchorMarker, OnlyClose, ContractClose}
	if len(ir.Markers) != len(wantKinds) {
		t.Fatalf("markers = %#v, want %d markers", ir.Markers, len(wantKinds))
	}
	for index, want := range wantKinds {
		if ir.Markers[index].Kind != want {
			t.Fatalf("marker %d kind = %q, want %q", index, ir.Markers[index].Kind, want)
		}
	}
	if ir.Markers[0].Token != "review" || ir.Markers[1].Token != "codex" || ir.Markers[2].Token != "gate" {
		t.Fatalf("marker tokens = %#v", ir.Markers)
	}
	firstMarkerStart := strings.Index(string(buffer), "<!-- @contract")
	if ir.Markers[0].Range != (source.Range{Start: firstMarkerStart, End: firstMarkerStart + len("<!-- @contract review -->\n")}) {
		t.Fatalf("contract marker range = %#v", ir.Markers[0].Range)
	}
	if ir.Markers[0].Position != (Position{Offset: firstMarkerStart, Line: 2, Column: 1}) {
		t.Fatalf("contract marker position = %#v", ir.Markers[0].Position)
	}
	if len(ir.ContractSpans) != 1 || ir.ContractSpans[0].Token != "review" {
		t.Fatalf("contract spans = %#v", ir.ContractSpans)
	}
	if ir.ContractSpans[0].Close == nil || ir.ContractSpans[0].Close.Kind != ContractClose {
		t.Fatalf("contract close = %#v", ir.ContractSpans[0].Close)
	}
	if got := string(buffer[ir.ContractSpans[0].ContentRange.Start:ir.ContractSpans[0].ContentRange.End]); got != "first {{parent_agent}}\n<!-- @only codex -->\n<!-- @anchor gate -->\nsecond\n<!-- @/only -->\n" {
		t.Fatalf("contract content = %q", got)
	}
	if len(ir.OnlyBlocks) != 1 || ir.OnlyBlocks[0].Token != "codex" {
		t.Fatalf("only blocks = %#v", ir.OnlyBlocks)
	}
	if ir.OnlyBlocks[0].Close == nil || ir.OnlyBlocks[0].Close.Kind != OnlyClose {
		t.Fatalf("only close = %#v", ir.OnlyBlocks[0].Close)
	}
	if got := string(buffer[ir.OnlyBlocks[0].ContentRange.Start:ir.OnlyBlocks[0].ContentRange.End]); got != "<!-- @anchor gate -->\nsecond\n" {
		t.Fatalf("only content = %q", got)
	}
	if len(ir.Anchors) != 1 || ir.Anchors[0].Token != "gate" || ir.Anchors[0].Range != ir.Markers[2].Range {
		t.Fatalf("anchors = %#v", ir.Anchors)
	}
	termStart := strings.Index(string(buffer), "{{parent_agent}}")
	if len(ir.TermUses) != 1 || ir.TermUses[0].Name != "parent_agent" || ir.TermUses[0].Range != (source.Range{Start: termStart, End: termStart + len("{{parent_agent}}")}) {
		t.Fatalf("term uses = %#v", ir.TermUses)
	}
}

func TestLexRecognizesOnlyExactDirectiveLines(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{name: "missing whitespace after comment open", line: "<!--@contract x-->\n"},
		{name: "missing whitespace before comment close", line: "<!-- @contract x-->\n"},
		{name: "indented", line: " <!-- @contract x -->\n"},
		{name: "extra token", line: "<!-- @contract x extra -->\n"},
		{name: "unknown body", line: "<!-- @unknown x -->\n"},
		{name: "token containing whitespace", line: "<!-- @anchor x y -->\n"},
		{name: "suffix text", line: "<!-- @anchor x --> suffix\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ir, diagnostics := lexWhole(tt.line + "{{valid_name}}\n")
			if len(diagnostics) != 0 {
				t.Fatalf("Lex() diagnostics = %#v", diagnostics)
			}
			if len(ir.Markers) != 0 {
				t.Fatalf("markers = %#v, want none", ir.Markers)
			}
			if len(ir.TermUses) != 1 || ir.TermUses[0].Name != "valid_name" {
				t.Fatalf("term uses = %#v", ir.TermUses)
			}
		})
	}
}

func TestLexAcceptsDirectiveWhitespaceAtTheGrammarBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		input string
		kind  MarkerKind
		token string
	}{
		{name: "anchor with mixed whitespace", input: "<!--\t @anchor\t \tvalue \t-->\t \n", kind: AnchorMarker, token: "value"},
		{name: "contract with multiple spaces", input: "<!-- @contract   review -->\n<!-- @/contract -->\n", kind: ContractOpen, token: "review"},
		{name: "only with tab", input: "<!-- @only\tcodex -->\n<!-- @/only -->\n", kind: OnlyOpen, token: "codex"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ir, diagnostics := lexWhole(tt.input)
			if len(diagnostics) != 0 {
				t.Fatalf("Lex() diagnostics = %#v", diagnostics)
			}
			if len(ir.Markers) == 0 || ir.Markers[0].Kind != tt.kind || ir.Markers[0].Token != tt.token {
				t.Fatalf("markers = %#v, want first marker %q token %q", ir.Markers, tt.kind, tt.token)
			}
		})
	}
}

func TestLexFenceRecognitionBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantTerms []string
	}{
		{
			name:      "three backticks close with longer run",
			input:     "```go\n{{hidden}}\n```` \t\n{{shown}}\n",
			wantTerms: []string{"shown"},
		},
		{
			name:      "opening fence info is not scanned for terms",
			input:     "```lang {{hidden}}\nbody\n```\n{{shown}}\n",
			wantTerms: []string{"shown"},
		},
		{
			name:      "four tildes ignore shorter and different closings",
			input:     "~~~~ info\n```\n~~~\n{{hidden}}\n~~~~\n{{shown}}\n",
			wantTerms: []string{"shown"},
		},
		{
			name:      "closing fence is recognized after whitespace trim",
			input:     "```` info\n \t```\t \n \t~~~~ \n{{hidden}}\n \t````` \t\n{{shown}}\n",
			wantTerms: []string{"shown"},
		},
		{
			name:      "backtick in backtick info prevents opening",
			input:     "```go`bad {{visible}}\n{{also_visible}}\n",
			wantTerms: []string{"visible", "also_visible"},
		},
		{
			name:      "EOF automatically terminates fence",
			input:     "~~~\n<!-- @anchor hidden -->\n{{hidden}}\n",
			wantTerms: nil,
		},
		{
			name:      "two fence characters are body",
			input:     "`` {{visible}}\n",
			wantTerms: []string{"visible"},
		},
		{
			name:      "opening fence remains line start only",
			input:     " ```\n{{visible}}\n",
			wantTerms: []string{"visible"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ir, diagnostics := lexWhole(tt.input)
			if len(diagnostics) != 0 {
				t.Fatalf("Lex() diagnostics = %#v", diagnostics)
			}
			if got := termNames(ir.TermUses); !equalStrings(got, tt.wantTerms) {
				t.Fatalf("term names = %#v, want %#v", got, tt.wantTerms)
			}
			if len(ir.Markers) != 0 {
				t.Fatalf("markers = %#v, want fence contents ignored", ir.Markers)
			}
		})
	}
}

func TestLexReportsDirectiveStackViolationsAtNormalizedPositions(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantCount   int
		wantOffset  int
		wantLine    int
		wantMessage string
	}{
		{name: "contract close without open", input: "body\n<!-- @/contract -->\n", wantCount: 1, wantOffset: 5, wantLine: 2, wantMessage: "without an open contract"},
		{name: "only close without open", input: "<!-- @/only -->\n", wantCount: 1, wantOffset: 0, wantLine: 1, wantMessage: "without an open only block"},
		{name: "contract inside only", input: "<!-- @only codex -->\n<!-- @contract x -->\n<!-- @/only -->\n", wantCount: 1, wantOffset: 21, wantLine: 2, wantMessage: "contract cannot open inside only"},
		{name: "nested contract", input: "<!-- @contract a -->\n<!-- @contract b -->\n<!-- @/contract -->\n", wantCount: 1, wantOffset: 21, wantLine: 2, wantMessage: "contract cannot nest"},
		{name: "nested only", input: "<!-- @only a -->\n<!-- @only b -->\n<!-- @/only -->\n", wantCount: 1, wantOffset: 17, wantLine: 2, wantMessage: "only block cannot nest"},
		{name: "contract close crosses only", input: "<!-- @contract a -->\n<!-- @only b -->\n<!-- @/contract -->\n", wantCount: 3, wantOffset: 38, wantLine: 3, wantMessage: "contract close crosses an open only block"},
		{name: "only close crosses contract", input: "<!-- @contract a -->\n<!-- @/only -->\n", wantCount: 2, wantOffset: 21, wantLine: 2, wantMessage: "only block close crosses an open contract"},
		{name: "contract unclosed at EOF", input: "line\n<!-- @contract a -->\n", wantCount: 1, wantOffset: 5, wantLine: 2, wantMessage: "contract is not closed before EOF"},
		{name: "only unclosed at EOF", input: "line\n<!-- @only codex -->\n", wantCount: 1, wantOffset: 5, wantLine: 2, wantMessage: "only block is not closed before EOF"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, diagnostics := lexWhole(tt.input)
			if len(diagnostics) != tt.wantCount {
				t.Fatalf("diagnostics = %#v, want %d", diagnostics, tt.wantCount)
			}
			if diagnostics[0].Path != "source.md" || diagnostics[0].Offset != tt.wantOffset || diagnostics[0].Line != tt.wantLine || diagnostics[0].Column != 1 || !strings.Contains(diagnostics[0].Message, tt.wantMessage) {
				t.Fatalf("first diagnostic = %#v, want offset %d line %d containing %q", diagnostics[0], tt.wantOffset, tt.wantLine, tt.wantMessage)
			}
		})
	}
}

func TestLexRecognizesOnlyValidTermTokensLeftToRight(t *testing.T) {
	input := "{{first}}{{second_name}} {{}} {{Bad}} {{bad-name}} {{bad name}} {{{nested}}} `{{inline}}` {{last}}\n"
	ir, diagnostics := lexWhole(input)
	if len(diagnostics) != 0 {
		t.Fatalf("Lex() diagnostics = %#v", diagnostics)
	}
	want := []string{"first", "second_name", "nested", "inline", "last"}
	if got := termNames(ir.TermUses); !equalStrings(got, want) {
		t.Fatalf("term names = %#v, want %#v", got, want)
	}
	for _, use := range ir.TermUses {
		if got := input[use.Range.Start:use.Range.End]; got != "{{"+use.Name+"}}" {
			t.Fatalf("term range %#v selects %q", use.Range, got)
		}
	}
}

func TestLexAppliesTheCompleteTermNameGrammar(t *testing.T) {
	input := "{{a0}} {{a1_b2}} {{_a}} {{a_}} {{a__b}}\n"
	ir, diagnostics := lexWhole(input)
	if len(diagnostics) != 0 {
		t.Fatalf("Lex() diagnostics = %#v", diagnostics)
	}
	if got := termNames(ir.TermUses); !equalStrings(got, []string{"a0", "a1_b2"}) {
		t.Fatalf("term names = %#v, want only valid boundary names", got)
	}
}

func TestLexDoesNotScanTermsInsideRecognizedDirectiveLines(t *testing.T) {
	ir, diagnostics := lexWhole("<!-- @anchor {{looks_like_term}} -->\n")
	if len(diagnostics) != 0 {
		t.Fatalf("Lex() diagnostics = %#v", diagnostics)
	}
	if len(ir.Anchors) != 1 || ir.Anchors[0].Token != "{{looks_like_term}}" {
		t.Fatalf("anchors = %#v", ir.Anchors)
	}
	if len(ir.TermUses) != 0 {
		t.Fatalf("term uses = %#v, want none", ir.TermUses)
	}
}

func TestLexScansTermsOnLiteralDirectiveLikeLines(t *testing.T) {
	ir, diagnostics := lexWhole(" <!-- @anchor ignored --> {{same_line_term}}\n")
	if len(diagnostics) != 0 {
		t.Fatalf("Lex() diagnostics = %#v", diagnostics)
	}
	if len(ir.Markers) != 0 {
		t.Fatalf("markers = %#v, want none", ir.Markers)
	}
	if len(ir.TermUses) != 1 || ir.TermUses[0].Name != "same_line_term" {
		t.Fatalf("term uses = %#v", ir.TermUses)
	}
}

func TestLexDoesNotRecognizeTermAcrossLines(t *testing.T) {
	ir, diagnostics := lexWhole("before {{split_name\n}} after\n")
	if len(diagnostics) != 0 {
		t.Fatalf("Lex() diagnostics = %#v", diagnostics)
	}
	if len(ir.TermUses) != 0 {
		t.Fatalf("term uses = %#v, want none", ir.TermUses)
	}
}

func TestLexPreservesUnresolvedTargetAndTermIdentifiers(t *testing.T) {
	input := "<!-- @anchor duplicated -->\n<!-- @anchor duplicated -->\n<!-- @only future-target -->\n{{undefined_term}}\n<!-- @/only -->\n"
	ir, diagnostics := lexWhole(input)
	if len(diagnostics) != 0 {
		t.Fatalf("Lex() diagnostics = %#v", diagnostics)
	}
	if len(ir.Anchors) != 2 || ir.Anchors[0].Token != "duplicated" || ir.Anchors[1].Token != "duplicated" {
		t.Fatalf("anchors = %#v", ir.Anchors)
	}
	if len(ir.OnlyBlocks) != 1 || ir.OnlyBlocks[0].Token != "future-target" {
		t.Fatalf("only blocks = %#v", ir.OnlyBlocks)
	}
	if len(ir.TermUses) != 1 || ir.TermUses[0].Name != "undefined_term" {
		t.Fatalf("term uses = %#v", ir.TermUses)
	}
}

func TestLexReportsByteBasedColumns(t *testing.T) {
	input := "é {{term}}\n"
	start := strings.Index(input, "{{")
	ir, diagnostics := lexWhole(input)
	if len(diagnostics) != 0 {
		t.Fatalf("Lex() diagnostics = %#v", diagnostics)
	}
	if len(ir.TermUses) != 1 {
		t.Fatalf("term uses = %#v", ir.TermUses)
	}
	if ir.TermUses[0].Position != (Position{Offset: start, Line: 1, Column: 4}) {
		t.Fatalf("term position = %#v, want offset %d line 1 byte column 4", ir.TermUses[0].Position, start)
	}
}

func TestLexScansOnlyTheDeclaredBodyRange(t *testing.T) {
	buffer := []byte("{{frontmatter_term}}\n<!-- @anchor body -->\n{{body_term}}\n")
	start := strings.Index(string(buffer), "<!-- @anchor")
	ir, diagnostics := Lex("source.md", buffer, source.Range{Start: start, End: len(buffer)})
	if len(diagnostics) != 0 {
		t.Fatalf("Lex() diagnostics = %#v", diagnostics)
	}
	if got := termNames(ir.TermUses); !equalStrings(got, []string{"body_term"}) {
		t.Fatalf("term names = %#v", got)
	}
	if len(ir.Anchors) != 1 || ir.Anchors[0].Position.Line != 2 {
		t.Fatalf("anchors = %#v", ir.Anchors)
	}
}

func TestLexOracleSourcesHaveNoSyntaxDiagnostics(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "oracle", "input")
	projectBytes, err := os.ReadFile(filepath.Join(root, "gunte.toml"))
	if err != nil {
		t.Fatal(err)
	}
	project, configDiagnostics := config.ParseProject("gunte.toml", projectBytes)
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
			document, sourceDiagnostics := source.Parse(relativePath, input)
			if len(sourceDiagnostics) != 0 {
				t.Fatalf("source.Parse() diagnostics = %#v", sourceDiagnostics)
			}
			ir, diagnostics := Lex(relativePath, document.Buffer, document.BodyRange)
			if len(diagnostics) != 0 {
				t.Fatalf("Lex() diagnostics = %#v", diagnostics)
			}
			for _, marker := range ir.Markers {
				if marker.Range.Start < document.BodyRange.Start || marker.Range.End > document.BodyRange.End {
					t.Fatalf("marker range %#v is outside body %#v", marker.Range, document.BodyRange)
				}
			}
			for _, use := range ir.TermUses {
				if got := string(document.Buffer[use.Range.Start:use.Range.End]); got != "{{"+use.Name+"}}" {
					t.Fatalf("term range %#v selects %q", use.Range, got)
				}
			}
		})
	}
}

func TestLexOracleInventoryMatchesThePinnedSourceNotation(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "oracle", "input")
	projectBytes, err := os.ReadFile(filepath.Join(root, "gunte.toml"))
	if err != nil {
		t.Fatal(err)
	}
	project, configDiagnostics := config.ParseProject("gunte.toml", projectBytes)
	if len(configDiagnostics) != 0 {
		t.Fatalf("ParseProject() diagnostics = %#v", configDiagnostics)
	}

	markerCounts := map[MarkerKind]int{}
	termCounts := map[string]int{}
	markerSequences := map[string][]string{}
	fileTermCounts := map[string]map[string]int{}
	for _, relativePath := range project.Sources.Files {
		input, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatal(err)
		}
		document, sourceDiagnostics := source.Parse(relativePath, input)
		if len(sourceDiagnostics) != 0 {
			t.Fatalf("source.Parse(%q) diagnostics = %#v", relativePath, sourceDiagnostics)
		}
		ir, diagnostics := Lex(relativePath, document.Buffer, document.BodyRange)
		if len(diagnostics) != 0 {
			t.Fatalf("Lex(%q) diagnostics = %#v", relativePath, diagnostics)
		}
		for _, marker := range ir.Markers {
			markerCounts[marker.Kind]++
			markerSequences[relativePath] = append(markerSequences[relativePath], string(marker.Kind)+":"+marker.Token)
		}
		for _, use := range ir.TermUses {
			termCounts[use.Name]++
			if fileTermCounts[relativePath] == nil {
				fileTermCounts[relativePath] = map[string]int{}
			}
			fileTermCounts[relativePath][use.Name]++
		}
	}

	wantMarkerCounts := map[MarkerKind]int{OnlyOpen: 21, OnlyClose: 21}
	if !equalMarkerCounts(markerCounts, wantMarkerCounts) {
		t.Fatalf("oracle marker counts = %#v, want %#v", markerCounts, wantMarkerCounts)
	}
	wantTermCounts := map[string]int{
		"parent_agent":           26,
		"reviewer_invocation":    4,
		"continuation_mechanism": 3,
		"new_worker_invocation":  2,
		"new_worker":             1,
	}
	if !equalCounts(termCounts, wantTermCounts) {
		t.Fatalf("oracle term counts = %#v, want %#v", termCounts, wantTermCounts)
	}

	implLead := "shared/skill/impl-lead/SKILL.md"
	wantImplLeadMarkers := []string{
		"only_open:claude", "only_close:",
		"only_open:codex", "only_close:",
		"only_open:codex", "only_close:",
	}
	if !equalStrings(markerSequences[implLead], wantImplLeadMarkers) {
		t.Fatalf("%s marker sequence = %#v, want %#v", implLead, markerSequences[implLead], wantImplLeadMarkers)
	}

	suiteScan := "shared/skill/test-audit/references/suite-scan.md"
	if !equalStrings(markerSequences[suiteScan], []string{"only_open:codex", "only_close:"}) {
		t.Fatalf("%s marker sequence = %#v", suiteScan, markerSequences[suiteScan])
	}
	wantSuiteScanTerms := map[string]int{"parent_agent": 7, "new_worker_invocation": 1, "continuation_mechanism": 1}
	if !equalCounts(fileTermCounts[suiteScan], wantSuiteScanTerms) {
		t.Fatalf("%s term counts = %#v, want %#v", suiteScan, fileTermCounts[suiteScan], wantSuiteScanTerms)
	}

	overEngineering := "shared/agents/over-engineering-reviewer.md"
	if fileTermCounts[overEngineering]["parent_agent"] != 3 {
		t.Fatalf("%s parent_agent count = %d, want 3", overEngineering, fileTermCounts[overEngineering]["parent_agent"])
	}
}

func lexWhole(input string) (IR, []source.Diagnostic) {
	return Lex("source.md", []byte(input), source.Range{Start: 0, End: len(input)})
}

func termNames(uses []TermUse) []string {
	names := make([]string, len(uses))
	for index, use := range uses {
		names[index] = use.Name
	}
	return names
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func equalCounts(got, want map[string]int) bool {
	if len(got) != len(want) {
		return false
	}
	for key, count := range want {
		if got[key] != count {
			return false
		}
	}
	return true
}

func equalMarkerCounts(got, want map[MarkerKind]int) bool {
	if len(got) != len(want) {
		return false
	}
	for key, count := range want {
		if got[key] != count {
			return false
		}
	}
	return true
}
