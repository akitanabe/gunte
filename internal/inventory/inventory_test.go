package inventory

import (
	"reflect"
	"testing"
)

func TestCompareUsesSegmentBoundariesAndAllowsExpectedAndExplicitPaths(t *testing.T) {
	scope := Scope{Root: "src", AllowFiles: []string{"src/keep.txt"}, AllowDirs: []string{"src/generated"}}
	entries := []Entry{
		{Path: "src", Kind: KindDir},
		{Path: "src/a.md", Kind: KindFile},
		{Path: "src/generated", Kind: KindDir},
		{Path: "src/generated/output", Kind: KindFile},
		{Path: "src/keep.txt", Kind: KindSymlink},
		{Path: "src/extra", Kind: KindDir},
		{Path: "src/extra/empty", Kind: KindDir},
		{Path: "src-other/file", Kind: KindFile},
	}
	got := Compare(scope, []string{"src/a.md"}, entries)
	want := []Diagnostic{
		{Path: "src/extra", Message: "path is outside expected and allowed inventory"},
		{Path: "src/extra/empty", Message: "path is outside expected and allowed inventory"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Compare() = %#v, want %#v", got, want)
	}
}

func TestCompareTreatsSymlinkDirectoryAsLeaf(t *testing.T) {
	scope := Scope{Root: "out", AllowDirs: []string{"out/allowed"}}
	entries := []Entry{
		{Path: "out", Kind: KindDir},
		{Path: "out/allowed", Kind: KindDir},
		{Path: "out/allowed/link", Kind: KindSymlink},
	}
	got := Compare(scope, nil, entries)
	if len(got) != 0 {
		t.Fatalf("symlink descendant diagnostics = %#v, want none", got)
	}

	exactSymlink := Compare(
		Scope{Root: "out", AllowDirs: []string{"out/allowed"}},
		nil,
		[]Entry{{Path: "out", Kind: KindDir}, {Path: "out/allowed", Kind: KindSymlink}},
	)
	if len(exactSymlink) != 1 || exactSymlink[0].Path != "out/allowed" {
		t.Fatalf("allow directory symlink diagnostics = %#v", exactSymlink)
	}
}

func TestCompareAllowsMissingExpectedAndAllowEntriesToBeHandledElsewhere(t *testing.T) {
	scope := Scope{Root: "src", AllowFiles: []string{"src/missing"}, AllowDirs: []string{"src/optional"}}
	if got := Compare(scope, []string{"src/expected"}, []Entry{{Path: "src", Kind: KindDir}}); len(got) != 0 {
		t.Fatalf("missing path diagnostics = %#v", got)
	}
}
