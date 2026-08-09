package contract

import (
	"reflect"
	"testing"

	"github.com/akitanabe/gunte/internal/compile"
	"github.com/akitanabe/gunte/internal/config"
	"github.com/akitanabe/gunte/internal/serialize"
	"github.com/akitanabe/gunte/internal/source"
)

func TestMatchTokenBoundariesWhitespaceAndUnicode(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		pattern string
		want    bool
	}{
		{"exact", "a implementer b", "implementer", true},
		{"suffix rejected", "senior-implementer", "implementer", false},
		{"prefix rejected", "implementer-extra", "implementer", false},
		{"ascii prefix adjacent rejected", "ximplementer", "implementer", false},
		{"ascii suffix adjacent rejected", "implementerx", "implementer", false},
		{"overlap does not relax Match boundaries", "aaa", "aa", false},
		{"punctuation boundary", "(implementer)", "implementer", true},
		{"unicode boundary", "実装implementer者", "implementer", true},
		{"fold spaces", "a\t  implementer\nb", "a implementer b", true},
		{"case sensitive", "Implementer", "implementer", false},
		{"unicode exact", "β", "β", true},
		{"multiple matches", "reviewer reviewer", "reviewer", true},
		{"mixed pattern whitespace run", "a b", "a \t\n b", true},
		{"match at body start and end", "reviewer", "reviewer", true},
		{"NFC and NFD are not normalized", "e\u0301", "é", false},
		{"NFD and NFC are not normalized", "é", "e\u0301", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Match([]byte(test.body), test.pattern); got != test.want {
				t.Fatalf("Match(%q, %q) = %v, want %v", test.body, test.pattern, got, test.want)
			}
		})
	}
}

func TestCountMatchesIsLeftToRightNonOverlapping(t *testing.T) {
	if got := CountMatches([]byte("aa aa"), "aa"); got != 2 {
		t.Fatalf("CountMatches() = %d, want 2", got)
	}
	if got := CountMatches([]byte("aaa"), "aa"); got != 0 {
		t.Fatalf("boundary CountMatches() = %d, want 0", got)
	}
	if got := CountMatches([]byte(":::"), "::"); got != 1 {
		t.Fatalf("overlapping CountMatches() = %d, want 1", got)
	}
	if got := CountMatches([]byte("a\t  a"), "a a"); got != 1 {
		t.Fatalf("whitespace CountMatches() = %d, want 1", got)
	}
	if got := CountMatches([]byte("senior-implementer implementer"), "implementer"); got != 1 {
		t.Fatalf("boundary CountMatches() = %d, want 1", got)
	}
}

func TestEvaluateOccurrenceCountsSlicesAndSelectedArtifacts(t *testing.T) {
	position := config.ContractPosition{Path: "contracts.toml", Line: 1, Column: 1}
	expected := int64(2)
	spanBody := []byte("reviewer reviewer")
	span := serialize.Artifact{TargetID: "one", Path: "out/a.md", Bytes: spanBody, Contracts: []serialize.Declaration{{ID: "span", Source: compile.SourcePosition{Path: "src/a.md", Line: 3, Column: 1}, Emitted: true, ArtifactRange: source.Range{Start: 0, End: len(spanBody)}}}}
	slice := config.Contract{ID: "count-span", Kind: config.PredicateOccurrences, Slice: "span", Pattern: "reviewer", Count: &expected, AppliesTo: []string{"one"}, Position: position}
	result, diagnostics := Evaluate(config.ContractRegistry{Contracts: []config.Contract{slice}}, []serialize.Artifact{span})
	if len(diagnostics) != 0 || len(result.Violations) != 0 {
		t.Fatalf("slice occurrence = %#v, %#v", result, diagnostics)
	}
	artifacts := []serialize.Artifact{
		{TargetID: "one", Path: "out/b.md", Bytes: []byte("reviewer reviewer")},
		{TargetID: "one", SourcePath: "src/a.md", Path: "out/a.md", Bytes: []byte("reviewer")},
		{TargetID: "one", Path: "out/skip.md", Bytes: []byte("reviewer reviewer")},
	}
	artifactPredicate := config.Contract{ID: "count-artifact", Kind: config.PredicateOccurrences, Pattern: "reviewer", Paths: []string{"out/*.md"}, ExcludePaths: []string{"out/skip.md"}, Count: &expected, AppliesTo: []string{"one"}, Position: position}
	result, diagnostics = Evaluate(config.ContractRegistry{Contracts: []config.Contract{artifactPredicate}}, artifacts)
	if len(diagnostics) != 0 || len(result.Violations) != 1 || result.Violations[0].ArtifactPath != "out/a.md" || result.Violations[0].ActualCount == nil || *result.Violations[0].ActualCount != 1 || result.Violations[0].ExpectedCount == nil || *result.Violations[0].ExpectedCount != expected || len(result.Violations[0].RelatedSource) != 1 || result.Violations[0].RelatedSource[0].Path != "src/a.md" {
		t.Fatalf("artifact occurrence = %#v, %#v", result, diagnostics)
	}
}

func TestEvaluateOccurrenceSliceMismatchRetainsArtifactSourceAndCounts(t *testing.T) {
	position := config.ContractPosition{Path: "contracts.toml", Line: 7, Column: 3}
	expected := int64(2)
	artifact := serialize.Artifact{
		TargetID: "one",
		Path:     "out/a.md",
		Bytes:    []byte("reviewer"),
		Contracts: []serialize.Declaration{{
			ID:            "span",
			Source:        compile.SourcePosition{Path: "src/a.md", Offset: 42, Line: 4, Column: 5},
			Emitted:       true,
			ArtifactRange: source.Range{Start: 0, End: len("reviewer")},
		}},
	}
	predicate := config.Contract{ID: "count", Kind: config.PredicateOccurrences, Slice: "span", Pattern: "reviewer", Count: &expected, AppliesTo: []string{"one"}, Position: position}
	result, diagnostics := Evaluate(config.ContractRegistry{Contracts: []config.Contract{predicate}}, []serialize.Artifact{artifact})
	if len(diagnostics) != 0 || len(result.Violations) != 1 {
		t.Fatalf("slice occurrence mismatch = %#v, %#v", result, diagnostics)
	}
	actual := int64(1)
	want := Violation{
		Kind:          OccurrencesViolation,
		PredicateID:   "count",
		TargetID:      "one",
		ArtifactPath:  "out/a.md",
		Predicate:     position,
		RelatedSource: []compile.SourcePosition{{Path: "src/a.md", Offset: 42, Line: 4, Column: 5}},
		ActualCount:   &actual,
		ExpectedCount: &expected,
	}
	if !reflect.DeepEqual(result.Violations[0], want) {
		t.Fatalf("slice occurrence mismatch = %#v, want %#v", result.Violations[0], want)
	}
}

func TestEvaluateScopedForbidsAndOccurrencesReportOwnerOnlyZeroSelection(t *testing.T) {
	position := config.ContractPosition{Path: "contracts.toml", Line: 1, Column: 1}
	for _, predicate := range []config.Contract{
		{ID: "ban", Kind: config.PredicateForbids, Pattern: "x", Paths: []string{"missing/*.md"}, AppliesTo: []string{"one"}, Position: position},
		{ID: "count", Kind: config.PredicateOccurrences, Pattern: "x", Paths: []string{"missing/*.md"}, Count: int64PointerForTest(1), AppliesTo: []string{"one"}, Position: position},
	} {
		result, diagnostics := Evaluate(config.ContractRegistry{Contracts: []config.Contract{predicate}}, []serialize.Artifact{{TargetID: "one", Path: "out/a.md", Bytes: []byte("x")}})
		if len(result.Violations) != 0 || len(diagnostics) != 1 || diagnostics[0].Kind != InvalidPredicate || diagnostics[0].PredicateID != predicate.ID || diagnostics[0].TargetID != "" || diagnostics[0].ArtifactPath != "" {
			t.Fatalf("zero selection = %#v, %#v", result, diagnostics)
		}
	}
}

func TestZeroSelectionOwnerDiagnosticIsReportedOncePerPredicate(t *testing.T) {
	predicate := config.Contract{ID: "ban", Kind: config.PredicateForbids, Pattern: "x", Paths: []string{"missing/*.md"}, AppliesTo: []string{"one", "two"}}
	result, diagnostics := Evaluate(config.ContractRegistry{Contracts: []config.Contract{predicate}}, []serialize.Artifact{{TargetID: "one", Path: "out/a.md", Bytes: []byte("x")}, {TargetID: "two", Path: "out/b.md", Bytes: []byte("x")}})
	if len(result.Violations) != 0 || len(diagnostics) != 1 || diagnostics[0].PredicateID != predicate.ID {
		t.Fatalf("owner-only diagnostics = %#v, %#v", result, diagnostics)
	}
}

func TestOccurrenceSelectionPreservesArtifactOrderAndDeduplicatesPaths(t *testing.T) {
	expected := int64(2)
	predicate := config.Contract{ID: "count", Kind: config.PredicateOccurrences, Pattern: "x", Paths: []string{"out/*.md"}, ExcludePaths: []string{"out/c.md"}, Count: &expected, AppliesTo: []string{"one"}}
	artifacts := []serialize.Artifact{
		{TargetID: "one", Path: "out/b.md", Bytes: []byte("x x")},
		{TargetID: "one", Path: "out/a.md", Bytes: []byte("x")},
		{TargetID: "one", Path: "out/b.md", Bytes: []byte("x")},
		{TargetID: "one", Path: "out/c.md", Bytes: []byte("x")},
	}
	result, diagnostics := Evaluate(config.ContractRegistry{Contracts: []config.Contract{predicate}}, artifacts)
	if len(diagnostics) != 0 || len(result.Violations) != 1 || result.Violations[0].ArtifactPath != "out/a.md" {
		t.Fatalf("selection = %#v, %#v", result, diagnostics)
	}
}

func TestScopedSelectionDistinguishesPositiveMatchFromFinalSelection(t *testing.T) {
	position := config.ContractPosition{Path: "contracts.toml", Line: 1, Column: 1}
	artifacts := []serialize.Artifact{{TargetID: "one", Path: "out/a.md", Bytes: []byte("x")}}
	for _, test := range []struct {
		name      string
		predicate config.Contract
		wantDiag  bool
	}{
		{name: "positive selector has no match", predicate: config.Contract{ID: "ban", Kind: config.PredicateForbids, Pattern: "x", Paths: []string{"missing/*.md"}, AppliesTo: []string{"one"}, Position: position}, wantDiag: true},
		{name: "positive match excluded", predicate: config.Contract{ID: "ban", Kind: config.PredicateForbids, Pattern: "x", Paths: []string{"out/*.md"}, ExcludePaths: []string{"out/*.md"}, AppliesTo: []string{"one"}, Position: position}, wantDiag: false},
		{name: "all artifacts excluded without positive selector", predicate: config.Contract{ID: "ban", Kind: config.PredicateForbids, Pattern: "x", ExcludePaths: []string{"out/*.md"}, AppliesTo: []string{"one"}, Position: position}, wantDiag: false},
		{name: "occurrence final selection empty", predicate: config.Contract{ID: "count", Kind: config.PredicateOccurrences, Pattern: "x", Paths: []string{"out/*.md"}, ExcludePaths: []string{"out/*.md"}, Count: int64PointerForTest(0), AppliesTo: []string{"one"}, Position: position}, wantDiag: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, diagnostics := Evaluate(config.ContractRegistry{Contracts: []config.Contract{test.predicate}}, artifacts)
			if len(result.Violations) != 0 || (len(diagnostics) != 0) != test.wantDiag {
				t.Fatalf("selection result = %#v, diagnostics = %#v", result, diagnostics)
			}
		})
	}
}

func int64PointerForTest(value int64) *int64 { return &value }

func TestEvaluateReportsEachViolationKindWithCompleteData(t *testing.T) {
	position := config.ContractPosition{Path: "contracts.toml", Line: 4, Column: 1}
	tests := []struct {
		name      string
		predicate config.Contract
		artifact  serialize.Artifact
		want      Violation
	}{
		{
			name:      "requires",
			predicate: config.Contract{ID: "need", Kind: config.PredicateRequires, Slice: "span", Pattern: "reviewer", AppliesTo: []string{"one"}, Position: position},
			artifact:  serialize.Artifact{TargetID: "one", Path: "out/requires.md", Bytes: []byte("absent"), Contracts: []serialize.Declaration{{ID: "span", Source: compile.SourcePosition{Path: "source.md", Offset: 10, Line: 2, Column: 3}, Emitted: true, ArtifactRange: source.Range{Start: 0, End: 6}}}},
			want:      Violation{Kind: RequiresViolation, PredicateID: "need", TargetID: "one", ArtifactPath: "out/requires.md", Predicate: position, RelatedSource: []compile.SourcePosition{{Path: "source.md", Offset: 10, Line: 2, Column: 3}}},
		},
		{
			name:      "forbids slice",
			predicate: config.Contract{ID: "ban-slice", Kind: config.PredicateForbids, Slice: "span", Pattern: "reviewer", AppliesTo: []string{"one"}, Position: position},
			artifact:  serialize.Artifact{TargetID: "one", Path: "out/forbids-slice.md", Bytes: []byte("reviewer"), Contracts: []serialize.Declaration{{ID: "span", Source: compile.SourcePosition{Path: "source.md", Offset: 20, Line: 3, Column: 2}, Emitted: true, ArtifactRange: source.Range{Start: 0, End: 8}}}},
			want:      Violation{Kind: ForbidsViolation, PredicateID: "ban-slice", TargetID: "one", ArtifactPath: "out/forbids-slice.md", Predicate: position, RelatedSource: []compile.SourcePosition{{Path: "source.md", Offset: 20, Line: 3, Column: 2}}},
		},
		{
			name:      "forbids global",
			predicate: config.Contract{ID: "ban-global", Kind: config.PredicateForbids, Pattern: "reviewer", AppliesTo: []string{"one"}, Position: position},
			artifact:  serialize.Artifact{TargetID: "one", Path: "out/forbids-global.md", Bytes: []byte("reviewer")},
			want:      Violation{Kind: ForbidsViolation, PredicateID: "ban-global", TargetID: "one", ArtifactPath: "out/forbids-global.md", Predicate: position},
		},
		{
			name:      "order",
			predicate: config.Contract{ID: "order", Kind: config.PredicateOrder, Before: "after", After: "before", AppliesTo: []string{"one"}, Position: position},
			artifact: serialize.Artifact{TargetID: "one", Path: "out/order.md", Bytes: []byte("before after"), Contracts: []serialize.Declaration{
				{ID: "before", Source: compile.SourcePosition{Path: "source.md", Offset: 31, Line: 4, Column: 2}, Emitted: true, ArtifactRange: source.Range{Start: 0, End: 6}},
				{ID: "after", Source: compile.SourcePosition{Path: "source.md", Offset: 40, Line: 5, Column: 2}, Emitted: true, ArtifactRange: source.Range{Start: 7, End: 12}},
			}},
			want: Violation{Kind: OrderViolation, PredicateID: "order", TargetID: "one", ArtifactPath: "out/order.md", Predicate: position, RelatedSource: []compile.SourcePosition{{Path: "source.md", Offset: 40, Line: 5, Column: 2}, {Path: "source.md", Offset: 31, Line: 4, Column: 2}}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, diagnostics := Evaluate(config.ContractRegistry{Contracts: []config.Contract{test.predicate}}, []serialize.Artifact{test.artifact})
			if len(diagnostics) != 0 {
				t.Fatalf("diagnostics = %#v", diagnostics)
			}
			if len(result.Violations) != 1 || !reflect.DeepEqual(result.Violations[0], test.want) {
				t.Fatalf("violations = %#v, want %#v", result.Violations, test.want)
			}
		})
	}
}

func TestEvaluateRequiresAndForbidsDistinguishEmptyAndNonMatchingSlices(t *testing.T) {
	position := config.ContractPosition{Path: "contracts.toml", Line: 1, Column: 1}
	span := func(body string) serialize.Artifact {
		return serialize.Artifact{TargetID: "one", Path: "out.md", Bytes: []byte(body), Contracts: []serialize.Declaration{{ID: "span", Emitted: true, ArtifactRange: source.Range{Start: 0, End: len([]byte(body))}}}}
	}
	tests := []struct {
		name      string
		predicate config.Contract
		body      string
		violates  bool
	}{
		{"requires non-empty non-match", config.Contract{ID: "need", Kind: config.PredicateRequires, Slice: "span", Pattern: "wanted", AppliesTo: []string{"one"}, Position: position}, "present", true},
		{"forbids non-empty non-match", config.Contract{ID: "ban", Kind: config.PredicateForbids, Slice: "span", Pattern: "wanted", AppliesTo: []string{"one"}, Position: position}, "present", false},
		{"forbids global match", config.Contract{ID: "ban-global", Kind: config.PredicateForbids, Pattern: "wanted", AppliesTo: []string{"one"}, Position: position}, "wanted", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, diagnostics := Evaluate(config.ContractRegistry{Contracts: []config.Contract{test.predicate}}, []serialize.Artifact{span(test.body)})
			wantViolations := 0
			if test.violates {
				wantViolations = 1
			}
			if len(diagnostics) != 0 || len(result.Violations) != wantViolations {
				t.Fatalf("result = %#v diagnostics = %#v, violates=%v", result, diagnostics, test.violates)
			}
		})
	}
}

func TestEvaluatePredicateSemanticsTable(t *testing.T) {
	position := config.ContractPosition{Path: "contracts.toml", Line: 3, Column: 1}
	span := func(id, body string, start int) serialize.Artifact {
		return serialize.Artifact{TargetID: "one", Path: id + ".md", Bytes: []byte(body), Contracts: []serialize.Declaration{{ID: id, Source: compile.SourcePosition{Path: "source.md", Offset: 2, Line: 1, Column: 3}, Emitted: true, ArtifactRange: source.Range{Start: start, End: start + len(body)}}}}
	}
	tests := []struct {
		name       string
		predicate  config.Contract
		artifacts  []serialize.Artifact
		violations int
	}{
		{"requires match", config.Contract{ID: "need", Kind: config.PredicateRequires, Slice: "span", Pattern: "reviewer", AppliesTo: []string{"one"}, Position: position}, []serialize.Artifact{span("span", "reviewer", 0)}, 0},
		{"requires empty", config.Contract{ID: "need", Kind: config.PredicateRequires, Slice: "span", Pattern: "reviewer", AppliesTo: []string{"one"}, Position: position}, []serialize.Artifact{span("span", " \t\n", 0)}, 1},
		{"forbids slice match", config.Contract{ID: "ban", Kind: config.PredicateForbids, Slice: "span", Pattern: "reviewer", AppliesTo: []string{"one"}, Position: position}, []serialize.Artifact{span("span", "reviewer", 0)}, 1},
		{"forbids slice empty", config.Contract{ID: "ban", Kind: config.PredicateForbids, Slice: "span", Pattern: "reviewer", AppliesTo: []string{"one"}, Position: position}, []serialize.Artifact{span("span", "\n", 0)}, 1},
		{"forbids global artifact boundary", config.Contract{ID: "ban", Kind: config.PredicateForbids, Pattern: "ab", AppliesTo: []string{"one"}, Position: position}, []serialize.Artifact{{TargetID: "one", Path: "a", Bytes: []byte("a")}, {TargetID: "one", Path: "b", Bytes: []byte("b")}}, 0},
		{"forbids global no artifacts", config.Contract{ID: "ban", Kind: config.PredicateForbids, Pattern: "bad", AppliesTo: []string{"one"}, Position: position}, nil, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, diagnostics := Evaluate(config.ContractRegistry{Contracts: []config.Contract{test.predicate}}, test.artifacts)
			if len(diagnostics) != 0 {
				t.Fatalf("diagnostics = %#v", diagnostics)
			}
			if len(result.Violations) != test.violations {
				t.Fatalf("violations = %#v, want %d", result.Violations, test.violations)
			}
		})
	}
}

func TestEvaluateOrderRequiresSameArtifactAndIncreasingOffsets(t *testing.T) {
	position := config.ContractPosition{Path: "contracts.toml", Line: 1, Column: 1}
	artifact := serialize.Artifact{TargetID: "one", Path: "out.md", Bytes: []byte("before after"), Contracts: []serialize.Declaration{
		{ID: "before", Source: compile.SourcePosition{Path: "source.md", Offset: 1}, Emitted: true, ArtifactRange: source.Range{Start: 0, End: 6}},
		{ID: "after", Source: compile.SourcePosition{Path: "source.md", Offset: 8}, Emitted: true, ArtifactRange: source.Range{Start: 7, End: 12}},
	}}
	base := config.Contract{ID: "order", Kind: config.PredicateOrder, Before: "before", After: "after", AppliesTo: []string{"one"}, Position: position}
	result, diagnostics := Evaluate(config.ContractRegistry{Contracts: []config.Contract{base}}, []serialize.Artifact{artifact})
	if len(diagnostics) != 0 || len(result.Violations) != 0 {
		t.Fatalf("valid order = %#v %#v", result, diagnostics)
	}
	artifact.Contracts[0].ArtifactRange = source.Range{Start: 9, End: 12}
	result, diagnostics = Evaluate(config.ContractRegistry{Contracts: []config.Contract{base}}, []serialize.Artifact{artifact})
	if len(diagnostics) != 0 || len(result.Violations) != 1 || result.Violations[0].Kind != OrderViolation {
		t.Fatalf("reverse order = %#v %#v", result, diagnostics)
	}
	other := serialize.Artifact{TargetID: "one", Path: "out.md", Bytes: []byte("after"), Contracts: []serialize.Declaration{{ID: "after", Source: compile.SourcePosition{Path: "source.md", Offset: 8}, Emitted: true, ArtifactRange: source.Range{Start: 0, End: 5}}}}
	firstOnly := serialize.Artifact{TargetID: "one", Path: "out.md", Bytes: []byte("before"), Contracts: []serialize.Declaration{{ID: "before", Source: compile.SourcePosition{Path: "source.md", Offset: 1}, Emitted: true, ArtifactRange: source.Range{Start: 0, End: 6}}}}
	result, diagnostics = Evaluate(config.ContractRegistry{Contracts: []config.Contract{base}}, []serialize.Artifact{firstOnly, other})
	if len(diagnostics) != 0 || len(result.Violations) != 1 {
		t.Fatalf("cross artifact = %#v %#v", result, diagnostics)
	}
	anchorArtifact := serialize.Artifact{TargetID: "one", Path: "anchors.md", Bytes: []byte("before after"), Anchors: []serialize.Declaration{
		{ID: "before", Source: compile.SourcePosition{Path: "source.md", Offset: 1}, Emitted: true, ArtifactRange: source.Range{Start: 0, End: 0}},
		{ID: "after", Source: compile.SourcePosition{Path: "source.md", Offset: 8}, Emitted: true, ArtifactRange: source.Range{Start: 7, End: 7}},
	}}
	result, diagnostics = Evaluate(config.ContractRegistry{Contracts: []config.Contract{base}}, []serialize.Artifact{anchorArtifact})
	if len(diagnostics) != 0 || len(result.Violations) != 0 {
		t.Fatalf("valid anchor order = %#v %#v", result, diagnostics)
	}
	anchorArtifact.Anchors[1].ArtifactRange.Start = 0
	result, diagnostics = Evaluate(config.ContractRegistry{Contracts: []config.Contract{base}}, []serialize.Artifact{anchorArtifact})
	if len(diagnostics) != 0 || len(result.Violations) != 1 || result.Violations[0].Kind != OrderViolation {
		t.Fatalf("same-offset anchor order = %#v %#v", result, diagnostics)
	}
}

func TestEvaluateReportsUnresolvedReferencesAndInvalidRanges(t *testing.T) {
	position := config.ContractPosition{Path: "contracts.toml", Line: 4, Column: 1}
	anchorOnly := serialize.Artifact{TargetID: "one", Path: "out.md", Bytes: []byte("body"), Anchors: []serialize.Declaration{{ID: "anchor", Source: compile.SourcePosition{Path: "source.md", Offset: 2}, Emitted: true, ArtifactRange: source.Range{Start: 1, End: 1}}}}
	registry := config.ContractRegistry{Contracts: []config.Contract{{ID: "need", Kind: config.PredicateRequires, Slice: "anchor", Pattern: "x", AppliesTo: []string{"one"}, Position: position}, {ID: "missing", Kind: config.PredicateForbids, Slice: "span", Pattern: "x", AppliesTo: []string{"one"}, Position: position}}}
	result, diagnostics := Evaluate(registry, []serialize.Artifact{anchorOnly})
	if len(result.Violations) != 0 || len(diagnostics) != 2 || diagnostics[0].Kind != ReferenceKindMismatch || diagnostics[1].Kind != UnresolvedReference {
		t.Fatalf("reference diagnostics = %#v %#v", result, diagnostics)
	}
	hidden := anchorOnly
	hidden.Anchors = append([]serialize.Declaration(nil), anchorOnly.Anchors...)
	hidden.Anchors[0].Emitted = false
	hidden.Anchors[0].ArtifactRange = source.Range{}
	_, diagnostics = Evaluate(config.ContractRegistry{Contracts: []config.Contract{{ID: "order", Kind: config.PredicateOrder, Before: "anchor", After: "anchor", AppliesTo: []string{"one"}, Position: position}}}, []serialize.Artifact{hidden})
	if len(diagnostics) != 2 || diagnostics[0].Kind != UnresolvedReference || diagnostics[1].Kind != UnresolvedReference {
		t.Fatalf("non-emitted references = %#v", diagnostics)
	}
	bad := anchorOnly
	bad.Anchors[0].ArtifactRange = source.Range{Start: 1, End: 99}
	_, diagnostics = Evaluate(config.ContractRegistry{Contracts: []config.Contract{{ID: "need", Kind: config.PredicateRequires, Slice: "anchor", Pattern: "x", AppliesTo: []string{"one"}, Position: position}}}, []serialize.Artifact{bad})
	if len(diagnostics) != 1 || diagnostics[0].Kind != InvalidArtifactRange {
		t.Fatalf("range diagnostics = %#v", diagnostics)
	}
}

func TestEvaluateSelectedTargetsReturnsResultsAndDiagnosticsOnlyForSelectedTargets(t *testing.T) {
	position := config.ContractPosition{Path: "contracts.toml", Line: 1, Column: 1}
	registry := config.ContractRegistry{Contracts: []config.Contract{{ID: "need", Kind: config.PredicateRequires, Slice: "span", Pattern: "x", AppliesTo: []string{"one", "two"}, Position: position}}}
	artifact := serialize.Artifact{TargetID: "one", Path: "one.md", Bytes: []byte("x"), Contracts: []serialize.Declaration{{ID: "span", Emitted: true, ArtifactRange: source.Range{Start: 0, End: 1}}}}
	result, diagnostics := EvaluateTargets(registry, []serialize.Artifact{artifact}, []string{"one"})
	if len(diagnostics) != 0 || len(result.Violations) != 0 {
		t.Fatalf("selected target = %#v %#v", result, diagnostics)
	}
	_, diagnostics = EvaluateTargets(registry, []serialize.Artifact{artifact}, nil)
	if len(diagnostics) != 1 || diagnostics[0].Kind != UnresolvedReference || diagnostics[0].TargetID != "two" {
		t.Fatalf("all target diagnostics = %#v", diagnostics)
	}
	badUnselected := serialize.Artifact{TargetID: "two", Path: "two.md", Bytes: []byte("body"), Anchors: []serialize.Declaration{{ID: "bad", Emitted: true, ArtifactRange: source.Range{Start: 1, End: 99}}}}
	result, diagnostics = EvaluateTargets(registry, []serialize.Artifact{artifact, badUnselected}, []string{"one"})
	if len(diagnostics) != 0 || len(result.Violations) != 0 {
		t.Fatalf("unselected invalid artifact = %#v %#v", result, diagnostics)
	}
}

func TestEvaluateSelectedTargetReportsItsViolationAndOtherTargetReference(t *testing.T) {
	position := config.ContractPosition{Path: "contracts.toml", Line: 2, Column: 1}
	registry := config.ContractRegistry{Contracts: []config.Contract{{ID: "need", Kind: config.PredicateRequires, Slice: "span", Pattern: "wanted", AppliesTo: []string{"one", "two"}, Position: position}}}
	artifact := serialize.Artifact{TargetID: "one", Path: "one.md", Bytes: []byte("bad"), Contracts: []serialize.Declaration{{ID: "span", Emitted: true, ArtifactRange: source.Range{Start: 0, End: 3}}}}
	result, diagnostics := EvaluateTargets(registry, []serialize.Artifact{artifact}, []string{"one"})
	if len(diagnostics) != 0 || len(result.Violations) != 1 || result.Violations[0].TargetID != "one" {
		t.Fatalf("selected violation = %#v %#v", result, diagnostics)
	}
	result, diagnostics = EvaluateTargets(registry, []serialize.Artifact{artifact}, []string{"two"})
	if len(result.Violations) != 0 || len(diagnostics) != 1 || diagnostics[0].Kind != UnresolvedReference || diagnostics[0].TargetID != "two" {
		t.Fatalf("selected unresolved = %#v %#v", result, diagnostics)
	}
}

func TestEvaluateEmittedFalseContractReportsUnresolvedData(t *testing.T) {
	position := config.ContractPosition{Path: "contracts.toml", Line: 7, Column: 1}
	sourcePosition := compile.SourcePosition{Path: "source.md", Offset: 11, Line: 2, Column: 4}
	registry := config.ContractRegistry{Contracts: []config.Contract{{ID: "need", Kind: config.PredicateRequires, Slice: "span", Pattern: "wanted", AppliesTo: []string{"one"}, Position: position}}}
	artifacts := []serialize.Artifact{{TargetID: "one", Path: "out.md", Bytes: []byte("body"), Contracts: []serialize.Declaration{{ID: "span", Source: sourcePosition, Emitted: false}}}}
	result, diagnostics := Evaluate(registry, artifacts)
	want := Diagnostic{Kind: UnresolvedReference, PredicateID: "need", TargetID: "one", Predicate: position, RelatedSource: []compile.SourcePosition{sourcePosition}, Message: "reference span is not emitted for target"}
	if len(result.Violations) != 0 || len(diagnostics) != 1 || !reflect.DeepEqual(diagnostics[0], want) {
		t.Fatalf("emitted false = %#v %#v, want %#v", result, diagnostics, want)
	}
}

func TestEvaluateDoesNotMutateInputs(t *testing.T) {
	position := config.ContractPosition{Path: "contracts.toml", Line: 1, Column: 1}
	registry := config.ContractRegistry{Contracts: []config.Contract{{ID: "ban", Kind: config.PredicateForbids, Pattern: "x", AppliesTo: []string{"one"}, Position: position}}}
	artifacts := []serialize.Artifact{{TargetID: "one", Path: "one.md", Bytes: []byte("x")}}
	beforeRegistry := registry
	beforeArtifacts := append([]serialize.Artifact(nil), artifacts...)
	beforeArtifacts[0].Bytes = append([]byte(nil), artifacts[0].Bytes...)
	_, _ = Evaluate(registry, artifacts)
	if !reflect.DeepEqual(registry, beforeRegistry) || !reflect.DeepEqual(artifacts, beforeArtifacts) {
		t.Fatalf("Evaluate mutated input")
	}
}
