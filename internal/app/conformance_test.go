package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestOracleInputEmitsEveryFrozenArtifactByte(t *testing.T) {
	t.Parallel()
	root := conformanceRepoRoot(t)
	fixture := filepath.Join(root, "testdata", "oracle")
	projectRoot := t.TempDir()
	copyTree(t, filepath.Join(fixture, "input"), projectRoot)

	result := NewRunner(projectRoot).Run([]string{"emit"})
	if result.ExitCode != ExitSuccess || len(result.Diagnostics) != 0 {
		t.Fatalf("oracle emit = %#v", result)
	}

	wantPaths := readFixtureLines(t, filepath.Join(fixture, "ORACLE_OUTPUTS"))
	if len(wantPaths) != 77 {
		t.Fatalf("oracle output path count = %d, want 77", len(wantPaths))
	}
	if !sort.StringsAreSorted(wantPaths) {
		t.Fatal("oracle output paths are not sorted")
	}
	if len(uniqueStrings(wantPaths)) != len(wantPaths) {
		t.Fatal("oracle output paths contain duplicates")
	}

	gotPaths := emittedPaths(t, filepath.Join(projectRoot, "plugins"))
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("emitted paths = %#v, want %#v", gotPaths, wantPaths)
	}

	goldenRoot := filepath.Join(fixture, "golden")
	for _, relativePath := range wantPaths {
		want, err := os.ReadFile(filepath.Join(goldenRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(projectRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(got, want) {
			continue
		}
		offset := firstByteDifference(want, got)
		t.Fatalf("artifact %s differs at byte %d: golden=%s got=%s", relativePath, offset, byteWindow(want, offset), byteWindow(got, offset))
	}

	digestLines := readFixtureLines(t, filepath.Join(fixture, "DIGESTS"))
	if len(digestLines) != len(wantPaths) {
		t.Fatalf("digest line count = %d, want %d", len(digestLines), len(wantPaths))
	}
	digestPaths := make([]string, 0, len(digestLines))
	for _, line := range digestLines {
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			t.Fatalf("invalid digest line %q", line)
		}
		wantDigest, relativePath := parts[0], parts[1]
		digestPaths = append(digestPaths, relativePath)
		if _, err := hex.DecodeString(wantDigest); err != nil || len(wantDigest) != sha256.Size*2 {
			t.Fatalf("invalid digest for %s: %q", relativePath, wantDigest)
		}
		data, err := os.ReadFile(filepath.Join(projectRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatal(err)
		}
		gotDigest := fmt.Sprintf("%x", sha256.Sum256(data))
		if gotDigest != wantDigest {
			t.Fatalf("digest %s = %s, want %s", relativePath, gotDigest, wantDigest)
		}
	}
	if !reflect.DeepEqual(digestPaths, wantPaths) {
		t.Fatalf("digest paths = %#v, want %#v", digestPaths, wantPaths)
	}
}

func TestInvalidInputFixtureOnlyRequiresFailure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFixtureFile(t, root, "gunte.toml", `spec_version = 1
[project]
id = "fixture"
version = "v1"
[sources]
files = ["src/input.md"]
[targets.one]
output_root = "out"
[[targets.one.rules]]
match = "src/*.md"
path = "{1}.md"
profile = "markdown-v1"
`)
	writeFixtureFile(t, root, "contracts.toml", "[contracts]\n")
	writeFixtureFile(t, root, "src/input.md", "<!-- @contract unclosed -->\nbody\n")

	result := NewRunner(root).Run([]string{"emit"})
	if result.ExitCode == ExitSuccess {
		t.Fatalf("invalid input unexpectedly succeeded: %#v", result)
	}
}

func TestContractFixtureReportsExactlyThreeViolationKinds(t *testing.T) {
	t.Parallel()
	root := writeProject(t, projectFixture{
		targets: []targetFixture{{id: "one", root: "out"}},
		sourceBody: "<!-- @contract required -->\nabsent\n<!-- @/contract -->\n" +
			"<!-- @contract forbidden -->\nblocked\n<!-- @/contract -->\n" +
			"<!-- @anchor after -->\nlate\n<!-- @anchor before -->\nearly\n",
		contracts: "[contracts.need]\nkind = \"requires\"\nslice = \"required\"\npattern = \"wanted\"\napplies_to = [\"one\"]\n" +
			"[contracts.ban]\nkind = \"forbids\"\nslice = \"forbidden\"\npattern = \"blocked\"\napplies_to = [\"one\"]\n" +
			"[contracts.ordering]\nkind = \"order\"\nbefore = \"before\"\nafter = \"after\"\napplies_to = [\"one\"]\n",
	})

	result := NewRunner(root).Run([]string{"emit"})
	if result.ExitCode != ExitFailure {
		t.Fatalf("contract fixture exit = %d, want %d (result=%#v)", result.ExitCode, ExitFailure, result)
	}
	gotKinds := make([]string, len(result.Diagnostics))
	for index, diagnostic := range result.Diagnostics {
		gotKinds[index] = diagnostic.Kind
	}
	sort.Strings(gotKinds)
	wantKinds := []string{"forbids_violation", "order_violation", "requires_violation"}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("contract fixture kinds = %#v, want %#v (result=%#v)", gotKinds, wantKinds, result)
	}
}

func TestCheckFixtureReportsExactOutputMismatch(t *testing.T) {
	t.Parallel()
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out"}}})
	if result := NewRunner(root).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("fixture emit = %#v", result)
	}
	writeFixtureFile(t, root, "out/a.md", "changed\n")

	result := NewRunner(root).Run([]string{"check"})
	if result.ExitCode != ExitFailure || len(result.Diagnostics) != 1 || result.Diagnostics[0].Kind != "output_mismatch" {
		t.Fatalf("check fixture = %#v", result)
	}
}

func conformanceRepoRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../.."))
}

func copyTree(t *testing.T, sourceRoot, destinationRoot string) {
	t.Helper()
	err := filepath.Walk(sourceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		destination := filepath.Join(destinationRoot, relative)
		if info.IsDir() {
			return os.MkdirAll(destination, info.Mode().Perm())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		return os.WriteFile(destination, data, info.Mode().Perm())
	})
	if err != nil {
		t.Fatal(err)
	}
}

func writeFixtureFile(t *testing.T, root, relativePath, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFixtureLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

func emittedPaths(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(filepath.Dir(root), path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	return paths
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func firstByteDifference(want, got []byte) int {
	limit := len(want)
	if len(got) < limit {
		limit = len(got)
	}
	for index := 0; index < limit; index++ {
		if want[index] != got[index] {
			return index
		}
	}
	return limit
}

func byteWindow(data []byte, offset int) string {
	start := offset - 16
	if start < 0 {
		start = 0
	}
	end := offset + 32
	if end > len(data) {
		end = len(data)
	}
	return fmt.Sprintf("%x", data[start:end])
}
