package gunte_test

import (
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/akitanabe/gunte/internal/config"
)

const (
	oracleCommit         = "4f014ed2ac6f578f54f0a6f774598fecae3bc36a"
	oracleOutputsSHA256  = "d4b57079913609cbd7d84e0c47a0e8585c1ba17906a7a083cda8cc8b38af8bea"
	digestsSHA256        = "f392585f469da738b149d6befae579a2eef9fa87c7e72c3c7ab7212a4c4f2612"
	sharedTreeSHA256     = "46dc9a58fa00abfe22afa86597ef0f4e6e306bfa2b600282910f4ffcb8361613"
	claudeManifestSHA256 = "472a1856cdfae5e5a843187047af38b4bbfbf370c92e472822a863b20030babc"
	codexManifestSHA256  = "fd6ed3a787939faccc77b959b35fcbed9215d1ba2bfd73fa7a1869851c01aa85"
)

func TestOracleOutputPathsAndGoldenBytesRemainPinnedAndComplete(t *testing.T) {
	fixture := filepath.Join("testdata", "oracle")
	input := filepath.Join(fixture, "input")
	golden := filepath.Join(fixture, "golden")

	outputBytes := readFile(t, filepath.Join(fixture, "ORACLE_OUTPUTS"))
	assertDigest(t, outputBytes, oracleOutputsSHA256)
	outputPaths := nonemptyLines(outputBytes)
	assertSortedUniqueCount(t, outputPaths, 77)

	digestBytes := readFile(t, filepath.Join(fixture, "DIGESTS"))
	assertDigest(t, digestBytes, digestsSHA256)
	digestPaths := assertGoldenDigests(t, golden, nonemptyLines(digestBytes))
	assertEqualStrings(t, digestPaths, outputPaths)
	assertEqualStrings(t, regularFiles(t, golden), outputPaths)
	assertEqualStrings(t, derivedOutputPaths(t, input), outputPaths)
}

func TestOracleInputsManifestsAndDirectivesRemainPinned(t *testing.T) {
	input := filepath.Join("testdata", "oracle", "input")
	assertTreeDigest(t, filepath.Join(input, "shared"), sharedTreeSHA256)
	assertDigest(t, readFile(t, filepath.Join(input, "declarations", "claude", "plugin.json")), claudeManifestSHA256)
	assertDigest(t, readFile(t, filepath.Join(input, "declarations", "codex", "plugin.json")), codexManifestSHA256)
	assertSourceDirectives(t, input)
}

func TestOracleConfigurationMatchesFixtureSourcesTermsAndProfiles(t *testing.T) {
	input := filepath.Join("testdata", "oracle", "input")
	projectBytes := readFile(t, filepath.Join(input, "gunte.toml"))
	project, diagnostics := config.ParseProject("gunte.toml", projectBytes)
	if len(diagnostics) != 0 {
		t.Fatalf("ParseProject() diagnostics = %#v", diagnostics)
	}

	sharedSources := removeString(regularFiles(t, filepath.Join(input, "shared")), "terms.toml")
	wantSources := make([]string, 0, len(sharedSources)+2)
	for _, path := range sharedSources {
		wantSources = append(wantSources, filepath.ToSlash(filepath.Join("shared", path)))
	}
	for _, path := range regularFiles(t, filepath.Join(input, "declarations")) {
		wantSources = append(wantSources, filepath.ToSlash(filepath.Join("declarations", path)))
	}
	sort.Strings(wantSources)
	gotSources := append([]string(nil), project.Sources.Files...)
	sort.Strings(gotSources)
	assertEqualStrings(t, gotSources, wantSources)
	assertUnique(t, project.Sources.Files)

	var sharedTerms struct {
		Terms map[string]map[string]string `toml:"terms"`
	}
	if _, err := toml.Decode(string(readFile(t, filepath.Join(input, "shared", "terms.toml"))), &sharedTerms); err != nil {
		t.Fatal(err)
	}
	if gotTerms := projectTerms(project); !reflect.DeepEqual(gotTerms, sharedTerms.Terms) {
		t.Fatalf("configured terms = %#v, want %#v", gotTerms, sharedTerms.Terms)
	}
	assertOracleProfilesAndFrontmatter(t, project)
}

func TestOracleContractsConfigurationIsValidAndEmpty(t *testing.T) {
	input := filepath.Join("testdata", "oracle", "input")
	project, projectDiagnostics := config.ParseProject(
		"gunte.toml",
		readFile(t, filepath.Join(input, "gunte.toml")),
	)
	if len(projectDiagnostics) != 0 {
		t.Fatalf("ParseProject() diagnostics = %#v", projectDiagnostics)
	}
	contracts, diagnostics := config.ParseContracts(
		"contracts.toml",
		readFile(t, filepath.Join(input, "contracts.toml")),
		project.TargetIDs(),
	)
	if len(diagnostics) != 0 || len(contracts.Contracts) != 0 {
		t.Fatalf("ParseContracts() = (%#v, %#v)", contracts, diagnostics)
	}
}

func TestOracleDocumentationRecordsProvenanceAndGapDecisions(t *testing.T) {
	fixture := filepath.Join("testdata", "oracle")
	readme := string(readFile(t, filepath.Join(fixture, "README.md")))
	for _, required := range []string{
		oracleCommit,
		"git -C",
		" archive ",
		"build_plugin_assets.py",
		"<!-- claude-only:start -->",
		"<!-- @only claude -->",
	} {
		if !strings.Contains(readme, required) {
			t.Errorf("README does not record %q", required)
		}
	}
	if !strings.Contains(strings.ToLower(readme), "sha256") {
		t.Error("README does not record SHA-256 verification")
	}

	outputPaths := nonemptyLines(readFile(t, filepath.Join(fixture, "ORACLE_OUTPUTS")))
	for _, path := range outputPaths {
		if !strings.Contains(readme, "`"+path+"`") {
			t.Errorf("README does not list %s", path)
		}
	}

	gapSection := sectionBetween(t, readme, "### gap", "### artifact 別判定")
	gapRows := markdownRows(t, gapSection, regexp.MustCompile(`^\|\s*([A-Z]\d)\s*\|\s*([^|]+)\|\s*([^|]+)\|\s*([^|]+)\|\s*$`))
	assertEqualStrings(t, sortedKeys(gapRows), []string{"A1", "A2", "C1", "J1", "S1"})

	artifactSection := sectionAfter(t, readme, "### artifact 別判定")
	artifactRows := markdownRows(t, artifactSection, regexp.MustCompile("^\\| `([^`]+)` \\| ([^|]+) \\| ([^|]+) \\|$"))
	assertEqualStrings(t, sortedKeys(artifactRows), outputPaths)
	for _, path := range outputPaths {
		want := expectedGapDecision(path)
		if got := artifactRows[path]; !reflect.DeepEqual(got, want) {
			t.Errorf("README decision for %s = %#v, want %#v", path, got, want)
		}
	}
}
