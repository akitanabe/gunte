package gunte_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/akitanabe/gunte/internal/config"
)

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertDigest(t *testing.T, data []byte, want string) {
	t.Helper()
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != want {
		t.Fatalf("SHA-256 = %s, want %s", got, want)
	}
}

func nonemptyLines(data []byte) []string {
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func assertSortedUniqueCount(t *testing.T, values []string, wantCount int) {
	t.Helper()
	if len(values) != wantCount {
		t.Fatalf("value count = %d, want %d", len(values), wantCount)
	}
	if !sort.StringsAreSorted(values) {
		t.Fatal("values are not sorted")
	}
	assertUnique(t, values)
}

func assertUnique(t *testing.T, values []string) {
	t.Helper()
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if seen[value] {
			t.Fatalf("duplicate value %q", value)
		}
		seen[value] = true
	}
}

func assertGoldenDigests(t *testing.T, golden string, lines []string) []string {
	t.Helper()
	paths := make([]string, 0, len(lines))
	seen := make(map[string]bool, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			t.Fatalf("invalid digest line %q", line)
		}
		wantDigest, path := parts[0], parts[1]
		decoded, err := hex.DecodeString(wantDigest)
		if err != nil || len(decoded) != sha256.Size {
			t.Fatalf("invalid digest for %s: %q", path, wantDigest)
		}
		if seen[path] {
			t.Fatalf("duplicate digest path %q", path)
		}
		seen[path] = true
		assertDigest(t, readFile(t, filepath.Join(golden, filepath.FromSlash(path))), wantDigest)
		paths = append(paths, path)
	}
	return paths
}

func regularFiles(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
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

func derivedOutputPaths(t *testing.T, input string) []string {
	t.Helper()
	paths := []string{
		"plugins/claude/.claude-plugin/plugin.json",
		"plugins/codex/.codex-plugin/plugin.json",
		"plugins/codex/install/VERSION",
	}
	for _, path := range regularFiles(t, filepath.Join(input, "shared", "agents")) {
		if filepath.Ext(path) != ".md" {
			continue
		}
		paths = append(paths,
			"plugins/claude/agents/"+path,
			"plugins/codex/install/agents/"+strings.TrimSuffix(path, ".md")+".toml",
		)
	}
	for _, path := range regularFiles(t, filepath.Join(input, "shared", "skill")) {
		if filepath.Ext(path) != ".md" {
			continue
		}
		paths = append(paths,
			"plugins/claude/skills/"+path,
			"plugins/codex/skills/"+path,
		)
	}
	sort.Strings(paths)
	return paths
}

func assertTreeDigest(t *testing.T, root, want string) {
	t.Helper()
	digest := sha256.New()
	for _, path := range regularFiles(t, root) {
		digest.Write([]byte(path))
		digest.Write([]byte{0})
		digest.Write(readFile(t, filepath.Join(root, filepath.FromSlash(path))))
		digest.Write([]byte{0})
	}
	if got := hex.EncodeToString(digest.Sum(nil)); got != want {
		t.Fatalf("tree SHA-256 = %s, want %s", got, want)
	}
}

func assertSourceDirectives(t *testing.T, input string) {
	t.Helper()
	legacyMarker := regexp.MustCompile(`<!--[ \t]*(claude|codex)-only[ \t]*:[ \t]*(start|end)[ \t]*-->`)
	for _, path := range regularFiles(t, input) {
		if legacyMarker.Match(readFile(t, filepath.Join(input, filepath.FromSlash(path)))) {
			t.Errorf("legacy marker remains in %s", path)
		}
	}
	expected := map[string]int{
		"<!-- @only claude -->": 9,
		"<!-- @only codex -->":  12,
		"<!-- @/only -->":       21,
	}
	shared := filepath.Join(input, "shared")
	for marker, want := range expected {
		got := 0
		for _, path := range regularFiles(t, shared) {
			got += bytes.Count(readFile(t, filepath.Join(shared, filepath.FromSlash(path))), []byte(marker))
		}
		if got != want {
			t.Errorf("directive %q count = %d, want %d", marker, got, want)
		}
	}
}

func assertEqualStrings(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("values = %#v, want %#v", got, want)
	}
}

func removeString(values []string, remove string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != remove {
			result = append(result, value)
		}
	}
	return result
}

func projectTerms(project config.ProjectConfig) map[string]map[string]string {
	terms := make(map[string]map[string]string, len(project.Terms))
	for _, term := range project.Terms {
		values := make(map[string]string, len(term.Values))
		for _, value := range term.Values {
			values[value.TargetID] = value.Value
		}
		terms[term.Name] = values
	}
	return terms
}

func assertOracleProfilesAndFrontmatter(t *testing.T, project config.ProjectConfig) {
	t.Helper()
	profiles := map[string]bool{}
	frontmatter := map[string]bool{}
	hasPlainToken := false
	for _, target := range project.Targets {
		for _, rule := range target.Rules {
			profiles[string(rule.Profile)] = true
			for _, metadata := range rule.Metadata {
				if strings.HasPrefix(metadata.From, "frontmatter:") {
					frontmatter[metadata.From] = true
				}
				if metadata.Type == config.MetadataPlainToken &&
					(metadata.From == "frontmatter:claude.model" || metadata.From == "frontmatter:claude.effort") {
					hasPlainToken = true
				}
			}
		}
	}
	assertEqualStrings(t, sortedBoolKeys(profiles), []string{
		"json-v1",
		"markdown+yaml-frontmatter-v1",
		"markdown-v1",
		"plain-text-v1",
		"toml-v1",
	})
	if len(frontmatter) == 0 || !frontmatter["frontmatter:claude.description"] || !frontmatter["frontmatter:codex.description"] {
		t.Errorf("frontmatter references = %#v", frontmatter)
	}
	for reference := range frontmatter {
		if !strings.Contains(reference, ".") {
			t.Errorf("frontmatter reference has no dotted field: %q", reference)
		}
	}
	if !hasPlainToken {
		t.Error("oracle has no plain_token model or effort metadata")
	}
}

func sortedBoolKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sectionBetween(t *testing.T, document, start, end string) string {
	t.Helper()
	startIndex := strings.Index(document, start)
	if startIndex < 0 {
		t.Fatalf("document has no %q section", start)
	}
	endIndex := strings.Index(document[startIndex+len(start):], end)
	if endIndex < 0 {
		t.Fatalf("document has no %q section after %q", end, start)
	}
	return document[startIndex+len(start) : startIndex+len(start)+endIndex]
}

func sectionAfter(t *testing.T, document, start string) string {
	t.Helper()
	index := strings.Index(document, start)
	if index < 0 {
		t.Fatalf("document has no %q section", start)
	}
	return document[index+len(start):]
}

func markdownRows(t *testing.T, section string, pattern *regexp.Regexp) map[string][]string {
	t.Helper()
	rows := map[string][]string{}
	for _, line := range strings.Split(section, "\n") {
		match := pattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		key := strings.TrimSpace(match[1])
		if _, exists := rows[key]; exists {
			t.Fatalf("duplicate Markdown row %q", key)
		}
		values := make([]string, len(match)-2)
		for index, value := range match[2:] {
			values[index] = strings.TrimSpace(value)
			if values[index] == "" {
				t.Errorf("Markdown row %q has an empty column", key)
			}
		}
		rows[key] = values
	}
	return rows
}

func sortedKeys(values map[string][]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func expectedGapDecision(path string) []string {
	switch {
	case strings.HasSuffix(path, "plugin.json"):
		return []string{"不能", "J1"}
	case strings.Contains(path, "/claude/agents/"):
		return []string{"不能", "A1, A2"}
	case strings.Contains(path, "/codex/install/agents/"):
		return []string{"不能", "A1, C1"}
	case strings.Contains(path, "/skills/") && strings.HasSuffix(path, "/SKILL.md"):
		return []string{"不能", "S1"}
	default:
		return []string{"可能", "—"}
	}
}
