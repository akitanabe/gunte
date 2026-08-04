package app

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/akitanabe/gunte/internal/compile"
	"github.com/akitanabe/gunte/internal/config"
	"github.com/akitanabe/gunte/internal/contract"
)

func TestRunRejectsUsageAndUnknownTarget(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out"}}})
	runner := NewRunner(root)
	for _, args := range [][]string{{}, {"unknown"}, {"emit", "extra"}, {"emit", "--target"}} {
		result := runner.Run(args)
		if result.ExitCode != ExitUsage {
			t.Errorf("args %v exit = %d, want %d", args, result.ExitCode, ExitUsage)
		}
	}
	result := runner.Run([]string{"emit", "--target", "missing"})
	if result.ExitCode != ExitFailure || !hasDiagnostic(result, "unknown_target") {
		t.Fatalf("unknown target result = %#v", result)
	}
}

func TestRunEmitIsDeterministicAndCheckDoesNotWrite(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out"}}})
	runner := NewRunner(root)
	first := runner.Run([]string{"emit"})
	if first.ExitCode != ExitSuccess {
		t.Fatalf("first emit = %#v", first)
	}
	path := filepath.Join(root, "out", "a.md")
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	second := runner.Run([]string{"emit"})
	if second.ExitCode != ExitSuccess {
		t.Fatalf("second emit = %#v", second)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("second emit changed bytes: %q != %q", got, want)
	}
	checkWriter := &recordingWriter{}
	check := newRunnerWithWriter(root, checkWriter).Run([]string{"check"})
	if check.ExitCode != ExitSuccess || checkWriter.count != 0 {
		t.Fatalf("check = %#v", check)
	}
	if err := os.WriteFile(path, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mismatchWriter := &recordingWriter{}
	mismatch := newRunnerWithWriter(root, mismatchWriter).Run([]string{"check"})
	if mismatch.ExitCode != ExitFailure || mismatchWriter.count != 0 || !hasDiagnostic(mismatch, "output_mismatch") {
		t.Fatalf("mismatch = %#v", mismatch)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	missingWriter := &recordingWriter{}
	missing := newRunnerWithWriter(root, missingWriter).Run([]string{"check"})
	if missing.ExitCode != ExitFailure || missingWriter.count != 0 || !hasDiagnostic(missing, "output_mismatch") {
		t.Fatalf("missing = %#v", missing)
	}
}

func TestEmitUsesProjectRelativePathsAndProducesRootIndependentBytes(t *testing.T) {
	firstRoot := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out/one"}, {id: "two", root: "out/two"}}})
	secondRoot := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out/one"}, {id: "two", root: "out/two"}}})
	if result := NewRunner(firstRoot).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("first root emit = %#v", result)
	}
	if result := NewRunner(secondRoot).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("second root emit = %#v", result)
	}
	for _, relative := range []string{"out/one/a.md", "out/two/a.md"} {
		first, err := os.ReadFile(filepath.Join(firstRoot, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		second, err := os.ReadFile(filepath.Join(secondRoot, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("%s differs across roots: %q != %q", relative, first, second)
		}
	}
}

func TestRunTargetSelectionWritesOnlySelectedTarget(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out/one"}, {id: "two", root: "out/two"}}})
	result := NewRunner(root).Run([]string{"emit", "--target", "two"})
	if result.ExitCode != ExitSuccess {
		t.Fatalf("selected emit = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "out", "two", "a.md")); err != nil {
		t.Fatalf("selected output missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "out", "one", "a.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unselected output exists: %v", err)
	}
}

func TestRunValidatesAllTargetsBeforeSelectedGeneration(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out/one"}, {id: "two", root: "out/two", extra: "unknown = true\n"}}})
	recorder := &recordingWriter{}
	result := newRunnerWithWriter(root, recorder).Run([]string{"emit", "--target", "one"})
	if result.ExitCode != ExitFailure || recorder.count != 0 {
		t.Fatalf("invalid unselected target = %#v writes=%d", result, recorder.count)
	}
}

func TestRunDoesNotWriteWhenInputLoadingFailsMidway(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out"}}})
	projectPath := filepath.Join(root, "gunte.toml")
	project, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	project = []byte(strings.Replace(string(project), "files = [\"src/a.md\"]", "files = [\"src/a.md\", \"src/missing.md\"]", 1))
	if err := os.WriteFile(projectPath, project, 0o644); err != nil {
		t.Fatal(err)
	}
	recorder := &recordingWriter{}
	result := newRunnerWithWriter(root, recorder).Run([]string{"emit"})
	if result.ExitCode != ExitFailure || recorder.count != 0 || !hasDiagnostic(result, "load_error") {
		t.Fatalf("midway source failure = %#v writes=%d", result, recorder.count)
	}
}

func TestRunContinuesOnAdapterWarningWithoutWritingUnmatchedSources(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out", match: "other/*.md"}}})
	recorder := &recordingWriter{}
	result := newRunnerWithWriter(root, recorder).Run([]string{"emit"})
	if result.ExitCode != ExitSuccess || recorder.count != 0 {
		t.Fatalf("adapter warning = %#v writes=%d", result, recorder.count)
	}
}

func TestRunDoesNotWriteWhenInputGenerationOrContractFails(t *testing.T) {
	for _, test := range []struct {
		name    string
		fixture projectFixture
	}{
		{name: "load", fixture: projectFixture{targets: []targetFixture{{id: "one", root: "out"}}, missingSource: true}},
		{name: "parse", fixture: projectFixture{targets: []targetFixture{{id: "one", root: "out"}}, sourceBody: "+++\ninvalid = [\n+++\nbody\n"}},
		{name: "lex", fixture: projectFixture{targets: []targetFixture{{id: "one", root: "out"}}, sourceBody: "<!-- @contract span -->\nbody\n"}},
		{name: "adapter", fixture: projectFixture{targets: []targetFixture{{id: "one", root: "out", extra: "[[targets.one.rules]]\nmatch = \"src/*.md\"\npath = \"duplicate.md\"\nprofile = \"markdown-v1\"\n"}}}},
		{name: "serialize", fixture: projectFixture{targets: []targetFixture{{id: "one", root: "out", profile: "json-v1"}}}},
		{name: "contract", fixture: projectFixture{targets: []targetFixture{{id: "one", root: "out"}}, contracts: "[contracts.bad]\nkind = \"requires\"\nslice = \"missing\"\npattern = \"x\"\napplies_to = [\"one\"]\n"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := writeProject(t, test.fixture)
			recorder := &recordingWriter{}
			result := newRunnerWithWriter(root, recorder).Run([]string{"emit"})
			if result.ExitCode != ExitFailure || recorder.count != 0 {
				t.Fatalf("failure result = %#v writes=%d", result, recorder.count)
			}
		})
	}
}

func TestRunDoesNotWriteAfterActualContractViolation(t *testing.T) {
	root := writeProject(t, projectFixture{
		targets:    []targetFixture{{id: "one", root: "out"}},
		sourceBody: "<!-- @contract span -->\nabsent\n<!-- @/contract -->\n",
		contracts:  "[contracts.need]\nkind = \"requires\"\nslice = \"span\"\npattern = \"wanted\"\napplies_to = [\"one\"]\n",
	})
	recorder := &recordingWriter{}
	result := newRunnerWithWriter(root, recorder).Run([]string{"emit"})
	if result.ExitCode != ExitFailure || recorder.count != 0 || !hasDiagnostic(result, "requires_violation") {
		t.Fatalf("contract violation = %#v writes=%d", result, recorder.count)
	}
}

func TestRunCheckTargetSelectionAndStaleOutputs(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out/one"}, {id: "two", root: "out/two"}}})
	if result := NewRunner(root).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("setup emit = %#v", result)
	}
	if err := os.WriteFile(filepath.Join(root, "out", "one", "a.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if result := NewRunner(root).Run([]string{"check", "--target", "two"}); result.ExitCode != ExitSuccess {
		t.Fatalf("unselected mismatch affected check = %#v", result)
	}
	if err := os.WriteFile(filepath.Join(root, "out", "one", "a.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(root, "out", "stale.md")
	if err := os.WriteFile(stale, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if result := NewRunner(root).Run([]string{"check"}); result.ExitCode != ExitSuccess {
		t.Fatalf("stale output affected check = %#v", result)
	}
	if result := NewRunner(root).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("stale output affected emit = %#v", result)
	}
	if _, err := os.Stat(stale); err != nil {
		t.Fatalf("stale output was removed: %v", err)
	}
}

func TestCheckMismatchKeepsExistingBytesAndDoesNotCreateMissingOutput(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out"}}})
	if result := NewRunner(root).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("setup emit = %#v", result)
	}
	path := filepath.Join(root, "out", "a.md")
	if err := os.WriteFile(path, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recorder := &recordingWriter{}
	result := newRunnerWithWriter(root, recorder).Run([]string{"check"})
	if result.ExitCode != ExitFailure || recorder.count != 0 || len(result.Diagnostics) != 1 || result.Diagnostics[0].Kind != "output_mismatch" || result.Diagnostics[0].Path != "out/a.md" {
		t.Fatalf("mismatch exact = %#v writes=%d", result, recorder.count)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "changed\n" {
		t.Fatalf("mismatch modified existing bytes: %q", got)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	result = newRunnerWithWriter(root, recorder).Run([]string{"check"})
	if result.ExitCode != ExitFailure || len(result.Diagnostics) != 1 || result.Diagnostics[0].Kind != "output_mismatch" || result.Diagnostics[0].Path != "out/a.md" {
		t.Fatalf("missing exact = %#v", result)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("check created missing output: %v", err)
	}
}

func TestSelectedTargetIgnoresFailureOnlyInOtherTarget(t *testing.T) {
	root := writeProject(t, projectFixture{
		targets:    []targetFixture{{id: "one", root: "out/one"}, {id: "two", root: "out/two"}},
		sourceBody: "<!-- @contract span -->\nabsent\n<!-- @/contract -->\n",
		contracts:  "[contracts.only-two]\nkind = \"requires\"\nslice = \"span\"\npattern = \"wanted\"\napplies_to = [\"two\"]\n",
	})
	selected := NewRunner(root).Run([]string{"emit", "--target", "one"})
	if selected.ExitCode != ExitSuccess {
		t.Fatalf("selected target with other failure = %#v", selected)
	}
	if result := NewRunner(root).Run([]string{"check", "--target", "one"}); result.ExitCode != ExitSuccess {
		t.Fatalf("selected check with other failure = %#v", result)
	}
}

func TestRunConfigAndRegistryLoadOrParseFailureNeverWrites(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out"}}})
	if err := os.Remove(filepath.Join(root, "gunte.toml")); err != nil {
		t.Fatal(err)
	}
	recorder := &recordingWriter{}
	result := newRunnerWithWriter(root, recorder).Run([]string{"emit"})
	if result.ExitCode != ExitFailure || recorder.count != 0 || !hasDiagnostic(result, "load_error") {
		t.Fatalf("project load failure = %#v writes=%d", result, recorder.count)
	}
	root = writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out"}}, contracts: "[contracts.bad\n"})
	recorder = &recordingWriter{}
	result = newRunnerWithWriter(root, recorder).Run([]string{"emit"})
	if result.ExitCode != ExitFailure || recorder.count != 0 || !hasDiagnostic(result, "config_error") {
		t.Fatalf("registry parse failure = %#v writes=%d", result, recorder.count)
	}
}

func TestCommandBinaryRunsInProjectRootWithExpectedExitCodesAndLocations(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out"}}})
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../.."))
	bin := filepath.Join(t.TempDir(), "gunte")
	build := exec.Command("go", "build", "-buildvcs=false", "-o", bin, "./cmd/gunte")
	build.Dir = repoRoot
	build.Env = append(os.Environ(), "GOCACHE="+filepath.Join(t.TempDir(), "gocache"))
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	run := func(args ...string) (int, string) {
		command := exec.Command(bin, args...)
		command.Dir = root
		output, err := command.CombinedOutput()
		if err == nil {
			return 0, string(output)
		}
		if exitError, ok := err.(*exec.ExitError); ok {
			return exitError.ExitCode(), string(output)
		}
		t.Fatalf("run CLI: %v", err)
		return -1, string(output)
	}
	if code, output := run("emit"); code != ExitSuccess || output != "" {
		t.Fatalf("CLI emit = code %d output %q", code, output)
	}
	if err := os.WriteFile(filepath.Join(root, "out", "a.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, output := run("check"); code != ExitFailure || !strings.Contains(output, "output_mismatch: out/a.md") {
		t.Fatalf("CLI check = code %d output %q", code, output)
	}
	if code, output := run("unknown"); code != ExitUsage || !strings.Contains(output, "usage: gunte") {
		t.Fatalf("CLI usage = code %d output %q", code, output)
	}
	if err := os.WriteFile(filepath.Join(root, "contracts.toml"), []byte("[contracts.need]\nkind = \"requires\"\nslice = \"span\"\npattern = \"wanted\"\napplies_to = [\"one\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "a.md"), []byte("<!-- @contract span -->\nabsent\n<!-- @/contract -->\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, output := run("emit"); code != ExitFailure || !strings.Contains(output, "requires_violation: contracts.toml:1:1") || !strings.Contains(output, "related src/a.md:") {
		t.Fatalf("CLI contract location = code %d output %q", code, output)
	}
}

func TestCheckReportsNonNotExistReadErrorWithoutWriting(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out"}}})
	if result := NewRunner(root).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("setup emit = %#v", result)
	}
	fs := &failingReadFileSystem{base: localReader{}, suffix: filepath.Join("out", "a.md")}
	recorder := &recordingWriter{}
	runner := newRunnerWithReader(root, fs, recorder)
	result := runner.Run([]string{"check"})
	want := Diagnostic{Kind: "output_error", Path: "out/a.md", Message: "artifact read failed"}
	if result.ExitCode != ExitFailure || recorder.count != 0 || len(result.Diagnostics) != 1 || !reflect.DeepEqual(result.Diagnostics[0], want) {
		t.Fatalf("output read error = %#v writes=%d, want %#v", result, recorder.count, want)
	}
}

func TestCompareOutputCalculatesMatchMismatchMissingAndReadError(t *testing.T) {
	tests := []struct {
		name   string
		input  outputComparison
		failed bool
		want   Diagnostic
	}{
		{name: "match", input: outputComparison{Path: "out/a.md", Expected: []byte("same"), Actual: []byte("same")}},
		{name: "mismatch", input: outputComparison{Path: "out/a.md", Expected: []byte("want"), Actual: []byte("got")}, failed: true, want: Diagnostic{Kind: "output_mismatch", Path: "out/a.md", Message: "output bytes differ"}},
		{name: "missing", input: outputComparison{Path: "out/a.md", Err: os.ErrNotExist}, failed: true, want: Diagnostic{Kind: "output_mismatch", Path: "out/a.md", Message: "output is missing"}},
		{name: "read error", input: outputComparison{Path: "out/a.md", Err: errors.New("read failed")}, failed: true, want: Diagnostic{Kind: "output_error", Path: "out/a.md", Message: "read failed"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, failed := compareOutput(test.input)
			if failed != test.failed || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("compareOutput() = %#v, %v; want %#v, %v", got, failed, test.want, test.failed)
			}
		})
	}
}

func TestRunReturnsWriteFailure(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out/one"}, {id: "two", root: "out/two"}}})
	recorder := &recordingWriter{failAt: 2, err: errors.New("disk full")}
	result := newRunnerWithWriter(root, recorder).Run([]string{"emit"})
	if result.ExitCode != ExitFailure || recorder.count != 2 || !hasDiagnostic(result, "write_error") {
		t.Fatalf("write failure = %#v writes=%d", result, recorder.count)
	}
}

func TestViolationMappingRetainsPredicateArtifactAndRelatedLocations(t *testing.T) {
	predicate := config.ContractPosition{Path: "contracts.toml", Line: 4, Column: 1}
	before := compile.SourcePosition{Path: "source.md", Offset: 10, Line: 2, Column: 3}
	after := compile.SourcePosition{Path: "source.md", Offset: 30, Line: 5, Column: 2}
	violations := []contract.Violation{
		{Kind: contract.RequiresViolation, PredicateID: "need", TargetID: "one", ArtifactPath: "out/need.md", Predicate: predicate, RelatedSource: []compile.SourcePosition{before}},
		{Kind: contract.ForbidsViolation, PredicateID: "ban", TargetID: "one", ArtifactPath: "out/ban.md", Predicate: predicate, RelatedSource: []compile.SourcePosition{before}},
		{Kind: contract.OrderViolation, PredicateID: "order", TargetID: "one", ArtifactPath: "out/order.md", Predicate: predicate, RelatedSource: []compile.SourcePosition{before, after}},
	}
	result := violationResults(violations)
	want := []Diagnostic{
		{Kind: "requires_violation", Path: "contracts.toml", Line: 4, Column: 1, ArtifactPath: "out/need.md", Related: []Location{{Path: "source.md", Line: 2, Column: 3}}, Message: "predicate need failed for target one"},
		{Kind: "forbids_violation", Path: "contracts.toml", Line: 4, Column: 1, ArtifactPath: "out/ban.md", Related: []Location{{Path: "source.md", Line: 2, Column: 3}}, Message: "predicate ban failed for target one"},
		{Kind: "order_violation", Path: "contracts.toml", Line: 4, Column: 1, ArtifactPath: "out/order.md", Related: []Location{{Path: "source.md", Line: 2, Column: 3}, {Path: "source.md", Line: 5, Column: 2}}, Message: "predicate order failed for target one"},
	}
	for index := range want {
		if !reflect.DeepEqual(result[index], want[index]) {
			t.Fatalf("violation %d = %#v, want %#v", index, result[index], want[index])
		}
	}
}

func TestRunDoesNotWriteWhenArtifactGenerationFails(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out/one"}, {id: "two", root: "out/two", profile: "json-v1"}}})
	recorder := &recordingWriter{}
	result := newRunnerWithWriter(root, recorder).Run([]string{"emit"})
	if result.ExitCode != ExitFailure || recorder.count != 0 || !hasDiagnostic(result, "invalid_json") {
		t.Fatalf("late serialization failure = %#v writes=%d", result, recorder.count)
	}
}

func TestRunCheckReportsAllOutputMismatches(t *testing.T) {
	root := writeProject(t, projectFixture{targets: []targetFixture{{id: "one", root: "out/one"}, {id: "two", root: "out/two"}}})
	if result := NewRunner(root).Run([]string{"emit"}); result.ExitCode != ExitSuccess {
		t.Fatalf("setup emit = %#v", result)
	}
	if err := os.WriteFile(filepath.Join(root, "out", "one", "a.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "out", "two", "a.md")); err != nil {
		t.Fatal(err)
	}
	recorder := &recordingWriter{}
	result := newRunnerWithWriter(root, recorder).Run([]string{"check"})
	if result.ExitCode != ExitFailure || recorder.count != 0 || len(result.Diagnostics) != 2 {
		t.Fatalf("all mismatch diagnostics = %#v", result)
	}
	if !hasDiagnostic(result, "output_mismatch") {
		t.Fatalf("mismatch kind missing = %#v", result)
	}
}

type projectFixture struct {
	targets       []targetFixture
	sourceBody    string
	contracts     string
	missingSource bool
}

type targetFixture struct {
	id      string
	root    string
	extra   string
	match   string
	profile string
}

func writeProject(t *testing.T, fixture projectFixture) string {
	t.Helper()
	root := t.TempDir()
	if fixture.sourceBody == "" {
		fixture.sourceBody = "hello\n"
	}
	if !fixture.missingSource {
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "a.md"), []byte(fixture.sourceBody), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var targets strings.Builder
	for _, target := range fixture.targets {
		match := target.match
		if match == "" {
			match = "src/*.md"
		}
		profile := target.profile
		if profile == "" {
			profile = "markdown-v1"
		}
		targets.WriteString("[targets." + target.id + "]\noutput_root = \"" + target.root + "\"\n" + target.extra)
		targets.WriteString("[[targets." + target.id + ".rules]]\nmatch = \"" + match + "\"\npath = \"a.md\"\nprofile = \"" + profile + "\"\n")
	}
	project := "spec_version = 1\n[project]\nid = \"p\"\nversion = \"v\"\n[sources]\nfiles = [\"src/a.md\"]\n" + targets.String()
	if err := os.WriteFile(filepath.Join(root, "gunte.toml"), []byte(project), 0o644); err != nil {
		t.Fatal(err)
	}
	contracts := fixture.contracts
	if contracts == "" {
		contracts = "[contracts]\n"
	}
	if err := os.WriteFile(filepath.Join(root, "contracts.toml"), []byte(contracts), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

type recordingWriter struct {
	count  int
	failAt int
	err    error
}

type failingReadFileSystem struct {
	base   reader
	suffix string
}

func (fs *failingReadFileSystem) ReadFile(path string) ([]byte, error) {
	if strings.HasSuffix(path, fs.suffix) {
		return nil, errors.New("artifact read failed")
	}
	return fs.base.ReadFile(path)
}

func (writer *recordingWriter) Write(path string, body []byte) error {
	writer.count++
	if writer.failAt != 0 && writer.count != writer.failAt {
		return nil
	}
	return writer.err
}

func hasDiagnostic(result Result, kind string) bool {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Kind == kind {
			return true
		}
	}
	return false
}
